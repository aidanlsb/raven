package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

var openCmd = &cobra.Command{
	Use:   "open <reference>",
	Short: "Open a file in your editor",
	Long: `Opens a file in your configured editor.

The reference can be:
  - A short reference: cursor, freya, bifrost
  - A partial path: companies/cursor, people/freya
  - A full path: objects/companies/cursor.md

The editor is determined by (in order):
  1. The 'editor' setting in ~/.config/raven/config.toml
  2. The $EDITOR environment variable

Examples:
  rvn open cursor              # Opens companies/cursor.md
  rvn open companies/cursor    # Opens objects/companies/cursor.md
  rvn open people/freya        # Opens people/freya.md
  rvn open daily/2025-01-09    # Opens a specific daily note`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]

		// Load vault config
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Resolve the reference to a file path
		filePath, err := resolveReferenceToFile(vaultPath, reference, vaultCfg)
		if err != nil {
			// In JSON mode, error is already output
			if jsonOutput {
				return nil
			}
			return err
		}

		relPath, _ := filepath.Rel(vaultPath, filePath)

		// JSON output
		if isJSONOutput() {
			cfg := getConfig()
			editor := ""
			if cfg != nil {
				editor = cfg.GetEditor()
			}

			opened := vault.OpenInEditor(cfg, filePath)
			outputSuccess(map[string]interface{}{
				"file":   relPath,
				"opened": opened,
				"editor": editor,
			}, nil)
			return nil
		}

		// Human output - open in editor
		openFileInEditor(filePath, relPath, false)

		return nil
	},
}

// openFileInEditor opens a file in the configured editor and prints appropriate output.
// If skipOpenMessage is true, it won't print "Opening..." (useful when a "Created" message was already shown).
func openFileInEditor(filePath, relPath string, skipOpenMessage bool) {
	cfg := getConfig()
	if vault.OpenInEditor(cfg, filePath) {
		if !skipOpenMessage {
			fmt.Printf("Opening %s\n", relPath)
		}
	} else {
		fmt.Printf("File: %s\n", relPath)
		fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
	}
}

// resolveReferenceToFile resolves a reference to an absolute file path.
// It handles short references, partial paths, and full paths.
func resolveReferenceToFile(vaultPath, reference string, vaultCfg *config.VaultConfig) (string, error) {
	// First, try treating it as a literal path
	literalPath := filepath.Join(vaultPath, reference)
	if fileExists(literalPath) {
		return literalPath, nil
	}

	// Try adding .md extension if not present
	if !strings.HasSuffix(reference, ".md") {
		literalPathMd := filepath.Join(vaultPath, reference+".md")
		if fileExists(literalPathMd) {
			return literalPathMd, nil
		}
	}

	// Try to resolve as a reference using the database
	db, err := index.Open(vaultPath)
	if err != nil {
		return "", resolveRefError(ErrDatabaseError,
			fmt.Sprintf("Failed to open database: %v", err),
			"Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	// Get the resolver from the database
	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.DailyDirectory != "" {
		dailyDir = vaultCfg.DailyDirectory
	}
	res, err := db.Resolver(dailyDir)
	if err != nil {
		return "", resolveRefError(ErrDatabaseError,
			fmt.Sprintf("Failed to create resolver: %v", err),
			"Run 'rvn reindex' to rebuild the database")
	}

	// Try to resolve the reference
	result := res.Resolve(reference)
	if result.Ambiguous {
		return "", resolveRefError(ErrRefAmbiguous,
			fmt.Sprintf("Reference '%s' is ambiguous, matches: %v", reference, result.Matches),
			"Use a more specific path")
	}

	if result.TargetID == "" {
		return "", resolveRefError(ErrRefNotFound,
			fmt.Sprintf("Reference '%s' not found", reference),
			"Check the reference and try again")
	}

	// Handle section references - strip the #section part for file opening
	targetID := result.TargetID
	if idx := strings.Index(targetID, "#"); idx >= 0 {
		targetID = targetID[:idx]
	}

	// Convert the resolved object ID to a file path
	resolvedPath, err := vault.ResolveObjectToFileWithConfig(vaultPath, targetID, vaultCfg)
	if err != nil {
		return "", resolveRefError(ErrFileNotFound,
			fmt.Sprintf("Could not find file for '%s'", targetID),
			"The reference resolved but the file could not be found")
	}

	// Verify file exists
	if !fileExists(resolvedPath) {
		return "", resolveRefError(ErrFileNotFound,
			fmt.Sprintf("File '%s' does not exist", resolvedPath),
			"The file may have been deleted or moved")
	}

	return resolvedPath, nil
}

// resolveRefError outputs a JSON error if in JSON mode and returns an error.
func resolveRefError(code, message, suggestion string) error {
	if jsonOutput {
		outputError(code, message, nil, suggestion)
	}
	return fmt.Errorf("%s", message)
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func init() {
	rootCmd.AddCommand(openCmd)
}
