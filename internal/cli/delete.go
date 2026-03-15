package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/ui"
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
  rvn query "object:project .status==archived" --ids | rvn delete --stdin
  rvn query "object:project .status==archived" --ids | rvn delete --stdin --confirm`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
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
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
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
	deletionCfg := vaultCfg.GetDeletionConfig()
	previewResult, err := objectsvc.PreviewDeleteBulk(objectsvc.DeleteBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		ObjectIDs:   ids,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}

	preview := &BulkPreview{
		Action:   "delete",
		Items:    make([]BulkPreviewItem, 0, len(previewResult.Items)),
		Skipped:  make([]BulkResult, 0, len(previewResult.Skipped)),
		Total:    previewResult.Total,
		Warnings: warnings,
	}
	for _, item := range previewResult.Items {
		preview.Items = append(preview.Items, BulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
			Changes: item.Changes,
		})
	}
	for _, skip := range previewResult.Skipped {
		preview.Skipped = append(preview.Skipped, BulkResult{
			ID:     skip.ID,
			Status: skip.Status,
			Reason: skip.Reason,
		})
	}

	return outputBulkPreview(preview, map[string]interface{}{
		"behavior": deletionCfg.Behavior,
	})
}

// applyDeleteBulk applies bulk delete operations.
func applyDeleteBulk(vaultPath string, ids []string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	deletionCfg := vaultCfg.GetDeletionConfig()
	summaryResult, err := objectsvc.ApplyDeleteBulk(objectsvc.DeleteBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		ObjectIDs:   ids,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}

	results := make([]BulkResult, 0, len(summaryResult.Results))
	for _, result := range summaryResult.Results {
		results = append(results, BulkResult{
			ID:     result.ID,
			Status: result.Status,
			Reason: result.Reason,
		})
	}

	combinedWarnings := append([]Warning{}, warnings...)
	for _, message := range summaryResult.WarningMessages {
		combinedWarnings = append(combinedWarnings, Warning{
			Code:    WarnIndexUpdateFailed,
			Message: message,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	summary := buildBulkSummary("delete", results, combinedWarnings)
	return outputBulkSummary(summary, combinedWarnings, map[string]interface{}{
		"behavior": deletionCfg.Behavior,
	})
}

// deleteSingleObject deletes a single object (non-bulk mode).
func deleteSingleObject(vaultPath, reference string) error {
	start := time.Now()

	// Load vault config for deletion settings and directory roots
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	deletionCfg := vaultCfg.GetDeletionConfig()
	preview, err := objectsvc.PreviewDeleteByReference(objectsvc.DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   reference,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return mapDeleteSingleServiceError(err)
	}

	// Build response data
	warnings := deleteBacklinkWarnings(preview.Backlinks)

	// In JSON mode or with --force, proceed without interactive confirmation
	if !isJSONOutput() && !deleteForce {
		fmt.Fprintf(os.Stderr, "Delete %s?\n", preview.ObjectID)
		if len(preview.Backlinks) > 0 {
			fmt.Fprintf(os.Stderr, "  ⚠ Warning: Referenced by %d objects:\n", len(preview.Backlinks))
			for _, bl := range preview.Backlinks {
				line := 0
				if bl.Line != nil {
					line = *bl.Line
				}
				fmt.Fprintf(os.Stderr, "    - %s (line %d)\n", bl.SourceID, line)
			}
		}
		fmt.Fprintf(os.Stderr, "\nBehavior: %s", deletionCfg.Behavior)
		if deletionCfg.Behavior == "trash" {
			fmt.Fprintf(os.Stderr, " (to %s/)\n", deletionCfg.TrashDir)
		} else {
			fmt.Fprintln(os.Stderr)
		}
		fmt.Fprint(os.Stderr, "Confirm? [y/N]: ")

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

	// Perform the deletion
	serviceResult, err := objectsvc.DeleteByReference(objectsvc.DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   reference,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return mapDeleteSingleServiceError(err)
	}
	for _, warningMsg := range serviceResult.WarningMessages {
		warnings = append(warnings, Warning{
			Code:    WarnIndexUpdateFailed,
			Message: warningMsg,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		payload := map[string]interface{}{
			"deleted":  serviceResult.ObjectID,
			"behavior": serviceResult.Behavior,
		}
		if serviceResult.TrashPath != "" {
			relDest, _ := filepath.Rel(vaultPath, serviceResult.TrashPath)
			payload["trash_path"] = relDest
		}
		outputSuccessWithWarnings(payload, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	if serviceResult.Behavior == "trash" {
		relDest, _ := filepath.Rel(vaultPath, serviceResult.TrashPath)
		fmt.Println(ui.Checkf("Moved to %s", ui.FilePath(relDest)))
	} else {
		fmt.Println(ui.Checkf("Deleted %s", ui.FilePath(serviceResult.ObjectID)))
	}

	return nil
}

func deleteBacklinkWarnings(backlinks []model.Reference) []Warning {
	if len(backlinks) == 0 {
		return nil
	}
	backlinkIDs := make([]string, 0, len(backlinks))
	for _, bl := range backlinks {
		backlinkIDs = append(backlinkIDs, bl.SourceID)
	}
	return []Warning{{
		Code:    WarnBacklinks,
		Message: fmt.Sprintf("Object is referenced by %d other objects", len(backlinks)),
		Ref:     strings.Join(backlinkIDs, ", "),
	}}
}

func mapDeleteSingleServiceError(err error) error {
	var svcErr *objectsvc.Error
	if !errors.As(err, &svcErr) {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case objectsvc.ErrorRefNotFound:
		return handleErrorMsg(ErrRefNotFound, svcErr.Message, svcErr.Suggestion)
	case objectsvc.ErrorRefAmbiguous:
		return handleErrorMsg(ErrRefAmbiguous, svcErr.Message, svcErr.Suggestion)
	case objectsvc.ErrorDatabase:
		return handleError(ErrDatabaseError, svcErr, svcErr.Suggestion)
	case objectsvc.ErrorInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case objectsvc.ErrorFileWrite:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	deleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(deleteCmd)
}
