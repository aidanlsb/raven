package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
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
  rvn new person "Freya"               # Creates people/freya.md
  rvn new person "Freya" --field name="Freya"  # With required field
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

		// Parse --field flags into a map
		fieldValues := make(map[string]string)
		for _, f := range newFieldFlags {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) == 2 {
				fieldValues[parts[0]] = parts[1]
			}
		}

		// Auto-fill a declared `title` field from the positional title.
		// This avoids a common footgun where a type defines a required `title` field
		// and agents/users assume `rvn new <type> <title>` satisfies it.
		if typeDef != nil {
			if _, hasTitleField := typeDef.Fields["title"]; hasTitleField {
				if _, provided := fieldValues["title"]; !provided && title != "" {
					fieldValues["title"] = title
				}
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

		}

		// In JSON mode, error if required fields are missing
		if isJSONOutput() && len(missingFields) > 0 {
			outputError(ErrRequiredField,
				fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", ")),
				map[string]interface{}{
					"missing_fields": fieldDetails,
				},
				"Ask user for values, then retry with --field name=value (CLI) or `field: {name: value}` (MCP)")
			return nil // Error already output
		}

		// Use title as target path - pages.Create will apply default_path from schema
		targetPath := title
		if targetPath == "" {
			return fmt.Errorf("invalid title: cannot generate safe filename")
		}

		// Load vault config for directory roots (optional)
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil || vaultCfg == nil {
			vaultCfg = &config.VaultConfig{}
		}
		objectsRoot := vaultCfg.GetObjectsRoot()
		pagesRoot := vaultCfg.GetPagesRoot()

		// Check if file exists (with full path resolution including directory roots)
		resolvedPath := pages.ResolveTargetPathWithRoots(targetPath, typeName, s, objectsRoot, pagesRoot)
		if pages.Exists(vaultPath, resolvedPath) {
			return fmt.Errorf("file already exists: %s.md", pages.SlugifyPath(resolvedPath))
		}

		// Create the page - pages.Create handles default_path and directory roots
		result, err := pages.Create(pages.CreateOptions{
			VaultPath:   vaultPath,
			TypeName:    typeName,
			Title:       title,
			TargetPath:  targetPath,
			Fields:      fieldValues,
			Schema:      s,
			ObjectsRoot: objectsRoot,
			PagesRoot:   pagesRoot,
		})
		if err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		// Auto-reindex if configured (vaultCfg already loaded above)
		if vaultCfg.IsAutoReindexEnabled() {
			if err := reindexFile(vaultPath, result.FilePath); err != nil {
				if !isJSONOutput() {
					fmt.Printf("  (reindex failed: %v)\n", err)
				}
			}
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"file":  result.RelativePath,
				"type":  typeName,
				"title": title,
				"id":    vaultCfg.FilePathToObjectID(result.RelativePath),
			}, nil)
			return nil
		}

		fmt.Printf("Created: %s\n", result.RelativePath)

		// Open in editor (or print path if no editor configured)
		vault.OpenInEditorOrPrintPath(getConfig(), result.FilePath)

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
