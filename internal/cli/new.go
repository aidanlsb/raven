package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	newFieldFlags []string
	newFieldJSON  string
	newPathFlag   string
	newTemplate   string
)

var newCmd = newCanonicalLeafCommand("new", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	Args:            cobra.RangeArgs(1, 2),
	BuildArgs:       buildNewArgs,
	Invoke:          invokeNew,
	RenderHuman:     renderNewResult,
	SkipFlagBinding: true,
})

func buildNewArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	typeName := args[0]
	title := ""
	if len(args) >= 2 {
		title = args[1]
	} else if isJSONOutput() {
		return nil, handleErrorMsg(ErrMissingArgument, "title is required", "Usage: rvn new <type> <title> --json")
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Fprintf(os.Stderr, "Title: ")
		value, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}
		title = strings.TrimSpace(value)
		if title == "" {
			return nil, handleErrorMsg(ErrMissingArgument, "title cannot be empty", "")
		}
	}

	if err := validateObjectTitle(title); err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Provide a plain title without path separators")
	}
	fieldValues, err := parseFieldFlags(newFieldFlags)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
	}
	fieldJSONRaw, err := parseFieldJSONObject(newFieldJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
	}
	targetPath := strings.TrimSpace(newPathFlag)
	if targetPath != "" {
		if err := validateObjectTargetPath(targetPath); err != nil {
			return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Use --path with an explicit object path like note/raven-friction")
		}
	}
	return buildNewCommandArgs(typeName, title, targetPath, newTemplate, fieldValues, fieldJSONRaw), nil
}

func invokeNew(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
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

func renderNewResult(_ *cobra.Command, result commandexec.Result) error {
	data, _ := result.Data.(map[string]interface{})
	relativePath, _ := data["file"].(string)
	fmt.Println(ui.Checkf("Created %s", ui.FilePath(relativePath)))
	vault.OpenInEditorOrPrintPath(getConfig(), filepath.Join(getVaultPath(), filepath.FromSlash(relativePath)))
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

func buildNewCommandArgs(typeName, title, targetPath, templateID string, fieldValues map[string]string, fieldJSONRaw map[string]interface{}) map[string]interface{} {
	args := map[string]interface{}{
		"type":  typeName,
		"title": title,
	}
	if len(fieldValues) > 0 {
		args["field"] = stringMapToAny(fieldValues)
	}
	if len(fieldJSONRaw) > 0 {
		args["field-json"] = fieldJSONRaw
	}
	if strings.TrimSpace(targetPath) != "" {
		args["path"] = targetPath
	}
	if strings.TrimSpace(templateID) != "" {
		args["template"] = templateID
	}
	return args
}

func stringMapToAny(values map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func parseFieldJSONObject(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
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

	s, err := schema.Load(vaultPath)
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
	newCmd.Flags().StringArrayVar(&newFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	newCmd.Flags().StringVar(&newFieldJSON, "field-json", "", "Set frontmatter fields via JSON object (typed values)")
	newCmd.Flags().StringVar(&newPathFlag, "path", "", "Explicit target path (overrides title-derived path)")
	newCmd.Flags().StringVar(&newTemplate, "template", "", "Type template ID to use for object creation")
	newCmd.ValidArgsFunction = completeTypes
	rootCmd.AddCommand(newCmd)
}
