package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/ui"
)

var setCmd = newCanonicalLeafCommand("set", canonicalLeafOptions{
	VaultPath: getVaultPath,
	Args:      cobra.ArbitraryArgs,
	BuildArgs: buildSetArgs,
	Invoke:    invokeSet,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		switch result.Data.(type) {
		case commandpayload.SetBulkPreviewResult, commandpayload.SetBulkResult:
			return renderCanonicalBulkResult(result)
		}
		return renderCanonicalSetSingleResult(result)
	},
})

func buildSetArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	fieldsJSON, _ := cmd.Flags().GetString("fields-json")

	if stdin {
		updates, err := parseSetFieldArgs(args)
		if err != nil {
			return nil, err
		}
		typedUpdates, err := fieldmutation.ParseFieldValuesJSON(fieldsJSON)
		if err != nil {
			return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
		}
		fieldJSONRaw, err := parseFieldJSONObject(fieldsJSON)
		if err != nil {
			return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
		}
		if len(updates) == 0 && len(typedUpdates) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set --stdin field=value... or --fields-json '{...}'")
		}

		fileIDs, sectionIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		ids := append(fileIDs, sectionIDs...)
		if len(ids) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no references provided via stdin", "Pipe references to stdin, one per line")
		}

		argsMap := map[string]interface{}{
			"stdin":      true,
			"references": stringsToAny(ids),
		}
		if len(updates) > 0 {
			argsMap["fields"] = stringMapToAny(updates)
		}
		if len(fieldJSONRaw) > 0 {
			argsMap["fields-json"] = fieldJSONRaw
		}
		return argsMap, nil
	}

	if len(args) < 1 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires reference", "Usage: rvn set <reference> field=value...")
	}

	reference := args[0]
	fieldArgs := args[1:]
	updates, err := parseSetFieldArgs(fieldArgs)
	if err != nil {
		return nil, err
	}

	typedUpdates, err := fieldmutation.ParseFieldValuesJSON(fieldsJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	fieldJSONRaw, err := parseFieldJSONObject(fieldsJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}

	if len(updates) == 0 && len(typedUpdates) == 0 {
		return nil, handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set <reference> field=value... or --fields-json '{...}'")
	}

	argsMap := map[string]interface{}{
		"reference": reference,
	}
	if len(updates) > 0 {
		argsMap["fields"] = stringMapToAny(updates)
	}
	if len(fieldJSONRaw) > 0 {
		argsMap["fields-json"] = fieldJSONRaw
	}
	return argsMap, nil
}

func parseSetFieldArgs(args []string) (map[string]string, error) {
	updates := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("invalid field format: %s", arg),
				"Use format: field=value")
		}
		updates[parts[0]] = parts[1]
	}
	return updates, nil
}

func invokeSet(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm, _ := cmd.Flags().GetBool("confirm")
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		args["dry-run"] = true
	}
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   confirm,
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
