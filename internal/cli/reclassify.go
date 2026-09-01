package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
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

type ReclassifyResult = commandpayload.ReclassifyResult

var reclassifyCmd = newCanonicalLeafCommand("reclassify", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeReclassify,
	RenderHuman: renderReclassifyResult,
})

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

		data, typed := result.Data.(commandpayload.ReclassifyResult)
		if typed && data.NeedsConfirm && !boolValue(fieldValues["force"]) {
			if isJSONOutput() {
				return result
			}
			fmt.Fprintf(os.Stderr, "The following fields are not defined on type '%s' and will be dropped:\n", stringValue(fieldValues["new-type"]))
			for _, f := range data.DroppedFields {
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
				return commandexec.Success(commandpayload.CancelledResult{Cancelled: true}, nil)
			}
			fieldValues["force"] = true
			continue
		}
		return result
	}
}

func renderReclassifyResult(_ *cobra.Command, result commandexec.Result) error {
	switch data := result.Data.(type) {
	case commandpayload.ReclassifyBulkPreviewResult, commandpayload.ReclassifyBulkResult:
		return renderCanonicalBulkResult(result)
	case commandpayload.CancelledResult:
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	case commandpayload.ReclassifyResult:
		fmt.Println(ui.Checkf("Reclassified %s: %s → %s", ui.FilePath(data.File), data.OldType, data.NewType))
		if len(data.AddedFields) > 0 {
			fmt.Printf("  %s\n", ui.Hint("Added fields: "+strings.Join(data.AddedFields, ", ")))
		}
		if len(data.DroppedFields) > 0 {
			fmt.Printf("  %s\n", ui.Hint("Dropped fields: "+strings.Join(data.DroppedFields, ", ")))
		}
		if data.Moved {
			fmt.Printf("  %s %s → %s\n", ui.Hint("Moved:"), ui.FilePath(data.OldPath), ui.FilePath(data.NewPath))
		}
		if len(data.UpdatedRefs) > 0 {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Updated %d references", len(data.UpdatedRefs))))
		}
	default:
		return handleErrorMsg(ErrInternal, "command execution failed", "")
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
