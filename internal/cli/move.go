package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/spf13/cobra"
)

var (
	moveForce          bool
	moveUpdateRefs     bool
	moveSkipTypeCheck  bool
)

var moveCmd = &cobra.Command{
	Use:   "move <source> <destination>",
	Short: "Move or rename an object within the vault",
	Long: `Move or rename a file/object within the vault.

Both source and destination must be within the vault. This command:
- Validates paths are within the vault (security constraint)
- Updates all references to the moved file if --update-refs is set
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed

Examples:
  rvn move people/loki people/loki-archived      # Rename
  rvn move inbox/task.md projects/website/task.md # Move to subdirectory
  rvn move drafts/person.md people/freya.md --update-refs`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		source := args[0]
		destination := args[1]

		// Normalize paths (add .md if missing)
		source = normalizePath(source)
		destination = normalizePath(destination)

		// Resolve source file
		sourceFile, err := vault.ResolveObjectToFile(vaultPath, strings.TrimSuffix(source, ".md"))
		if err != nil {
			return handleErrorMsg(ErrFileNotFound,
				fmt.Sprintf("Source '%s' does not exist", source),
				"Check the source path and try again")
		}

		// Security: Validate source is within vault
		if err := validateWithinVault(vaultPath, sourceFile); err != nil {
			return handleErrorMsg(ErrValidationFailed,
				"Source path is outside vault",
				"Files can only be moved within the vault")
		}

		// Build destination path
		destFile := filepath.Join(vaultPath, destination)

		// Security: Validate destination is within vault
		if err := validateWithinVault(vaultPath, destFile); err != nil {
			return handleErrorMsg(ErrValidationFailed,
				"Destination path is outside vault",
				"Files can only be moved within the vault")
		}

		// Security: Ensure destination is not in .raven directory
		relDest, _ := filepath.Rel(vaultPath, destFile)
		if strings.HasPrefix(relDest, ".raven") || strings.HasPrefix(relDest, ".trash") {
			return handleErrorMsg(ErrValidationFailed,
				"Cannot move to system directory",
				"Destination cannot be in .raven or .trash directories")
		}

		// Check if destination already exists
		if _, err := os.Stat(destFile); err == nil {
			return handleErrorMsg(ErrValidationFailed,
				fmt.Sprintf("Destination '%s' already exists", destination),
				"Choose a different destination or delete the existing file first")
		}

		// Load schema for type checking
		sch, err := schema.Load(vaultPath)
		if err != nil {
			sch = schema.NewSchema()
		}

		// Parse source file to get its type
		content, err := os.ReadFile(sourceFile)
		if err != nil {
			return handleError(ErrFileReadError, err, "")
		}
		doc, err := parser.ParseDocument(string(content), sourceFile, vaultPath)
		if err != nil {
		return handleError(ErrInternal, err, "Failed to parse source file")
	}

	// Get file type from first object
	var fileType string
	if len(doc.Objects) > 0 {
		fileType = doc.Objects[0].ObjectType
	}

		// Check for type-directory mismatch
		var warnings []Warning
		destDir := filepath.Dir(relDest)
		mismatchType := ""
		for typeName, typeDef := range sch.Types {
			if typeDef.DefaultPath != "" {
				defaultPath := strings.TrimSuffix(typeDef.DefaultPath, "/")
				if destDir == defaultPath && typeName != fileType {
					mismatchType = typeName
					break
				}
			}
		}

		if mismatchType != "" && !moveSkipTypeCheck {
			warnings = append(warnings, Warning{
				Code: WarnTypeMismatch,
				Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
					destDir, mismatchType, fileType),
				Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatchType),
			})

			// In JSON mode, return warning for agent to handle
			if isJSONOutput() {
				result := MoveResult{
					Source:       strings.TrimSuffix(source, ".md"),
					Destination:  strings.TrimSuffix(destination, ".md"),
					NeedsConfirm: true,
					Reason:       fmt.Sprintf("Type mismatch: file is '%s' but destination is default path for '%s'", fileType, mismatchType),
				}
				outputSuccessWithWarnings(result, warnings, nil)
				return nil
			}

			// Interactive confirmation
			if !moveForce {
				fmt.Printf("⚠ Warning: Moving to '%s/' which is the default directory for type '%s'\n", destDir, mismatchType)
				fmt.Printf("  But this file has type '%s'\n\n", fileType)
				fmt.Print("Proceed anyway? [y/N]: ")

				reader := bufio.NewReader(os.Stdin)
				response, err := reader.ReadString('\n')
				if err != nil {
					return handleError(ErrInternal, err, "")
				}
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					fmt.Println("Cancelled.")
					return nil
				}
			}
		}

		// Find backlinks to update
		var updatedRefs []string
		if moveUpdateRefs {
			db, err := index.Open(vaultPath)
			if err == nil {
				defer db.Close()
				sourceID := strings.TrimSuffix(source, ".md")
				backlinks, _ := db.Backlinks(sourceID)

				destID := strings.TrimSuffix(destination, ".md")
				for _, bl := range backlinks {
					if err := updateReference(vaultPath, bl.SourceID, sourceID, destID); err == nil {
						updatedRefs = append(updatedRefs, bl.SourceID)
					}
				}
			}
		}

		// Create destination directory if needed
		destDirPath := filepath.Dir(destFile)
		if err := os.MkdirAll(destDirPath, 0755); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		// Perform the move
		if err := os.Rename(sourceFile, destFile); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

	// Update index
	db, err := index.Open(vaultPath)
	if err == nil {
		defer db.Close()
		// Remove old entry
		sourceID := strings.TrimSuffix(source, ".md")
		db.RemoveDocument(sourceID)
		// Index new location
		newContent, _ := os.ReadFile(destFile)
		newDoc, _ := parser.ParseDocument(string(newContent), destFile, vaultPath)
		if newDoc != nil {
			db.IndexDocument(newDoc, sch)
		}
	}

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			result := MoveResult{
				Source:      strings.TrimSuffix(source, ".md"),
				Destination: strings.TrimSuffix(destination, ".md"),
				UpdatedRefs: updatedRefs,
			}
			outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		relSource, _ := filepath.Rel(vaultPath, sourceFile)
		fmt.Printf("✓ Moved %s → %s\n", relSource, destination)
		if len(updatedRefs) > 0 {
			fmt.Printf("  Updated %d references\n", len(updatedRefs))
		}

		return nil
	},
}

