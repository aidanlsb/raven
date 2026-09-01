package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var deleteCmd = newCanonicalLeafCommand("delete", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeDelete,
	RenderHuman: renderDeleteResult,
})

func invokeDelete(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Bulk delete stays preview-first: changes apply only with --confirm.
	// (confirm already in args from buildCanonicalArgsForMeta)
	if boolValue(args["stdin"]) {
		return executeCanonicalCommand(commandID, vaultPath, args)
	}

	// Single-object delete applies immediately; --dry-run previews instead.
	if dryRun {
		previewArgs := make(map[string]interface{}, len(args))
		for k, v := range args {
			previewArgs[k] = v
		}
		delete(previewArgs, "confirm")
		delete(previewArgs, "dry-run")
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      previewArgs,
			Preview:   true,
		})
	}

	// Non-interactive (JSON) or forced runs apply without prompting.
	if isJSONOutput() || force {
		confirmArgs := make(map[string]interface{}, len(args))
		for k, v := range args {
			confirmArgs[k] = v
		}
		confirmArgs["confirm"] = true
		return executeCanonicalCommand(commandID, vaultPath, confirmArgs)
	}

	// Interactive terminals still preview and prompt before deleting.
	previewArgs := make(map[string]interface{}, len(args))
	for k, v := range args {
		previewArgs[k] = v
	}
	delete(previewArgs, "confirm")
	delete(previewArgs, "dry-run")

	preview := executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      previewArgs,
		Preview:   true,
	})
	if !preview.OK {
		return preview
	}
	if !renderDeletePreviewPrompt(preview) {
		return commandexec.Success(commandpayload.CancelledResult{Cancelled: true}, nil)
	}

	confirmArgs := make(map[string]interface{}, len(args))
	for k, v := range args {
		confirmArgs[k] = v
	}
	confirmArgs["confirm"] = true
	return executeCanonicalCommand(commandID, vaultPath, confirmArgs)
}

func renderDeleteResult(_ *cobra.Command, result commandexec.Result) error {
	switch data := result.Data.(type) {
	case commandpayload.CancelledResult:
		fmt.Println(ui.Star("Cancelled."))
		return nil
	case commandpayload.DeleteBulkPreviewResult, commandpayload.DeleteBulkResult:
		return renderCanonicalBulkResult(result)
	case commandpayload.DeletePreviewResult:
		printDeletePreview(data)
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to delete"))
		return nil
	case commandpayload.DeleteResult:
		if data.Behavior == "trash" && strings.TrimSpace(data.TrashPath) != "" {
			renderObjectMoved(data.TrashPath)
			return nil
		}
		renderObjectDeleted(data.Deleted)
		return nil
	default:
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
}

func renderDeletePreviewPrompt(result commandexec.Result) bool {
	data, ok := result.Data.(commandpayload.DeletePreviewResult)
	if !ok {
		return false
	}
	printDeletePreview(data)
	return promptForConfirm("Confirm?")
}

func printDeletePreview(data commandpayload.DeletePreviewResult) {
	fmt.Printf("%s %s?\n", ui.SectionHeader("Delete"), ui.Bold.Render(data.ObjectID))

	if len(data.Backlinks) > 0 {
		fmt.Printf("%s\n", ui.Warningf("Referenced by %d objects:", len(data.Backlinks)))
		for _, bl := range data.Backlinks {
			line := 0
			if bl.Line != nil {
				line = *bl.Line
			}
			fmt.Println(ui.Indent(2, ui.Bullet(fmt.Sprintf("%s (line %d)", bl.SourceID, line))))
		}
	}

	fmt.Printf("\n%s %s", ui.Hint("Behavior:"), data.Behavior)
	if data.Behavior == "trash" {
		if strings.TrimSpace(data.TrashDir) != "" {
			fmt.Printf(" %s\n", ui.Hint("(to "+data.TrashDir+"/)"))
		} else {
			fmt.Println()
		}
	} else {
		fmt.Println()
	}
}

func init() {
	deleteCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(deleteCmd)
}
