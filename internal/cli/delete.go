package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	deleteForce   bool
	deleteStdin   bool
	deleteConfirm bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete <object_id>",
	Short: "Delete an object from the vault",
	Long: `Delete a file/object from the vault.

By default, files are moved to a trash directory (.trash/) within the vault.
This behavior can be changed to permanent deletion via raven.yaml.

The command will warn about any backlinks (objects that reference the deleted item)
and require confirmation unless --force is used.

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn delete people/loki           # Move people/loki.md to trash
  rvn delete people/loki --force   # Skip confirmation
  rvn delete projects/old-project --json

Bulk examples:
  rvn query "object:project .status:archived" --ids | rvn delete --stdin
  rvn query "object:project .status:archived" --ids | rvn delete --stdin --confirm`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --stdin mode for bulk operations
		if deleteStdin {
			return runDeleteBulk(vaultPath)
		}

		// Single object mode - requires object-id
		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "requires object-id argument", "Usage: rvn delete <object-id>")
		}

		return deleteSingleObject(vaultPath, args[0])
	},
}

// runDeleteBulk handles bulk delete operations from stdin.
func runDeleteBulk(vaultPath string) error {
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
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	// If not confirming, show preview
	if !deleteConfirm {
		return previewDeleteBulk(vaultPath, ids, warnings, vaultCfg)
	}

	// Apply the deletions
	return applyDeleteBulk(vaultPath, ids, warnings, vaultCfg)
}

// previewDeleteBulk shows a preview of bulk delete operations.
func previewDeleteBulk(vaultPath string, ids []string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	var previewItems []BulkPreviewItem
	var skipped []BulkResult

	deletionCfg := vaultCfg.GetDeletionConfig()

	// Open database for backlink checks
	db, err := index.Open(vaultPath)
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	for _, id := range ids {
		objectID := vaultCfg.FilePathToObjectID(id)
		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "object not found",
			})
			continue
		}

		// Check for backlinks
		backlinks, _ := db.Backlinks(objectID)
		details := ""
		if len(backlinks) > 0 {
			details = fmt.Sprintf("⚠ referenced by %d objects", len(backlinks))
		}

		item := BulkPreviewItem{
			ID:      id,
			Action:  "delete",
			Details: details,
		}
		if deletionCfg.Behavior == "trash" {
			item.Changes = map[string]string{"behavior": fmt.Sprintf("move to %s/", deletionCfg.TrashDir)}
		} else {
			item.Changes = map[string]string{"behavior": "permanent deletion"}
		}

		// Verify file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "file not found",
			})
			continue
		}

		previewItems = append(previewItems, item)
	}

	preview := &BulkPreview{
		Action:   "delete",
		Items:    previewItems,
		Skipped:  skipped,
		Total:    len(ids),
		Warnings: warnings,
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"preview":  true,
			"action":   "delete",
			"behavior": deletionCfg.Behavior,
			"items":    previewItems,
			"skipped":  skipped,
			"total":    len(ids),
			"warnings": warnings,
		}, &Meta{Count: len(previewItems)})
		return nil
	}

	PrintBulkPreview(preview)
	return nil
}

// applyDeleteBulk applies bulk delete operations.
func applyDeleteBulk(vaultPath string, ids []string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	var results []BulkResult
	deleted := 0
	skipped := 0
	errors := 0

	deletionCfg := vaultCfg.GetDeletionConfig()

	// Open database for cleanup
	db, err := index.Open(vaultPath)
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	for _, id := range ids {
		result := BulkResult{ID: id}

		// Canonicalize the object ID, but resolve the file using the original input
		// (it may already include a rooted path).
		objectID := vaultCfg.FilePathToObjectID(id)
		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skipped++
			results = append(results, result)
			continue
		}

		// Perform the deletion
		if deletionCfg.Behavior == "trash" {
			// Move to trash
			trashDir := filepath.Join(vaultPath, deletionCfg.TrashDir)
			if err := os.MkdirAll(trashDir, 0755); err != nil {
				result.Status = "error"
				result.Reason = fmt.Sprintf("failed to create trash dir: %v", err)
				errors++
				results = append(results, result)
				continue
			}

			// Preserve the file's actual directory structure in trash.
			relPath, _ := filepath.Rel(vaultPath, filePath)
			destPath := filepath.Join(trashDir, relPath)

			// Create parent directories in trash
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				result.Status = "error"
				result.Reason = fmt.Sprintf("failed to create parent dirs: %v", err)
				errors++
				results = append(results, result)
				continue
			}

			// If file already exists in trash, add timestamp
			if _, err := os.Stat(destPath); err == nil {
				timestamp := time.Now().Format("2006-01-02-150405")
				base := strings.TrimSuffix(filepath.Base(destPath), ".md")
				destPath = filepath.Join(filepath.Dir(destPath), base+"-"+timestamp+".md")
			}

			if err := os.Rename(filePath, destPath); err != nil {
				result.Status = "error"
				result.Reason = fmt.Sprintf("move failed: %v", err)
				errors++
				results = append(results, result)
				continue
			}
		} else {
			// Permanent deletion
			if err := os.Remove(filePath); err != nil {
				result.Status = "error"
				result.Reason = fmt.Sprintf("delete failed: %v", err)
				errors++
				results = append(results, result)
				continue
			}
		}

		// Remove from index
		db.RemoveDocument(objectID)

		result.Status = "deleted"
		deleted++
		results = append(results, result)
	}

	summary := &BulkSummary{
		Action:  "delete",
		Results: results,
		Total:   len(ids),
		Deleted: deleted,
		Skipped: skipped,
		Errors:  errors,
	}

	if isJSONOutput() {
		data := map[string]interface{}{
			"ok":       errors == 0,
			"action":   "delete",
			"behavior": deletionCfg.Behavior,
			"results":  results,
			"total":    len(ids),
			"deleted":  deleted,
			"skipped":  skipped,
			"errors":   errors,
		}
		if len(warnings) > 0 {
			outputSuccessWithWarnings(data, warnings, &Meta{Count: deleted})
		} else {
			outputSuccess(data, &Meta{Count: deleted})
		}
		return nil
	}

	PrintBulkSummary(summary)
	for _, w := range warnings {
		fmt.Printf("⚠ %s\n", w.Message)
	}
	return nil
}

// deleteSingleObject deletes a single object (non-bulk mode).
func deleteSingleObject(vaultPath, reference string) error {
	start := time.Now()

	// Load vault config for deletion settings and directory roots
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		// Config is optional; fall back to defaults.
		vaultCfg = &config.VaultConfig{}
	}

	// Resolve the reference using unified resolver
	result, err := ResolveReference(reference, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return handleResolveError(err, reference)
	}

	objectID := result.ObjectID
	filePath := result.FilePath
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

		// Preserve the file's actual directory structure in trash.
		relPath, _ := filepath.Rel(vaultPath, filePath)
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
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	deleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(deleteCmd)
}
