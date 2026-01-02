package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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
  rvn type --list          # List available types
  rvn type person --json   # JSON output for agents`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Handle --list flag
		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			return listTypesWithJSON(vaultPath, start)
		}

		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "specify a type name", "Run 'rvn type --list' to see available types")
		}

		typeName := args[0]

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		results, err := db.QueryObjects(typeName)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()

		// Sort by ID for consistent output
		sort.Slice(results, func(i, j int) bool {
			return results[i].ID < results[j].ID
		})

		if isJSONOutput() {
			items := make([]ObjectResult, len(results))
			for i, obj := range results {
				var fields map[string]interface{}
				if obj.Fields != "" && obj.Fields != "{}" {
					json.Unmarshal([]byte(obj.Fields), &fields)
				}
				items[i] = ObjectResult{
					ID:        obj.ID,
					Type:      typeName,
					FilePath:  obj.FilePath,
					LineStart: obj.LineStart,
					Fields:    fields,
				}
			}
			outputSuccess(map[string]interface{}{
				"type":  typeName,
				"items": items,
			}, &Meta{Count: len(items), QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		if len(results) == 0 {
			fmt.Printf("No objects of type '%s' found.\n", typeName)
			return nil
		}

		fmt.Printf("# %s (%d)\n\n", typeName, len(results))

		for _, obj := range results {
			printObject(obj)
		}

		return nil
	},
	ValidArgsFunction: completeTypesForQuery,
}

func listTypes(vaultPath string) error {
	return listTypesWithJSON(vaultPath, time.Now())
}

func listTypesWithJSON(vaultPath string, start time.Time) error {
	// Load schema to get defined types
	s, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	// Open database to get counts
	db, err := index.Open(vaultPath)
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	// Collect type names (using map for deduplication)
	typeSet := make(map[string]bool)
	for name := range s.Types {
		typeSet[name] = false // false = user-defined
	}
	// Add built-in types
	typeSet["page"] = true // true = built-in
	typeSet["section"] = true
	typeSet["date"] = true

	// Sort type names
	var typeNames []string
	for name := range typeSet {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	// Gather data
	var summaries []TypeSummary
	for _, typeName := range typeNames {
		results, err := db.QueryObjects(typeName)
		count := 0
		if err == nil {
			count = len(results)
		}
		summaries = append(summaries, TypeSummary{
			Name:    typeName,
			Count:   count,
			Builtin: typeSet[typeName],
		})
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"types": summaries,
		}, &Meta{Count: len(summaries), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Types:")

	for _, ts := range summaries {
		marker := ""
		if ts.Builtin {
			marker = " (built-in)"
		}

		if ts.Count > 0 {
			fmt.Printf("  %-15s %d objects%s\n", ts.Name, ts.Count, marker)
		} else {
			fmt.Printf("  %-15s -%s\n", ts.Name, marker)
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
