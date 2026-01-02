package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ravenscroftj/raven/internal/pages"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/ravenscroftj/raven/internal/vault"
	"github.com/spf13/cobra"
)

var newFieldFlags []string

var newCmd = &cobra.Command{
	Use:   "new <type> [title]",
	Short: "Create a new typed note",
	Long: `Creates a new note with the specified type.

The type is required. If title is not provided, you will be prompted for it.
The file location is determined by the type's default_path setting in schema.yaml.
Required fields (as defined in schema.yaml) will be prompted for interactively,
or can be provided via --field flags.

Examples:
  rvn new person                       # Prompts for title, creates in people/
  rvn new person "Alice Chen"          # Creates people/alice-chen.md
  rvn new person "Alice" --field name="Alice Chen"  # With required field
  rvn new project "Website Redesign"   # Creates projects/website-redesign.md
  rvn new page "Quick Note"            # Creates quick-note.md in vault root`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		typeName := args[0]

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
		}

		// Check if type exists
		typeDef, typeExists := s.Types[typeName]
		if !typeExists && typeName != "page" && typeName != "section" && typeName != "date" {
			// List available types
			var typeNames []string
			for name := range s.Types {
				typeNames = append(typeNames, name)
			}
			sort.Strings(typeNames)
			return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", ")))
		}

		// Get title from args or prompt
		var title string
		reader := bufio.NewReader(os.Stdin)

		if len(args) >= 2 {
			title = args[1]
		} else if isJSONOutput() {
			// Non-interactive mode: require title as argument
			return handleErrorMsg(ErrMissingArgument, "title is required", "Usage: rvn new <type> <title> --json")
		} else {
			fmt.Printf("Title: ")
			title, err = reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			title = strings.TrimSpace(title)
			if title == "" {
				return handleErrorMsg(ErrMissingArgument, "title cannot be empty", "")
			}
		}

		// Get default_path
		var defaultPath string
		if typeDef != nil && typeDef.DefaultPath != "" {
			defaultPath = typeDef.DefaultPath
		}

		// Parse --field flags into a map
		fieldValues := make(map[string]string)
		for _, f := range newFieldFlags {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) == 2 {
				fieldValues[parts[0]] = parts[1]
			}
		}

		// Collect required fields and check which are missing
		var missingFields []string
		var fieldDetails []map[string]interface{}
		
		if typeDef != nil {
			// Sort field names for consistent order
			var fieldNames []string
			for name := range typeDef.Fields {
				fieldNames = append(fieldNames, name)
			}
			sort.Strings(fieldNames)

			for _, fieldName := range fieldNames {
				fieldDef := typeDef.Fields[fieldName]
				if fieldDef != nil && fieldDef.Required {
					// Check if already provided via --field
					if _, ok := fieldValues[fieldName]; ok {
						continue
					}
					
					// Check if there's a default
					if fieldDef.Default != nil {
						fieldValues[fieldName] = fmt.Sprintf("%v", fieldDef.Default)
						continue
					}
					
					if isJSONOutput() {
						// Non-interactive: collect missing required fields for error
						missingFields = append(missingFields, fieldName)
						detail := map[string]interface{}{
							"name":     fieldName,
							"type":     string(fieldDef.Type),
							"required": true,
						}
						if len(fieldDef.Values) > 0 {
							detail["values"] = fieldDef.Values
						}
						fieldDetails = append(fieldDetails, detail)
					} else {
						// Interactive: prompt for value
						fmt.Printf("%s (required): ", fieldName)
						value, err := reader.ReadString('\n')
						if err != nil {
							return fmt.Errorf("failed to read input: %w", err)
						}
						value = strings.TrimSpace(value)
						if value == "" {
							return fmt.Errorf("required field '%s' cannot be empty", fieldName)
						}
						fieldValues[fieldName] = value
					}
				}
			}

			// Collect required traits
			for _, traitName := range typeDef.Traits.List() {
				if typeDef.Traits.IsRequired(traitName) {
					// Check if already provided via --field
					if _, ok := fieldValues[traitName]; ok {
						continue
					}
					
					// Check for default
					traitConfig := typeDef.Traits.Configs[traitName]
					if traitConfig != nil && traitConfig.Default != nil {
						fieldValues[traitName] = fmt.Sprintf("%v", traitConfig.Default)
						continue
					}
					
					traitDef := s.Traits[traitName]
					
					if isJSONOutput() {
						// Non-interactive: collect missing required traits for error
						missingFields = append(missingFields, traitName)
						detail := map[string]interface{}{
							"name":     traitName,
							"type":     "trait",
							"required": true,
						}
						if traitDef != nil {
							detail["trait_type"] = string(traitDef.Type)
							if len(traitDef.Values) > 0 {
								detail["values"] = traitDef.Values
							}
						}
						fieldDetails = append(fieldDetails, detail)
					} else {
						// Interactive: prompt for value
						hint := ""
						if traitDef != nil {
							switch traitDef.Type {
							case schema.FieldTypeDate:
								hint = " (YYYY-MM-DD)"
							case schema.FieldTypeEnum:
								hint = fmt.Sprintf(" (%s)", strings.Join(traitDef.Values, "/"))
							}
						}
						fmt.Printf("%s (required)%s: ", traitName, hint)
						value, err := reader.ReadString('\n')
						if err != nil {
							return fmt.Errorf("failed to read input: %w", err)
						}
						value = strings.TrimSpace(value)
						if value == "" {
							return fmt.Errorf("required trait '%s' cannot be empty", traitName)
						}
						fieldValues[traitName] = value
					}
				}
			}
		}
		
		// In JSON mode, error if required fields are missing
		if isJSONOutput() && len(missingFields) > 0 {
			outputError(ErrRequiredField, 
				fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", ")),
				map[string]interface{}{
					"missing_fields": fieldDetails,
				},
				"Ask user for values, then retry with --field name=value flags")
			return nil // Error already output
		}

		// Build target path from default_path + slugified title
		filename := pages.Slugify(title)
		if filename == "" {
			return fmt.Errorf("invalid title: cannot generate safe filename")
		}

		var targetPath string
		if defaultPath != "" {
			// Validate default_path doesn't escape vault
			cleanPath := filepath.Clean(defaultPath)
			if strings.Contains(cleanPath, "..") || filepath.IsAbs(cleanPath) {
				return fmt.Errorf("invalid default_path in schema: %s", defaultPath)
			}
			targetPath = filepath.Join(cleanPath, filename)
		} else {
			targetPath = filename
		}

		// Check if file exists
		if pages.Exists(vaultPath, targetPath) {
			return fmt.Errorf("file already exists: %s.md", pages.SlugifyPath(targetPath))
		}

		// Create the page
		result, err := pages.Create(pages.CreateOptions{
			VaultPath:  vaultPath,
			TypeName:   typeName,
			Title:      title,
			TargetPath: targetPath,
			Fields:     fieldValues,
			Schema:     s,
		})
		if err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"file":  result.RelativePath,
				"type":  typeName,
				"title": title,
				"id":    strings.TrimSuffix(result.RelativePath, ".md"),
			}, nil)
			return nil
		}

		fmt.Printf("Created: %s\n", result.RelativePath)

		// Open in editor (only in interactive mode)
		vault.OpenInEditor(getConfig(), result.FilePath)

		return nil
	},
	ValidArgsFunction: completeTypes,
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
		return []string{"page", "section", "date"}, cobra.ShellCompDirectiveNoFileComp
	}

	s, err := schema.Load(vaultPath)
	if err != nil {
		return []string{"page", "section", "date"}, cobra.ShellCompDirectiveNoFileComp
	}

	// Collect all type names
	var types []string
	for name := range s.Types {
		types = append(types, name)
	}
	// Add built-in types
	types = append(types, "page", "date")

	sort.Strings(types)
	return types, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	newCmd.Flags().StringArrayVar(&newFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	rootCmd.AddCommand(newCmd)
}
