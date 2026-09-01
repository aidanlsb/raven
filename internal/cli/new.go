package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	newFieldFlags []string
	newFieldJSON  string
	newObjectPath string
	newTemplate   string
)

var newCmd = newCanonicalLeafCommand("new", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Prepare:     prepareNewArgs,
	Invoke:      invokeNew,
	RenderHuman: renderNewResult,
	FlagBindings: map[string]interface{}{
		"field":       &newFieldFlags,
		"fields-json": &newFieldJSON,
		"object-path": &newObjectPath,
		"template":    &newTemplate,
	},
})

func prepareNewArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	// If title is missing and not JSON mode, prompt for it
	if len(args) < 2 && !isJSONOutput() {
		reader := bufio.NewReader(os.Stdin)
		fmt.Fprintf(os.Stderr, "Title: ")
		value, err := reader.ReadString('\n')
		if err != nil {
			return nil, false, fmt.Errorf("failed to read input: %w", err)
		}
		title := strings.TrimSpace(value)
		if title == "" {
			return nil, false, handleErrorMsg(ErrMissingArgument, "title cannot be empty", "")
		}
		args = append(args, title)
	}
	return args, false, nil
}

func invokeNew(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Validate title
	title := stringValue(args["title"])
	if err := validateObjectTitle(title); err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Provide a non-empty title")
	}

	// Validate object path if provided
	targetPath := stringValue(args["object-path"])
	if targetPath != "" {
		if err := validateObjectPath(targetPath); err != nil {
			return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --object-path with an object path like note/raven-friction (no type/ prefix, no .md suffix)")
		}
	}

	// Interactive field prompts in non-JSON mode
	if !isJSONOutput() {
		// Convert field map[string]interface{} to map[string]string for prompting
		fieldValuesRaw, _ := args["field"].(map[string]interface{})
		fieldValues := make(map[string]string)
		for k, v := range fieldValuesRaw {
			if s, ok := v.(string); ok {
				fieldValues[k] = s
			}
		}
		
		fieldJSONRaw, _ := args["fields-json"].(map[string]interface{})
		if fieldJSONRaw == nil {
			fieldJSONRaw = make(map[string]interface{})
		}
		
		reader := bufio.NewReader(os.Stdin)
		typeName := stringValue(args["type"])
		if err := promptNewSchemaFields(reader, os.Stderr, vaultPath, typeName, title, fieldValues, fieldJSONRaw); err != nil {
			return commandexec.Failure("SCHEMA_INVALID", err.Error(), nil, "")
		}
		
		// Update args with prompted values
		if len(fieldValues) > 0 {
			args["field"] = stringMapToAny(fieldValues)
		}
		if len(fieldJSONRaw) > 0 {
			args["fields-json"] = fieldJSONRaw
		}
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		result := executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
		})
		if !result.OK {
			if !isJSONOutput() && result.Error != nil && result.Error.Code == ErrRequiredFieldMissing {
				fieldValues := map[string]string{}
				for key, value := range stringMapValue(args["field"]) {
					fieldValues[key] = value
				}
				prompted := false
				for _, fieldName := range missingFieldNamesFromDetails(result.Error.Details) {
					if _, exists := fieldValues[fieldName]; exists {
						continue
					}
					fmt.Fprintf(os.Stderr, "%s (required): ", fieldName)
					value, readErr := reader.ReadString('\n')
					if readErr != nil {
						return commandexec.Failure(ErrInternal, fmt.Sprintf("failed to read input: %v", readErr), nil, "")
					}
					value = strings.TrimSpace(value)
					if value == "" {
						return commandexec.Failure(ErrRequiredFieldMissing, fmt.Sprintf("required field '%s' cannot be empty", fieldName), nil, "Provide a non-empty value")
					}
					fieldValues[fieldName] = value
					prompted = true
				}
				if prompted {
					args["field"] = stringMapToAny(fieldValues)
					continue
				}
			}
			return result
		}
		return result
	}
}

func promptNewSchemaFields(reader *bufio.Reader, writer io.Writer, vaultPath, typeName, title string, fieldValues map[string]string, fieldJSONRaw map[string]interface{}) error {
	sch, err := loadSchemaSafe(vaultPath)
	if err != nil {
		return handleErrorMsg(ErrSchemaInvalid, "failed to load schema", "Fix schema.yaml and try again")
	}
	typeDef := sch.Types[strings.TrimSpace(typeName)]
	if typeDef == nil || len(typeDef.Fields) == 0 {
		return nil
	}

	provided := make(map[string]bool, len(fieldValues)+len(fieldJSONRaw)+1)
	for key := range fieldValues {
		provided[key] = true
	}
	for key := range fieldJSONRaw {
		provided[key] = true
	}
	if typeDef.NameField != "" && strings.TrimSpace(title) != "" {
		provided[typeDef.NameField] = true
	}

	fieldNames := make([]string, 0, len(typeDef.Fields))
	for fieldName := range typeDef.Fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)

	for _, fieldName := range fieldNames {
		fieldDef := typeDef.Fields[fieldName]
		if fieldDef == nil || provided[fieldName] {
			continue
		}

		fmt.Fprint(writer, schemaFieldPromptText(fieldName, fieldDef))
		value, readErr := reader.ReadString('\n')
		if readErr != nil {
			return fmt.Errorf("failed to read input: %w", readErr)
		}
		resolved, set, resolveErr := resolveInteractiveSchemaFieldInput(fieldName, fieldDef, value)
		if resolveErr != nil {
			return resolveErr
		}
		if !set {
			continue
		}
		fieldValues[fieldName] = resolved
		provided[fieldName] = true
	}

	return nil
}

func renderNewResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.NewResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	fmt.Println(ui.Checkf("Created %s", ui.FilePath(data.File)))
	if data.ID != "" {
		fmt.Println(ui.LinkAs(data.ID))
	}
	promptCreateMissingRefsFromResult(getVaultPath(), result)
	vault.OpenInEditorOrPrintPath(getConfig(), filepath.Join(getVaultPath(), filepath.FromSlash(data.File)))
	return nil
}

func missingFieldNamesFromDetails(details interface{}) []string {
	detailMap, ok := details.(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := detailMap["missing_fields"]
	if !ok {
		return nil
	}

	items, ok := raw.([]map[string]interface{})
	if ok {
		names := make([]string, 0, len(items))
		for _, item := range items {
			if name, ok := item["name"].(string); ok && strings.TrimSpace(name) != "" {
				names = append(names, name)
			}
		}
		return names
	}

	rawItems, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	names := make([]string, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := item["name"].(string); ok && strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	return names
}

func stringMapToAny(values map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// completeTypes provides shell completion for type names
func completeTypes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first argument (type)
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Try to load schema for dynamic completion
	vaultPath := getVaultPath()
	if vaultPath == "" {
		// Fall back to built-in types only
		return schema.BuiltinTypeNames(), cobra.ShellCompDirectiveNoFileComp
	}

	s, err := loadSchemaSafe(vaultPath)
	if err != nil {
		return schema.BuiltinTypeNames(), cobra.ShellCompDirectiveNoFileComp
	}

	// Collect all type names
	var types []string
	for name := range s.Types {
		types = append(types, name)
	}
	// Add built-in types
	types = append(types, schema.BuiltinTypeNames()...)

	sort.Strings(types)
	return types, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	newCmd.ValidArgsFunction = completeTypes
	rootCmd.AddCommand(newCmd)
}