// MoveResult represents the result of a move operation.
type MoveResult struct {
	Source       string   `json:"source"`
	Destination  string   `json:"destination"`
	UpdatedRefs  []string `json:"updated_refs,omitempty"`
	NeedsConfirm bool     `json:"needs_confirm,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// validateWithinVault checks that a path is within the vault.
func validateWithinVault(vaultPath, targetPath string) error {
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		return err
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	// Resolve any symlinks for security
	realVault, err := filepath.EvalSymlinks(absVault)
	if err != nil {
		realVault = absVault
	}

	// For target, we may be checking a path that doesn't exist yet
	// So check the parent directory
	targetDir := filepath.Dir(absTarget)
	realTargetDir, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		// Parent might not exist yet, check grandparent
		realTargetDir = targetDir
	}

	// Ensure target is within vault
	if !strings.HasPrefix(realTargetDir+string(filepath.Separator), realVault+string(filepath.Separator)) &&
		realTargetDir != realVault {
		return fmt.Errorf("path is outside vault")
	}

	return nil
}

// normalizePath ensures the path has a .md extension.
func normalizePath(p string) string {
	if !strings.HasSuffix(p, ".md") {
		return p + ".md"
	}
	return p
}

// updateReference updates a reference in a source file.
func updateReference(vaultPath, sourceID, oldRef, newRef string) error {
	filePath, err := vault.ResolveObjectToFile(vaultPath, sourceID)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Replace the reference
	oldPattern := "[[" + oldRef + "]]"
	newPattern := "[[" + newRef + "]]"
	newContent := strings.ReplaceAll(string(content), oldPattern, newPattern)

	// Also handle references with display text: [[old|text]] -> [[new|text]]
	oldPatternWithText := "[[" + oldRef + "|"
	newPatternWithText := "[[" + newRef + "|"
	newContent = strings.ReplaceAll(newContent, oldPatternWithText, newPatternWithText)

	if newContent == string(content) {
		return nil // No changes needed
	}

	return os.WriteFile(filePath, []byte(newContent), 0644)
}

func init() {
	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompts")
	moveCmd.Flags().BoolVar(&moveUpdateRefs, "update-refs", true, "Update references to moved file")
	moveCmd.Flags().BoolVar(&moveSkipTypeCheck, "skip-type-check", false, "Skip type-directory mismatch warning")
	rootCmd.AddCommand(moveCmd)
}
