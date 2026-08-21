package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var editCmd = newCanonicalLeafCommand("edit", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalEditFailure,
	Invoke:      invokeEdit,
	RenderHuman: renderEditResult,
})

func handleCanonicalEditFailure(result commandexec.Result) error {
	if result.Error == nil {
		return handleErrorMsg(ErrInternal, "edit failed", "")
	}
	if result.Error.Details != nil {
		return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
	}
	return handleErrorMsg(result.Error.Code, result.Error.Message, result.Error.Suggestion)
}

func invokeEdit(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
}

func renderEditResult(_ *cobra.Command, result commandexec.Result) error {
	return renderCanonicalEditResult(result)
}

func renderCanonicalEditResult(result commandexec.Result) error {
	switch data := result.Data.(type) {
	case commandpayload.EditBatchPreviewResult:
		fmt.Printf("%s %s\n\n", ui.SectionHeader("Preview edits"), ui.FilePath(data.Path))
		for _, edit := range data.Edits {
			fmt.Println(ui.Muted.Render(fmt.Sprintf("EDIT %d (line %d):", edit.Index, edit.Line)))
			fmt.Println(ui.Muted.Render("BEFORE:"))
			fmt.Println(indent(edit.Preview.Before, "  "))
			fmt.Println()
			fmt.Println(ui.Bold.Render("AFTER:"))
			fmt.Println(indent(edit.Preview.After, "  "))
			fmt.Println()
		}
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply this edit"))
		return nil
	case commandpayload.EditSinglePreviewResult:
		fmt.Printf("%s %s\n\n", ui.SectionHeader("Preview edit"), ui.FilePath(fmt.Sprintf("%s:%d", data.Path, data.Line)))
		fmt.Println(ui.Muted.Render("BEFORE:"))
		fmt.Println(indent(data.Preview.Before, "  "))
		fmt.Println()
		fmt.Println(ui.Bold.Render("AFTER:"))
		fmt.Println(indent(data.Preview.After, "  "))
		fmt.Println()
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply this edit"))
		return nil
	case commandpayload.EditBatchResult:
		fmt.Println(ui.Checkf("Applied %d edits in %s", len(data.Edits), ui.FilePath(data.Path)))
		fmt.Println()
		for _, edit := range data.Edits {
			fmt.Println(ui.Muted.Render(fmt.Sprintf("EDIT %d (line %d):", edit.Index, edit.Line)))
			fmt.Println(indent(edit.Context, "  "))
			fmt.Println()
		}
		promptCreateMissingRefsFromResult(getVaultPath(), result)
		return nil
	case commandpayload.EditSingleResult:
		fmt.Println(ui.Checkf("Applied edit in %s", ui.FilePath(fmt.Sprintf("%s:%d", data.Path, data.Line))))
		fmt.Println()
		fmt.Println(ui.Muted.Render("Context:"))
		fmt.Println(indent(data.Context, "  "))
		promptCreateMissingRefsFromResult(getVaultPath(), result)
		return nil
	default:
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
}

func stringMapValue(raw interface{}) map[string]string {
	switch value := raw.(type) {
	case map[string]string:
		return value
	case map[string]interface{}:
		out := make(map[string]string, len(value))
		for key, item := range value {
			if s, ok := item.(string); ok {
				out[key] = s
			}
		}
		return out
	default:
		return map[string]string{}
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func init() {
	editCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(editCmd)
}
