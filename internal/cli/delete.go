package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
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
	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	args := map[string]interface{}{
		"stdin":      true,
		"object_ids": stringsToAny(append(ids, embedded...)),
	}
	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "delete",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
		Confirm:   deleteConfirm,
	})
	return handleCanonicalDeleteResult(vaultPath, result, true)
}

// deleteSingleObject deletes a single object (non-bulk mode).
func deleteSingleObject(vaultPath, reference string) error {
	args := map[string]interface{}{
		"object_id": reference,
	}
	if !isJSONOutput() && !deleteForce {
		preview := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "delete",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args:      args,
			Preview:   true,
		})
		if !preview.OK {
			return handleCanonicalDeleteResult(vaultPath, preview, false)
		}
		if !renderDeletePreviewPrompt(preview) {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "delete",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
		Confirm:   true,
	})
	return handleCanonicalDeleteResult(vaultPath, result, false)
}

func handleCanonicalDeleteResult(vaultPath string, result commandexec.Result, bulk bool) error {
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}
	if bulk {
		return renderCanonicalBulkResult(result)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	behavior, _ := data["behavior"].(string)
	if behavior == "trash" {
		if trashPath, ok := data["trash_path"].(string); ok && strings.TrimSpace(trashPath) != "" {
			fmt.Println(ui.Checkf("Moved to %s", ui.FilePath(trashPath)))
			return nil
		}
	}
	if deleted, ok := data["deleted"].(string); ok {
		fmt.Println(ui.Checkf("Deleted %s", ui.FilePath(deleted)))
		return nil
	}
	return handleErrorMsg(ErrInternal, "command execution failed", "")
}

func renderDeletePreviewPrompt(result commandexec.Result) bool {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return false
	}
	objectID, _ := data["object_id"].(string)
	fmt.Printf("Delete %s?\n", objectID)

	backlinks := deletePreviewBacklinks(data["backlinks"])
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

	behavior, _ := data["behavior"].(string)
	fmt.Printf("\nBehavior: %s", behavior)
	if behavior == "trash" {
		if trashDir, ok := data["trash_dir"].(string); ok && strings.TrimSpace(trashDir) != "" {
			fmt.Printf(" (to %s/)\n", trashDir)
		} else {
			fmt.Println()
		}
	} else {
		fmt.Println()
	}

	return promptForConfirm("Confirm?")
}

func deletePreviewBacklinks(raw interface{}) []model.Reference {
	var backlinks []model.Reference
	_ = decodeResultData(raw, &backlinks)
	return backlinks
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	deleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(deleteCmd)
}
