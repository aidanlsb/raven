package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	moveForce         bool
	moveUpdateRefs    bool
	moveSkipTypeCheck bool
	moveStdin         bool
	moveConfirm       bool
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

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Destination must be a directory (ending with /).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn move people/loki people/loki-archived      # Rename
  rvn move inbox/task.md projects/website/task.md # Move to subdirectory
  rvn move drafts/person.md people/freya.md --update-refs

Bulk examples:
  rvn query "object:project .status==archived" --ids | rvn move --stdin archive/projects/
  rvn query "object:project .status==archived" --ids | rvn move --stdin archive/projects/ --confirm`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --stdin mode for bulk operations
		if moveStdin {
			return runMoveBulk(args, vaultPath)
		}

		// Single object mode - requires source and destination
		if len(args) < 2 {
			return handleErrorMsg(ErrMissingArgument, "requires source and destination arguments", "Usage: rvn move <source> <destination>")
		}

		return moveSingleObject(vaultPath, args[0], args[1])
	},
}

// runMoveBulk handles bulk move operations from stdin.
func runMoveBulk(args []string, vaultPath string) error {
	// Destination is provided as argument
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no destination provided", "Usage: rvn move --stdin <destination-directory/>")
	}
	destination := args[0]

	// Destination must be a directory (end with /)
	if !strings.HasSuffix(destination, "/") {
		return handleErrorMsg(ErrInvalidInput,
			"destination must be a directory (end with /)",
			"Example: rvn move --stdin archive/projects/")
	}

	// Read IDs from stdin
	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	// Build warnings for embedded objects
	var warnings []Warning
	if w := BuildEmbeddedSkipWarning(embedded); w != nil {
		warnings = append(warnings, *w)
	}

	// Load vault config
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil || vaultCfg == nil {
		vaultCfg = &config.VaultConfig{}
	}

	// If not confirming, show preview
	if !moveConfirm {
		return previewMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
	}

	// Apply the moves
	return applyMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
}

// previewMoveBulk shows a preview of bulk move operations.
func previewMoveBulk(vaultPath string, ids []string, destDir string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	preview := buildBulkPreview("move", ids, warnings, func(id string) (*BulkPreviewItem, *BulkResult) {
		sourceFile, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: "object not found"}
		}

		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(destDir, filename)
		fullDestPath := filepath.Join(vaultPath, destPath)
		if _, err := os.Stat(fullDestPath); err == nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("destination already exists: %s", destPath)}
		}

		return &BulkPreviewItem{
			ID:      id,
			Action:  "move",
			Details: fmt.Sprintf("→ %s", destPath),
		}, nil
	})

	return outputBulkPreview(preview, map[string]interface{}{
		"destination": destDir,
	})
}

// applyMoveBulk applies bulk move operations.
func applyMoveBulk(vaultPath string, ids []string, destDir string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	// Load schema for type checking
	sch, _ := schema.Load(vaultPath)

	// Open database for reference updates
	db, err := index.Open(vaultPath)
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	// Create destination directory
	fullDestDir := filepath.Join(vaultPath, destDir)
	if err := os.MkdirAll(fullDestDir, 0755); err != nil {
		return handleError(ErrFileWriteError, err, "Failed to create destination directory")
	}

	results := applyBulk(ids, func(id string) BulkResult {
		result := BulkResult{ID: id}
		sourceFile, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			return result
		}

		// Build destination path
		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(destDir, filename)
		fullDestPath := filepath.Join(vaultPath, destPath)

		// Check if destination already exists
		if _, err := os.Stat(fullDestPath); err == nil {
			result.Status = "skipped"
			result.Reason = fmt.Sprintf("destination already exists: %s", destPath)
			return result
		}

		// Update references if enabled
		if moveUpdateRefs {
			relSource, _ := filepath.Rel(vaultPath, sourceFile)
			sourceID := vaultCfg.FilePathToObjectID(relSource)
			destID := vaultCfg.FilePathToObjectID(destPath)
			aliases, _ := db.AllAliases()
			res, _ := db.Resolver(index.ResolverOptions{DailyDirectory: vaultCfg.DailyDirectory, ExtraIDs: []string{destID}})
			aliasSlugToID := make(map[string]string, len(aliases))
			for a, oid := range aliases {
				aliasSlugToID[pages.SlugifyPath(a)] = oid
			}

			backlinks, _ := db.Backlinks(sourceID)
			for _, bl := range backlinks {
				oldRaw := strings.TrimSpace(bl.TargetRaw)
				oldRaw = strings.TrimPrefix(strings.TrimSuffix(oldRaw, "]]"), "[[") // tolerate bracketed legacy data
				base := oldRaw
				if i := strings.Index(base, "#"); i >= 0 {
					base = base[:i]
				}
				if base == "" {
					continue
				}
				repl := chooseReplacementRefBase(base, sourceID, destID, aliasSlugToID, res)
				line := 0
				if bl.Line != nil {
					line = *bl.Line
				}
				if err := updateReferenceAtLine(vaultPath, vaultCfg, bl.SourceID, line, base, repl); err != nil {
					// Best-effort: moving the file is the primary action; reference updates may fail.
					continue
				}
			}
		}

		// Perform the move
		if err := os.Rename(sourceFile, fullDestPath); err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("move failed: %v", err)
			return result
		}

		// Update index
		sourceID := strings.TrimSuffix(id, ".md")
		if err := db.RemoveDocument(sourceID); err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("failed to remove from index: %v", err)
			return result
		}

		// Reindex new location
		newContent, err := os.ReadFile(fullDestPath)
		if err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("failed to read moved file: %v", err)
			return result
		}
		var parseOpts *parser.ParseOptions
		if vaultCfg.HasDirectoriesConfig() {
			parseOpts = &parser.ParseOptions{
				ObjectsRoot: vaultCfg.GetObjectsRoot(),
				PagesRoot:   vaultCfg.GetPagesRoot(),
			}
		}
		newDoc, err := parser.ParseDocumentWithOptions(string(newContent), fullDestPath, vaultPath, parseOpts)
		if err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("failed to parse moved file: %v", err)
			return result
		}
		if newDoc == nil {
			result.Status = "error"
			result.Reason = "failed to parse moved file: got nil document"
			return result
		}
		if sch != nil {
			if err := db.IndexDocument(newDoc, sch); err != nil {
				result.Status = "error"
				result.Reason = fmt.Sprintf("failed to index moved file: %v", err)
				return result
			}
		}

		result.Status = "moved"
		result.Details = destPath
		return result
	})

	summary := buildBulkSummary("move", results, warnings)
	return outputBulkSummary(summary, warnings, map[string]interface{}{
		"destination": destDir,
	})
}

// moveSingleObject handles single move operation (non-bulk mode).
func moveSingleObject(vaultPath, source, destination string) error {
	start := time.Now()

	// Load vault config for directory roots
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil || vaultCfg == nil {
		vaultCfg = &config.VaultConfig{}
	}

	// Normalize destination path (add .md if missing)
	destination = normalizePath(destination)

	// Resolve source using unified resolver (supports short names, aliases, etc.)
	sourceResult, err := ResolveReference(source, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return handleResolveError(err, source)
	}
	sourceFile := sourceResult.FilePath

	// Security: Validate source is within vault
	if err := validateWithinVault(vaultPath, sourceFile); err != nil {
		return handleErrorMsg(ErrValidationFailed,
			"Source path is outside vault",
			"Files can only be moved within the vault")
	}

	// Build destination path - apply directory roots if configured
	destPath := destination
	if vaultCfg.HasDirectoriesConfig() {
		// If destination is an object ID (like "people/freya.md"), resolve to file path
		destPath = vaultCfg.ResolveReferenceToFilePath(strings.TrimSuffix(destination, ".md"))
	}
	destFile := filepath.Join(vaultPath, destPath)

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

	// Build parse options from vault config
	var parseOpts *parser.ParseOptions
	if vaultCfg.HasDirectoriesConfig() {
		parseOpts = &parser.ParseOptions{
			ObjectsRoot: vaultCfg.GetObjectsRoot(),
			PagesRoot:   vaultCfg.GetPagesRoot(),
		}
	}

	// Parse source file to get its type
	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), sourceFile, vaultPath, parseOpts)
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
				Source:       vaultCfg.FilePathToObjectID(source),
				Destination:  vaultCfg.FilePathToObjectID(destPath),
				NeedsConfirm: true,
				Reason:       fmt.Sprintf("Type mismatch: file is '%s' but destination is default path for '%s'", fileType, mismatchType),
			}
			outputSuccessWithWarnings(result, warnings, nil)
			return nil
		}

		// Interactive confirmation
		if !moveForce {
			fmt.Fprintf(os.Stderr, "⚠ Warning: Moving to '%s/' which is the default directory for type '%s'\n", destDir, mismatchType)
			fmt.Fprintf(os.Stderr, "  But this file has type '%s'\n\n", fileType)
			fmt.Fprint(os.Stderr, "Proceed anyway? [y/N]: ")

			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return handleError(ErrInternal, err, "")
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Fprintln(os.Stderr, "Cancelled.")
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
			sourceID := vaultCfg.FilePathToObjectID(source)
			backlinks, _ := db.Backlinks(sourceID)

			destID := vaultCfg.FilePathToObjectID(destPath)
			aliases, _ := db.AllAliases()
			res, _ := db.Resolver(index.ResolverOptions{DailyDirectory: vaultCfg.DailyDirectory, ExtraIDs: []string{destID}})
			aliasSlugToID := make(map[string]string, len(aliases))
			for a, oid := range aliases {
				aliasSlugToID[pages.SlugifyPath(a)] = oid
			}

			for _, bl := range backlinks {
				oldRaw := strings.TrimSpace(bl.TargetRaw)
				oldRaw = strings.TrimPrefix(strings.TrimSuffix(oldRaw, "]]"), "[[") // tolerate bracketed legacy data
				base := oldRaw
				if i := strings.Index(base, "#"); i >= 0 {
					base = base[:i]
				}
				if base == "" {
					continue
				}
				repl := chooseReplacementRefBase(base, sourceID, destID, aliasSlugToID, res)
				line := 0
				if bl.Line != nil {
					line = *bl.Line
				}
				if err := updateReferenceAtLine(vaultPath, vaultCfg, bl.SourceID, line, base, repl); err == nil {
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
	if err != nil {
		warnings = append(warnings, Warning{
			Code:    WarnIndexUpdateFailed,
			Message: fmt.Sprintf("Failed to open index database for update: %v", err),
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	} else {
		defer db.Close()

		// Remove old entry
		sourceID := vaultCfg.FilePathToObjectID(source)
		if err := db.RemoveDocument(sourceID); err != nil {
			warnings = append(warnings, Warning{
				Code:    WarnIndexUpdateFailed,
				Message: fmt.Sprintf("Failed to remove old index entry: %v", err),
				Ref:     "Run 'rvn reindex' to rebuild the database",
			})
		}

		// Index new location
		newContent, err := os.ReadFile(destFile)
		if err != nil {
			warnings = append(warnings, Warning{
				Code:    WarnIndexUpdateFailed,
				Message: fmt.Sprintf("Failed to read moved file for indexing: %v", err),
				Ref:     "Run 'rvn reindex' to rebuild the database",
			})
		} else {
			newDoc, err := parser.ParseDocumentWithOptions(string(newContent), destFile, vaultPath, parseOpts)
			if err != nil {
				warnings = append(warnings, Warning{
					Code:    WarnIndexUpdateFailed,
					Message: fmt.Sprintf("Failed to parse moved file for indexing: %v", err),
					Ref:     "Run 'rvn reindex' to rebuild the database",
				})
			} else if newDoc == nil {
				warnings = append(warnings, Warning{
					Code:    WarnIndexUpdateFailed,
					Message: "Failed to parse moved file for indexing: got nil document",
					Ref:     "Run 'rvn reindex' to rebuild the database",
				})
			} else {
				if err := db.IndexDocument(newDoc, sch); err != nil {
					warnings = append(warnings, Warning{
						Code:    WarnIndexUpdateFailed,
						Message: fmt.Sprintf("Failed to index moved file: %v", err),
						Ref:     "Run 'rvn reindex' to rebuild the database",
					})
				}
			}
		}
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := MoveResult{
			Source:      vaultCfg.FilePathToObjectID(source),
			Destination: vaultCfg.FilePathToObjectID(destPath),
			UpdatedRefs: updatedRefs,
		}
		outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	relSource, _ := filepath.Rel(vaultPath, sourceFile)
	fmt.Println(ui.Checkf("Moved %s → %s", ui.FilePath(relSource), ui.FilePath(destination)))
	if len(updatedRefs) > 0 {
		fmt.Printf("  Updated %d references\n", len(updatedRefs))
	}

	return nil
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
func updateReference(vaultPath string, vaultCfg *config.VaultConfig, sourceID, oldRef, newRef string) error {
	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, sourceID, vaultCfg)
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

	// Also handle section/fragment links: [[old#section]] -> [[new#section]]
	oldPatternWithFragment := "[[" + oldRef + "#"
	newPatternWithFragment := "[[" + newRef + "#"
	newContent = strings.ReplaceAll(newContent, oldPatternWithFragment, newPatternWithFragment)

	if newContent == string(content) {
		return nil // No changes needed
	}

	return atomicfile.WriteFile(filePath, []byte(newContent), 0o644)
}

func updateReferenceAtLine(vaultPath string, vaultCfg *config.VaultConfig, sourceID string, line int, oldRef, newRef string) error {
	if line <= 0 {
		// Fallback to whole-file replacement.
		return updateReference(vaultPath, vaultCfg, sourceID, oldRef, newRef)
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, sourceID, vaultCfg)
	if err != nil {
		return err
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(contentBytes), "\n")
	idx := line - 1 // 1-indexed in DB
	if idx < 0 || idx >= len(lines) {
		// Out of range; avoid rewriting unknown locations.
		return nil
	}

	orig := lines[idx]
	updated := orig

	// Replace the reference
	oldPattern := "[[" + oldRef + "]]"
	newPattern := "[[" + newRef + "]]"
	updated = strings.ReplaceAll(updated, oldPattern, newPattern)

	// Also handle references with display text: [[old|text]] -> [[new|text]]
	oldPatternWithText := "[[" + oldRef + "|"
	newPatternWithText := "[[" + newRef + "|"
	updated = strings.ReplaceAll(updated, oldPatternWithText, newPatternWithText)

	// Also handle section/fragment links: [[old#section]] -> [[new#section]]
	oldPatternWithFragment := "[[" + oldRef + "#"
	newPatternWithFragment := "[[" + newRef + "#"
	updated = strings.ReplaceAll(updated, oldPatternWithFragment, newPatternWithFragment)

	if updated == orig {
		return nil
	}
	lines[idx] = updated

	return atomicfile.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644)
}

func shortNameFromID(id string) string {
	id = strings.TrimSuffix(id, ".md")
	if i := strings.LastIndex(id, "/"); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

func chooseReplacementRefBase(oldBase, sourceID, destID string, aliasSlugToID map[string]string, res *resolver.Resolver) string {
	// If the original reference was explicit (contains a path), keep it explicit.
	if strings.Contains(oldBase, "/") {
		return destID
	}

	// If the original reference looks like an alias for this object, keep it stable.
	// Aliases are designed to survive renames/moves without needing ref rewrites.
	if aliasSlugToID != nil {
		if aliasSlugToID[pages.SlugifyPath(oldBase)] == sourceID {
			return oldBase
		}
	}

	// Otherwise, this is a short ref. Prefer keeping it short *if* the new short name
	// resolves uniquely to the destination ID.
	candidate := shortNameFromID(destID)
	if candidate != "" && res != nil {
		r := res.Resolve(candidate)
		if !r.Ambiguous && r.TargetID == destID {
			return candidate
		}
	}

	// Fall back to explicit ref (always correct).
	return destID
}

func init() {
	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompts")
	moveCmd.Flags().BoolVar(&moveUpdateRefs, "update-refs", true, "Update references to moved file")
	moveCmd.Flags().BoolVar(&moveSkipTypeCheck, "skip-type-check", false, "Skip type-directory mismatch warning")
	moveCmd.Flags().BoolVar(&moveStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	moveCmd.Flags().BoolVar(&moveConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(moveCmd)
}
