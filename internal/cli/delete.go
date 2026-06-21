package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	deleteForce   bool
	deleteStdin   bool
	deleteConfirm bool
	deleteDryRun  bool
)

var deleteCmd = newCanonicalLeafCommand("delete", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	Args:            cobra.MaximumNArgs(1),
	BuildArgs:       buildDeleteArgs,
	Invoke:          invokeDelete,
	RenderHuman:     renderDeleteResult,
	SkipFlagBinding: true,
})

func buildDeleteArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	if deleteStdin {
		ids, sectionIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		if len(ids) == 0 && len(sectionIDs) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}
		return map[string]interface{}{
			"stdin":      true,
			"object_ids": stringsToAny(append(ids, sectionIDs...)),
		}, nil
	}

	if len(args) == 0 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires object-id argument", "Usage: rvn delete <object-id>")
	}
	return map[string]interface{}{
		"object_id": args[0],
	}, nil
}

func invokeDelete(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Bulk delete stays preview-first: changes apply only with --confirm.
	if boolValue(args["stdin"]) {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   deleteConfirm,
		})
	}

	// Single-object delete applies immediately; --dry-run previews instead.
	if deleteDryRun {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Preview:   true,
		})
	}

	// Non-interactive (JSON) or forced runs apply without prompting.
	if isJSONOutput() || deleteForce {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   true,
		})
	}

	// Interactive terminals still preview and prompt before deleting.
	preview := executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Preview:   true,
	})
	if !preview.OK {
		return preview
	}
	if !renderDeletePreviewPrompt(preview) {
		return commandexec.Success(map[string]interface{}{"cancelled": true}, nil)
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   true,
	})
}

func renderDeleteResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	if boolValue(data["cancelled"]) {
		fmt.Println(ui.Star("Cancelled."))
		return nil
	}
	if boolValue(data["bulk"]) || boolValue(data["stdin"]) {
		return renderCanonicalBulkResult(result)
	}
	if boolValue(data["preview"]) {
		printDeletePreview(data)
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to delete"))
		return nil
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
	printDeletePreview(data)
	return promptForConfirm("Confirm?")
}

func printDeletePreview(data map[string]interface{}) {
	objectID, _ := data["object_id"].(string)
	fmt.Printf("%s %s?\n", ui.SectionHeader("Delete"), ui.Bold.Render(objectID))

	backlinks := deletePreviewBacklinks(data["backlinks"])
	if len(backlinks) > 0 {
		fmt.Printf("%s\n", ui.Warningf("Referenced by %d objects:", len(backlinks)))
		for _, bl := range backlinks {
			line := 0
			if bl.Line != nil {
				line = *bl.Line
			}
			fmt.Println(ui.Indent(2, ui.Bullet(fmt.Sprintf("%s (line %d)", bl.SourceID, line))))
		}
	}

	behavior, _ := data["behavior"].(string)
	fmt.Printf("\n%s %s", ui.Hint("Behavior:"), behavior)
	if behavior == "trash" {
		if trashDir, ok := data["trash_dir"].(string); ok && strings.TrimSpace(trashDir) != "" {
			fmt.Printf(" %s\n", ui.Hint("(to "+trashDir+"/)"))
		} else {
			fmt.Println()
		}
	} else {
		fmt.Println()
	}
}

func deletePreviewBacklinks(raw interface{}) []model.Reference {
	var backlinks []model.Reference
	_ = decodeResultData(raw, &backlinks)
	return backlinks
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	deleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Apply bulk delete (without this flag, bulk shows preview only)")
	deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "Preview a single-object delete without applying it")
	deleteCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(deleteCmd)
}
