package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var setCmd = newCanonicalLeafCommand("set", canonicalLeafOptions{
	VaultPath: getVaultPath,
	Args:      cobra.ArbitraryArgs,
	BuildArgs: buildSetArgs,
	Invoke:    invokeSet,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		data := canonicalDataMap(result)
		if boolValue(data["bulk"]) || boolValue(data["stdin"]) {
			return renderCanonicalBulkResult(result)
		}
		return renderCanonicalSetSingleResult(result)
	},
})

func buildSetArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	fieldsJSON, _ := cmd.Flags().GetString("fields-json")

	if stdin {
		if strings.TrimSpace(fieldsJSON) != "" {
			return nil, handleErrorMsg(ErrInvalidInput,
				"--fields-json is not supported with --stdin",
				"Use positional field=value updates when using --stdin")
		}
		updates, err := parseSetFieldArgs(args)
		if err != nil {
			return nil, err
		}
		if len(updates) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set --stdin field=value...")
		}

		fileIDs, embeddedIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		ids := append(fileIDs, embeddedIDs...)
		if len(ids) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}

		return map[string]interface{}{
			"stdin":      true,
			"object_ids": stringsToAny(ids),
			"fields":     stringMapToAny(updates),
		}, nil
	}

	if len(args) < 1 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires object-id", "Usage: rvn set <object-id> field=value...")
	}

	objectID := args[0]
	fieldArgs := args[1:]
	updates, err := parseSetFieldArgs(fieldArgs)
	if err != nil {
		return nil, err
	}

	typedUpdates, err := parseFieldValuesJSON(fieldsJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	fieldJSONRaw, err := parseFieldJSONObject(fieldsJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}

	if len(updates) == 0 && len(typedUpdates) == 0 {
		return nil, handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set <object-id> field=value... or --fields-json '{...}'")
	}

	argsMap := map[string]interface{}{
		"object_id": objectID,
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
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   confirm,
	})
}

func renderCanonicalSetSingleResult(result commandexec.Result) error {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	relativePath, _ := data["file"].(string)
	embedded, _ := data["embedded"].(bool)
	if embedded {
		embeddedSlug, _ := data["embedded_slug"].(string)
		fmt.Println(ui.Checkf("Updated %s %s", ui.FilePath(relativePath), ui.Hint("(embedded: "+embeddedSlug+")")))
	} else {
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(relativePath)))
	}

	updatedFields := stringMapFromAny(data["updated_fields"])
	previousFields := interfaceMapFromAny(data["previous_fields"])
	fieldNames := make([]string, 0, len(updatedFields))
	for name := range updatedFields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		oldValue := ""
		if value, ok := previousFields[name]; ok {
			oldValue = fmt.Sprintf("%v", value)
		}
		newValue := updatedFields[name]
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

func interfaceMapFromAny(raw interface{}) map[string]interface{} {
	switch values := raw.(type) {
	case map[string]interface{}:
		return values
	case map[string]string:
		out := make(map[string]interface{}, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out
	default:
		return map[string]interface{}{}
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
