package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
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

var newCmd = &cobra.Command{
	Use:   "new <type> [title]",
	Short: "Create a new typed note",
	Long: `Creates a new note with the specified type.

The type is required. If title is not provided, you will be prompted for it.
The file location is determined by the type's default_path setting in schema.yaml.
Required fields (as defined in schema.yaml) will be prompted for interactively,
or can be provided via --field or --field-json flags.

Examples:
  rvn new person                       # Prompts for title, creates in people/
  rvn new person "Freya"               # Creates people/freya.md
  rvn new person "Freya" --field name="Freya"  # With required field
  rvn new interview "Jane Doe @ Acme" --template technical
  rvn new note "Raven Friction" --path note/raven-friction
  rvn new project "Website Redesign"   # Creates projects/website-redesign.md
  rvn new page "Quick Note"            # Creates quick-note.md in vault root`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		typeName := args[0]

		// Get title from args or prompt
		var title string
		reader := bufio.NewReader(os.Stdin)
		var err error

		if len(args) >= 2 {
			title = args[1]
		} else if isJSONOutput() {
			// Non-interactive mode: require title as argument
			return handleErrorMsg(ErrMissingArgument, "title is required", "Usage: rvn new <type> <title> --json")
		} else {
			fmt.Fprintf(os.Stderr, "Title: ")
			title, err = reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			title = strings.TrimSpace(title)
			if title == "" {
				return handleErrorMsg(ErrMissingArgument, "title cannot be empty", "")
			}
		}

		if err := validateObjectTitle(title); err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Provide a plain title without path separators")
		}

		fieldValues, err := parseFieldFlags(newFieldFlags)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
		}
		fieldJSONRaw, err := parseFieldJSONObject(newFieldJSON)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
		}

		targetPath := strings.TrimSpace(newPathFlag)
		if targetPath != "" {
			if err := validateObjectTargetPath(targetPath); err != nil {
				return handleErrorMsg(ErrInvalidInput, err.Error(), "Use --path with an explicit object path like note/raven-friction")
			}
		}

		for {
			result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
				CommandID: "new",
				VaultPath: vaultPath,
				Caller:    commandexec.CallerCLI,
				Args: buildNewCommandArgs(
					typeName,
					title,
					targetPath,
					newTemplate,
					fieldValues,
					fieldJSONRaw,
				),
			})
			if !result.OK {
				if !isJSONOutput() && result.Error != nil && result.Error.Code == ErrRequiredFieldMissing {
					prompted := false
					for _, fieldName := range missingFieldNamesFromDetails(result.Error.Details) {
						if _, exists := fieldValues[fieldName]; exists {
							continue
						}
						fmt.Fprintf(os.Stderr, "%s (required): ", fieldName)
						value, readErr := reader.ReadString('\n')
						if readErr != nil {
							return fmt.Errorf("failed to read input: %w", readErr)
						}
						value = strings.TrimSpace(value)
						if value == "" {
							return fmt.Errorf("required field '%s' cannot be empty", fieldName)
						}
						fieldValues[fieldName] = value
						prompted = true
					}
					if prompted {
						continue
					}
				}

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

			data, _ := result.Data.(map[string]interface{})
			relativePath, _ := data["file"].(string)
			fmt.Println(ui.Checkf("Created %s", ui.FilePath(relativePath)))

			// Open in editor (or print path if no editor configured)
			vault.OpenInEditorOrPrintPath(getConfig(), filepath.Join(vaultPath, filepath.FromSlash(relativePath)))

			return nil
		}
	},
	ValidArgsFunction: completeTypes,
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
	rootCmd.AddCommand(newCmd)
}
