package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ravenscroftj/raven/internal/audit"
	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/vault"
	"github.com/spf13/cobra"
)

var deleteForce bool

var deleteCmd = &cobra.Command{
	Use:   "delete <object_id>",
	Short: "Delete an object from the vault",
	Long: `Delete a file/object from the vault.

By default, files are moved to a trash directory (.trash/) within the vault.
This behavior can be changed to permanent deletion via raven.yaml.

The command will warn about any backlinks (objects that reference the deleted item)
and require confirmation unless --force is used.

Examples:
  rvn delete people/alice           # Move people/alice.md to trash
  rvn delete people/alice --force   # Skip confirmation
  rvn delete projects/old-project --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		objectID := args[0]

		// Normalize the object ID (remove .md extension if present)
		objectID = strings.TrimSuffix(objectID, ".md")

		// Resolve the file path (supports slugified matching)
		filePath, err := vault.ResolveObjectToFile(vaultPath, objectID)
		if err != nil {
			return handleErrorMsg(ErrFileNotFound,
				fmt.Sprintf("Object '%s' does not exist", objectID),
				"Check the object ID and try again")
		}

		// Load vault config for deletion settings
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		deletionCfg := vaultCfg.GetDeletionConfig()

		// Find backlinks
		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		backlinks, err := db.Backlinks(objectID)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		// Build response data
		var warnings []Warning
		if len(backlinks) > 0 {
			var backlinkIDs []string
			for _, bl := range backlinks {
				backlinkIDs = append(backlinkIDs, bl.SourceID)
			}
			warnings = append(warnings, Warning{
				Code:    WarnBacklinks,
				Message: fmt.Sprintf("Object is referenced by %d other objects", len(backlinks)),
				Ref:     strings.Join(backlinkIDs, ", "),
			})
		}

		// In JSON mode or with --force, proceed without interactive confirmation
		if !isJSONOutput() && !deleteForce {
			fmt.Printf("Delete %s?\n", objectID)
			if len(backlinks) > 0 {
				fmt.Printf("  ⚠ Warning: Referenced by %d objects:\n", len(backlinks))
				for _, bl := range backlinks {
					line := 0
					if bl.Line != nil {
						line = *bl.Line
					}
					fmt.Printf("    - %s (line %d)\n", bl.SourceID, line)
				}
			}
			fmt.Printf("\nBehavior: %s", deletionCfg.Behavior)
			if deletionCfg.Behavior == "trash" {
				fmt.Printf(" (to %s/)\n", deletionCfg.TrashDir)
			} else {
				fmt.Println()
			}
			fmt.Print("Confirm? [y/N]: ")

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

		// Perform the deletion
		var destPath string
		if deletionCfg.Behavior == "trash" {
			// Move to trash
			trashDir := filepath.Join(vaultPath, deletionCfg.TrashDir)
			if err := os.MkdirAll(trashDir, 0755); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}

			// Preserve directory structure in trash
			relPath := objectID + ".md"
			destPath = filepath.Join(trashDir, relPath)

			// Create parent directories in trash
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}

			// If file already exists in trash, add timestamp
			if _, err := os.Stat(destPath); err == nil {
				timestamp := time.Now().Format("2006-01-02-150405")
				base := strings.TrimSuffix(filepath.Base(destPath), ".md")
				destPath = filepath.Join(filepath.Dir(destPath), base+"-"+timestamp+".md")
			}

			if err := os.Rename(filePath, destPath); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
		} else {
			// Permanent deletion
			if err := os.Remove(filePath); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
		}

		// Remove from index
		if err := db.RemoveDocument(objectID); err != nil {
			// Log but don't fail - file is already deleted
			if !isJSONOutput() {
				fmt.Printf("  (warning: failed to remove from index: %v)\n", err)
			}
		}

		// Log to audit
		auditLogger := audit.New(vaultPath, vaultCfg.IsAuditLogEnabled())
		auditLogger.LogDelete("object", objectID)

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			result := map[string]interface{}{
				"deleted":  objectID,
				"behavior": deletionCfg.Behavior,
			}
			if destPath != "" {
				relDest, _ := filepath.Rel(vaultPath, destPath)
				result["trash_path"] = relDest
			}
			outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		if deletionCfg.Behavior == "trash" {
			relDest, _ := filepath.Rel(vaultPath, destPath)
			fmt.Printf("✓ Moved to %s\n", relDest)
		} else {
			fmt.Printf("✓ Deleted %s\n", objectID)
		}

		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}
