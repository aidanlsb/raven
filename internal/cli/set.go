package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/ui"
)

var setCmd = newCanonicalLeafCommand("set", canonicalLeafOptions{
	VaultPath: getVaultPath,
	Invoke:    invokeSet,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		switch result.Data.(type) {
		case commandpayload.SetBulkPreviewResult, commandpayload.SetBulkResult:
			return renderCanonicalBulkResult(result)
		}
		return renderCanonicalSetSingleResult(result)
	},
})

func invokeSet(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm, _ := cmd.Flags().GetBool("confirm")
	preview := false
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		preview = true
	}
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   confirm,
		Preview:   preview,
	})
}

func renderCanonicalSetSingleResult(result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.SetResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if data.Preview {
		fmt.Println(ui.Star(fmt.Sprintf("Would update %s", ui.FilePath(data.File))))
	} else {
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(data.File)))
	}

	fieldNames := make([]string, 0, len(data.UpdatedFields))
	for name := range data.UpdatedFields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		oldValue := ""
		if value, ok := data.PreviousFields[name]; ok {
			oldValue = fieldmutation.SerializeFieldValueLiteral(value)
		}
		newValue := data.UpdatedFields[name]
		if oldValue != "" && oldValue != newValue {
			fmt.Printf("  %s\n", ui.FieldChange(name, oldValue, newValue))
		} else if oldValue == "" {
			fmt.Printf("  %s\n", ui.FieldAdd(name, newValue))
		} else {
			fmt.Printf("  %s\n", ui.FieldSet(name, newValue))
		}
	}
	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warning(warning.Message))
	}
	if data.Preview {
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}
	promptCreateMissingRefsFromResult(getVaultPath(), result)
	return nil
}

func stringMapFromAny(raw interface{}) map[string]string {
	switch values := raw.(type) {
	case map[string]string:
		return values
	case map[string]interface{}:
		out := make(map[string]string, len(values))
		for key, value := range values {
			out[key] = fmt.Sprintf("%v", value)
		}
		return out
	default:
		return map[string]string{}
	}
}

func init() {
	setCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(setCmd)
}
