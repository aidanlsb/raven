package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

var migrateDirectoriesCmd = &cobra.Command{
	Use:   "directories",
	Short: "Migrate vault to new directory organization",
	Long: `Migrate an existing vault to use the directory organization structure.

This command reads the 'directories' configuration from raven.yaml and moves
files to their new locations:
- Typed objects → object/<type>/
- Untyped pages → page/
- Daily notes → as configured (usually unchanged)

All [[references]] throughout the vault remain valid because object IDs don't change.
The physical file paths change, but the logical IDs (person/freya, project/website)
stay the same.

Before running, ensure your raven.yaml has a 'directories' section:

  directories:
    object: object/
    page: page/

Run with --dry-run first to preview changes.

Examples:
  rvn migrate directories --dry-run  # Preview what would change
  rvn migrate directories            # Apply the migration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Load vault config
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Check if directories are configured
		dirs := vaultCfg.GetDirectoriesConfig()
		if dirs == nil || (dirs.Object == "" && dirs.Page == "") {
			return handleErrorMsg(ErrValidationFailed,
				"No directories configuration found in raven.yaml",
				"Add a 'directories' section with 'object' and/or 'page' keys")
		}

		// Load schema for type information
		sch, err := schema.Load(vaultPath)
		if err != nil {
			sch = schema.NewSchema()
		}

		if dryRun {
			fmt.Println("=== DRY RUN - No changes will be made ===")
			fmt.Println()
		}

		fmt.Printf("Migrating vault: %s\n", vaultPath)
		if dirs.Object != "" {
			fmt.Printf("  Object root: %s\n", dirs.Object)
		}
		if dirs.Page != "" {
			fmt.Printf("  Page root: %s\n", dirs.Page)
		}
		fmt.Println()

		// Collect files to move
		type fileMove struct {
			oldPath string // Current relative path
			newPath string // New relative path
			objType string // Object type (for categorization)
		}
		var moves []fileMove

		err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
			if result.Error != nil {
				return nil // Skip errors
			}

			relPath := result.RelativePath

			// Skip files already in target directories
			if dirs.Object != "" && strings.HasPrefix(relPath, dirs.Object) {
				return nil
			}
			if dirs.Page != "" && strings.HasPrefix(relPath, dirs.Page) {
				return nil
			}

			// Skip daily notes, templates, workflows
			if strings.HasPrefix(relPath, vaultCfg.GetDailyDirectory()+"/") ||
				strings.HasPrefix(relPath, "templates/") ||
				strings.HasPrefix(relPath, "workflows/") {
				return nil
			}

			// Get file type from parsed document
			objType := "page"
			if result.Document != nil && len(result.Document.Objects) > 0 {
				objType = result.Document.Objects[0].ObjectType
			}

			// Determine new path
			var newPath string
			if objType == "page" || objType == "" {
				// Untyped page
				if dirs.Page != "" {
					newPath = filepath.Join(dirs.Page, relPath)
				} else if dirs.Object != "" {
					newPath = filepath.Join(dirs.Object, relPath)
				}
			} else {
				// Typed object - check if it has a default_path in schema
				if typeDef, ok := sch.Types[objType]; ok && typeDef != nil && typeDef.DefaultPath != "" {
					// File is in a type's default_path directory
					defaultPath := strings.TrimSuffix(typeDef.DefaultPath, "/")
					if strings.HasPrefix(relPath, defaultPath+"/") {
						// Already in correct type directory, just add object root
						if dirs.Object != "" {
							newPath = filepath.Join(dirs.Object, relPath)
						}
					} else {
						// File is typed but not in its default directory
						// Move to object root, keeping current structure
						if dirs.Object != "" {
							newPath = filepath.Join(dirs.Object, relPath)
						}
					}
				} else {
					// Typed but no default_path in schema
					if dirs.Object != "" {
						newPath = filepath.Join(dirs.Object, relPath)
					}
				}
			}

			if newPath != "" && newPath != relPath {
				moves = append(moves, fileMove{
					oldPath: relPath,
					newPath: newPath,
					objType: objType,
				})
			}

			return nil
		})

		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if len(moves) == 0 {
			fmt.Println("✓ No files need to be moved. Vault already organized correctly.")
			return nil
		}

		// Group by category for display
		var objectMoves, pageMoves []fileMove
		for _, m := range moves {
			if m.objType == "page" || m.objType == "" {
				pageMoves = append(pageMoves, m)
			} else {
				objectMoves = append(objectMoves, m)
			}
		}

		if len(objectMoves) > 0 {
			fmt.Printf("Typed objects to move: %d\n", len(objectMoves))
			for _, m := range objectMoves {
				fmt.Printf("  %s → %s\n", m.oldPath, m.newPath)
			}
		}
		if len(pageMoves) > 0 {
			fmt.Printf("Untyped pages to move: %d\n", len(pageMoves))
			for _, m := range pageMoves {
				fmt.Printf("  %s → %s\n", m.oldPath, m.newPath)
			}
		}

		if dryRun {
			fmt.Printf("\n%d files would be moved. Run without --dry-run to apply.\n", len(moves))
			return nil
		}

		// Perform the moves
		fmt.Println()
		var moved int
		for _, m := range moves {
			oldFullPath := filepath.Join(vaultPath, m.oldPath)
			newFullPath := filepath.Join(vaultPath, m.newPath)

			// Create destination directory
			if err := os.MkdirAll(filepath.Dir(newFullPath), 0755); err != nil {
				fmt.Printf("  ✗ Failed to create directory for %s: %v\n", m.newPath, err)
				continue
			}

			// Move the file
			if err := os.Rename(oldFullPath, newFullPath); err != nil {
				fmt.Printf("  ✗ Failed to move %s: %v\n", m.oldPath, err)
				continue
			}

			moved++
		}

		fmt.Printf("\n✓ Moved %d files\n", moved)

		// Clean up empty directories
		if err := cleanEmptyDirs(vaultPath, vaultCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean up empty directories: %v\n", err)
		}

		// Suggest reindex
		fmt.Println("\nRun 'rvn reindex --full' to update the index.")

		return nil
	},
}

// cleanEmptyDirs removes empty directories left after migration.
func cleanEmptyDirs(vaultPath string, cfg *config.VaultConfig) error {
	dirs := cfg.GetDirectoriesConfig()
	if dirs == nil {
		return nil
	}

	// Walk and collect empty directories
	var emptyDirs []string
	if err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}
		if !info.IsDir() {
			return nil
		}

		// Skip system directories
		name := info.Name()
		if name == ".raven" || name == ".trash" || name == ".git" {
			return filepath.SkipDir
		}

		// Skip target directories
		relPath, _ := filepath.Rel(vaultPath, path)
		if dirs.Object != "" && strings.HasPrefix(relPath, strings.TrimSuffix(dirs.Object, "/")) {
			return nil
		}
		if dirs.Page != "" && strings.HasPrefix(relPath, strings.TrimSuffix(dirs.Page, "/")) {
			return nil
		}
		if strings.HasPrefix(relPath, cfg.GetDailyDirectory()) {
			return nil
		}

		// Check if directory is empty
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil //nolint:nilerr
		}
		if len(entries) == 0 {
			emptyDirs = append(emptyDirs, path)
		}

		return nil
	}); err != nil {
		return err
	}

	// Remove empty directories (deepest first)
	for i := len(emptyDirs) - 1; i >= 0; i-- {
		if err := os.Remove(emptyDirs[i]); err == nil {
			relPath, _ := filepath.Rel(vaultPath, emptyDirs[i])
			fmt.Printf("  Removed empty directory: %s\n", relPath)
		}
	}
	return nil
}

func init() {
	migrateDirectoriesCmd.Flags().Bool("dry-run", false, "Preview changes without applying")
	migrateCmd.AddCommand(migrateDirectoriesCmd)
}
