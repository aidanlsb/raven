package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var typeCmd = &cobra.Command{
	Use:   "type <name>",
	Short: "List objects of a specific type",
	Long: `Lists all objects in the vault of the specified type.

Examples:
  rvn type person          # List all people
  rvn type project         # List all projects
  rvn type meeting         # List all meetings
  rvn type --list          # List available types`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --list flag
		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			return listTypes(vaultPath)
		}

		if len(args) == 0 {
			return fmt.Errorf("specify a type name\n\nRun 'rvn type --list' to see available types")
		}

		typeName := args[0]

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		results, err := db.QueryObjects(typeName)
		if err != nil {
			return fmt.Errorf("failed to query: %w", err)
		}

		if len(results) == 0 {
			fmt.Printf("No objects of type '%s' found.\n", typeName)
			return nil
		}

		fmt.Printf("# %s (%d)\n\n", typeName, len(results))

		// Sort by ID for consistent output
		sort.Slice(results, func(i, j int) bool {
			return results[i].ID < results[j].ID
		})

		for _, obj := range results {
			printObject(obj)
		}

		return nil
	},
	ValidArgsFunction: completeTypesForQuery,
}

func listTypes(vaultPath string) error {
	// Load schema to get defined types
	s, err := schema.Load(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	// Open database to get counts
	db, err := index.Open(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Println("Types:")

	// Collect type names (using map for deduplication)
	typeSet := make(map[string]bool)
	for name := range s.Types {
		typeSet[name] = false // false = user-defined
	}
	// Add built-in types
	typeSet["page"] = true  // true = built-in
	typeSet["section"] = true
	typeSet["date"] = true

	// Sort type names
	var typeNames []string
	for name := range typeSet {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	for _, typeName := range typeNames {
		results, err := db.QueryObjects(typeName)
		if err != nil {
			continue
		}
		count := len(results)

		marker := ""
		if isBuiltin := typeSet[typeName]; isBuiltin {
			marker = " (built-in)"
		}

		if count > 0 {
			fmt.Printf("  %-15s %d objects%s\n", typeName, count, marker)
		} else {
			fmt.Printf("  %-15s -%s\n", typeName, marker)
		}
	}

	return nil
}

func printObject(obj index.ObjectResult) {
	fmt.Printf("• %s\n", obj.ID)

	// Parse and display key fields
	if obj.Fields != "" && obj.Fields != "{}" {
		var fields map[string]interface{}
		if err := json.Unmarshal([]byte(obj.Fields), &fields); err == nil {
			// Show select fields inline
			var fieldStrs []string
			for key, val := range fields {
				// Skip internal fields
				if key == "type" || key == "id" {
					continue
				}
				// Format value
				valStr := fmt.Sprintf("%v", val)
				if len(valStr) > 30 {
					valStr = valStr[:27] + "..."
				}
				fieldStrs = append(fieldStrs, fmt.Sprintf("%s: %s", key, valStr))
			}
			if len(fieldStrs) > 0 {
				sort.Strings(fieldStrs)
				fmt.Printf("  %s\n", strings.Join(fieldStrs, ", "))
			}
		}
	}

	fmt.Printf("  %s:%d\n", obj.FilePath, obj.LineStart)
}

// completeTypesForQuery provides shell completion for type names
func completeTypesForQuery(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	vaultPath := getVaultPath()
	if vaultPath == "" {
		return []string{"page", "section", "date"}, cobra.ShellCompDirectiveNoFileComp
	}

	s, err := schema.Load(vaultPath)
	if err != nil {
		return []string{"page", "section", "date"}, cobra.ShellCompDirectiveNoFileComp
	}

	var types []string
	for name := range s.Types {
		types = append(types, name)
	}
	types = append(types, "page", "section", "date")
	sort.Strings(types)

	return types, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	typeCmd.Flags().BoolP("list", "l", false, "List available types with counts")
	rootCmd.AddCommand(typeCmd)
}
