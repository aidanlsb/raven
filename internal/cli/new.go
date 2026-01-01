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

var newCmd = &cobra.Command{
	Use:   "new <type> [title]",
	Short: "Create a new typed note",
	Long: `Creates a new note with the specified type.

The type is required. If title is not provided, you will be prompted for it.
The file location is determined by the type's default_path setting in schema.yaml.
Required fields (as defined in schema.yaml) will be prompted for interactively.

Examples:
  rvn new person                       # Prompts for title, creates in people/
  rvn new person "Alice Chen"          # Creates people/alice-chen.md
  rvn new project "Website Redesign"   # Creates projects/website-redesign.md
  rvn new page "Quick Note"            # Creates quick-note.md in vault root`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		typeName := args[0]

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
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
			return fmt.Errorf("type '%s' not defined in schema.yaml\nAvailable types: %s", typeName, strings.Join(typeNames, ", "))
		}

		// Get title from args or prompt
		var title string
		reader := bufio.NewReader(os.Stdin)

		if len(args) >= 2 {
			title = args[1]
		} else {
			fmt.Printf("Title: ")
			title, err = reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			title = strings.TrimSpace(title)
			if title == "" {
				return fmt.Errorf("title cannot be empty")
			}
		}

		// Get default_path
		var defaultPath string
		if typeDef != nil && typeDef.DefaultPath != "" {
			defaultPath = typeDef.DefaultPath
		}

		// Collect required field values
		fieldValues := make(map[string]string)
		if typeDef != nil {
			// Sort field names for consistent prompting order
			var fieldNames []string
			for name := range typeDef.Fields {
				fieldNames = append(fieldNames, name)
			}
			sort.Strings(fieldNames)

			for _, fieldName := range fieldNames {
				fieldDef := typeDef.Fields[fieldName]
				if fieldDef != nil && fieldDef.Required {
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

			// Collect required traits
			for _, traitName := range typeDef.Traits.List() {
				if typeDef.Traits.IsRequired(traitName) {
					traitDef := s.Traits[traitName]
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
			return err
		}

		fmt.Printf("Created: %s\n", result.RelativePath)

		// Open in editor
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
	rootCmd.AddCommand(newCmd)
}
