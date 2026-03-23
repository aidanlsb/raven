package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	setStdin      bool
	setConfirm    bool
	setFieldsJSON string
)

var setCmd = &cobra.Command{
	Use:   "set <object-id> <field=value>...",
	Short: "Set frontmatter fields on an object",
	Long: `Set one or more frontmatter fields on an existing object.

The object ID can be a full path (e.g., "people/freya") or a short reference
that uniquely identifies an object. Field values are validated against the
schema if the object has a known type. Unknown fields are rejected.

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn set people/freya email=freya@asgard.realm
  rvn set people/freya name="Freya" email=freya@vanaheim.realm
  rvn set projects/website status=active priority=high
  rvn set projects/website --json

Bulk examples:
  rvn query "object:project .status==active" --ids | rvn set --stdin status=archived
  rvn query "object:project .status==active" --ids | rvn set --stdin status=archived --confirm`,
	Args: cobra.ArbitraryArgs,
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: runSet,
}

func runSet(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()

	if setStdin {
		if strings.TrimSpace(setFieldsJSON) != "" {
			return handleErrorMsg(ErrInvalidInput,
				"--fields-json is not supported with --stdin",
				"Use positional field=value updates when using --stdin")
		}
		updates, err := parseSetFieldArgs(args)
		if err != nil {
			return err
		}
		if len(updates) == 0 {
			return handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set --stdin field=value...")
		}

		fileIDs, embeddedIDs, err := ReadIDsFromStdin()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		ids := append(fileIDs, embeddedIDs...)
		if len(ids) == 0 {
			return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}

		return executeCanonicalSet(vaultPath, map[string]interface{}{
			"stdin":      true,
			"object_ids": stringsToAny(ids),
			"fields":     stringMapToAny(updates),
		}, true)
	}

	if len(args) < 1 {
		return handleErrorMsg(ErrMissingArgument, "requires object-id", "Usage: rvn set <object-id> field=value...")
	}

	objectID := args[0]
	fieldArgs := args[1:]
	updates, err := parseSetFieldArgs(fieldArgs)
	if err != nil {
		return err
	}

	typedUpdates, err := parseFieldValuesJSON(setFieldsJSON)
	if err != nil {
		return handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	fieldJSONRaw, err := parseFieldJSONObject(setFieldsJSON)
	if err != nil {
		return handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}

	if len(updates) == 0 && len(typedUpdates) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set <object-id> field=value... or --fields-json '{...}'")
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
	return executeCanonicalSet(vaultPath, argsMap, false)
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

func executeCanonicalSet(vaultPath string, args map[string]interface{}, bulk bool) error {
	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "set",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
		Confirm:   setConfirm,
	})
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
	return renderCanonicalSetSingleResult(result)
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
	setCmd.Flags().BoolVar(&setStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	setCmd.Flags().BoolVar(&setConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	setCmd.Flags().StringVar(&setFieldsJSON, "fields-json", "", "Set fields via JSON object (typed values)")
	rootCmd.AddCommand(setCmd)
}
