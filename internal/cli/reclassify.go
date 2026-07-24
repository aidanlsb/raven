package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	reclassifyFieldFlags []string
	reclassifyFieldJSON  string
	reclassifyNoMove     bool
	reclassifyUpdateRefs bool
	reclassifyForce      bool
	reclassifyStdin      bool
	reclassifyConfirm    bool
)

var reclassifyCmd = newCanonicalLeafCommand("reclassify", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.MaximumNArgs(2),
	BuildArgs:   buildReclassifyArgs,
	Invoke:      invokeReclassify,
	RenderHuman: renderReclassifyResult,
	FlagBindings: map[string]interface{}{
		"field":       &reclassifyFieldFlags,
		"fields-json": &reclassifyFieldJSON,
		"no-move":     &reclassifyNoMove,
		"update-refs": &reclassifyUpdateRefs,
		"force":       &reclassifyForce,
		"stdin":       &reclassifyStdin,
		"confirm":     &reclassifyConfirm,
	},
})

type ReclassifyResult = objectsvc.ReclassifyResult

func buildReclassifyArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	fieldJSONRaw, err := parseFieldJSONObject(reclassifyFieldJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	parsedFieldFlags, err := parseKeyValueArgs("field", reclassifyFieldFlags)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
	}
	fieldFlags := make(map[string]string, len(parsedFieldFlags))
	for key, value := range parsedFieldFlags {
		text, _ := value.(string)
		fieldFlags[key] = text
	}

	argsMap := map[string]interface{}{
		"field":       fieldFlags,
		"no-move":     reclassifyNoMove,
		"update-refs": reclassifyUpdateRefs,
		"force":       reclassifyForce,
	}
	if len(fieldJSONRaw) > 0 {
		argsMap["fields-json"] = fieldJSONRaw
	}

	if reclassifyStdin {
		if len(args) != 1 {
			return nil, handleErrorMsg(ErrMissingArgument, "requires one target type with --stdin", "Usage: rvn reclassify <new-type> --stdin")
		}
		ids, sectionIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		ids = append(ids, sectionIDs...)
		if len(ids) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}
		argsMap["stdin"] = true
		argsMap["object_ids"] = stringsToAny(ids)
		argsMap["new-type"] = args[0]
		return argsMap, nil
	}

	if len(args) != 2 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires object and target type arguments", "Usage: rvn reclassify <object> <new-type>")
	}
	argsMap["object"] = args[0]
	argsMap["new-type"] = args[1]
	return argsMap, nil
}

func invokeReclassify(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	if boolValue(args["stdin"]) {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   reclassifyConfirm,
		})
	}

	fieldValues := cloneArgsMap(args)
	for {
		result := executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      fieldValues,
		})
		if !result.OK {
			if !isJSONOutput() && result.Error != nil && result.Error.Code == ErrRequiredFieldMissing {
				details, _ := result.Error.Details.(map[string]interface{})
				prompted, promptErr := promptMissingReclassifyFields(stringValue(fieldValues["new-type"]), details)
				if promptErr != nil {
					return commandexec.Failure(ErrInternal, promptErr.Error(), nil, "")
				}
				fields, _ := fieldValues["field"].(map[string]string)
				if fields == nil {
					fields = map[string]string{}
				}
				for k, v := range prompted {
					fields[k] = v
				}
				fieldValues["field"] = fields
				continue
			}
			return result
		}

		data := canonicalDataMap(result)
		if boolValue(data["needs_confirm"]) && !boolValue(fieldValues["force"]) {
			if isJSONOutput() {
				return result
			}
			fmt.Fprintf(os.Stderr, "The following fields are not defined on type '%s' and will be dropped:\n", stringValue(fieldValues["new-type"]))
			for _, f := range stringSliceFromAny(data["dropped_fields"]) {
				fmt.Fprintf(os.Stderr, "  - %s\n", f)
			}
			fmt.Fprint(os.Stderr, "\nProceed? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			response, readErr := reader.ReadString('\n')
			if readErr != nil {
				return commandexec.Failure(ErrInternal, readErr.Error(), nil, "")
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				return commandexec.Success(map[string]interface{}{"cancelled": true}, nil)
			}
			fieldValues["force"] = true
			continue
		}
		return result
	}
}

func renderReclassifyResult(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if stringValue(data["action"]) == "reclassify" {
		return renderCanonicalBulkResult(result)
	}
	if boolValue(data["cancelled"]) {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}
	fmt.Println(ui.Checkf("Reclassified %s: %s → %s", ui.FilePath(stringValue(data["file"])), stringValue(data["old_type"]), stringValue(data["new_type"])))
	if added := stringSliceFromAny(data["added_fields"]); len(added) > 0 {
		fmt.Printf("  %s\n", ui.Hint("Added fields: "+strings.Join(added, ", ")))
	}
	if dropped := stringSliceFromAny(data["dropped_fields"]); len(dropped) > 0 {
		fmt.Printf("  %s\n", ui.Hint("Dropped fields: "+strings.Join(dropped, ", ")))
	}
	if boolValue(data["moved"]) {
		fmt.Printf("  %s %s → %s\n", ui.Hint("Moved:"), ui.FilePath(stringValue(data["old_path"])), ui.FilePath(stringValue(data["new_path"])))
	}
	if updatedRefs := stringSliceFromAny(data["updated_refs"]); len(updatedRefs) > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Updated %d references", len(updatedRefs))))
	}
	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warning(warning.Message))
	}
	return nil
}

func promptMissingReclassifyFields(newTypeName string, details map[string]interface{}) (map[string]string, error) {
	rawMissing, ok := details["missing_fields"]
	if !ok {
		return nil, handleErrorMsg(ErrRequiredFieldMissing, fmt.Sprintf("Missing required fields for type '%s'", newTypeName), "Provide required fields with --field")
	}

	entries, ok := rawMissing.([]map[string]interface{})
	if !ok {
		if genericEntries, ok2 := rawMissing.([]interface{}); ok2 {
			entries = make([]map[string]interface{}, 0, len(genericEntries))
			for _, entry := range genericEntries {
				if m, ok3 := entry.(map[string]interface{}); ok3 {
					entries = append(entries, m)
				}
			}
		}
	}

	fieldNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		name = strings.TrimSpace(name)
		if name != "" {
			fieldNames = append(fieldNames, name)
		}
	}
	if len(fieldNames) == 0 {
		return nil, handleErrorMsg(ErrRequiredFieldMissing, fmt.Sprintf("Missing required fields for type '%s'", newTypeName), "Provide required fields with --field")
	}

	sort.Strings(fieldNames)
	reader := bufio.NewReader(os.Stdin)
	values := make(map[string]string, len(fieldNames))

	for _, fieldName := range fieldNames {
		fmt.Fprintf(os.Stderr, "%s (required for type '%s'): ", fieldName, newTypeName)
		value, err := reader.ReadString('\n')
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, handleErrorMsg(ErrRequiredFieldMissing, fmt.Sprintf("required field '%s' cannot be empty", fieldName), "Provide a non-empty value")
		}
		values[fieldName] = value
	}

	return values, nil
}

func init() {
	reclassifyCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(reclassifyCmd)
}
