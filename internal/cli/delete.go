package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	deleteForce   bool
	deleteStdin   bool
	deleteConfirm bool
	deleteDryRun  bool
)

var deleteCmd = newCanonicalLeafCommand("delete", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeDelete,
	RenderHuman: renderDeleteResult,
	FlagBindings: map[string]interface{}{
		"force":   &deleteForce,
		"stdin":   &deleteStdin,
		"confirm": &deleteConfirm,
		"dry-run": &deleteDryRun,
	},
})


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
		return commandexec.Success(commandpayload.CancelledResult{Cancelled: true}, nil)
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   true,
	})
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
			fmt.Println(ui.Checkf("Moved to %s", ui.FilePath(data.TrashPath)))
			return nil
		}
		fmt.Println(ui.Checkf("Deleted %s", ui.FilePath(data.Deleted)))
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
