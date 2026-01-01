package cli

import (
	"fmt"
	"strings"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <name-or-trait>",
	Short: "Run a saved query or query a trait",
	Long: `Query traits using saved queries from raven.yaml or ad-hoc trait queries.

Saved queries are defined in raven.yaml under 'queries:'.

Examples:
  rvn query tasks           # Run saved query 'tasks'
  rvn query overdue         # Run saved query 'overdue'
  rvn query due             # Query all @due traits
  rvn query status --value todo  # Query @status traits with value 'todo'
  
List saved queries:
  rvn query --list`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Load vault config for saved queries
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load vault config: %w", err)
		}

		// Handle --list flag
		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			fmt.Println("Saved queries:")
			if len(vaultCfg.Queries) == 0 {
				fmt.Println("  (none defined)")
				fmt.Println("\nDefine queries in raven.yaml under 'queries:'")
				return nil
			}
			for name, q := range vaultCfg.Queries {
				desc := q.Description
				if desc == "" {
					desc = fmt.Sprintf("traits: %v", q.Traits)
				}
				fmt.Printf("  %-12s %s\n", name, desc)
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("specify a query name or trait type\n\nRun 'rvn query --list' to see saved queries")
		}

		queryName := args[0]
		valueFilter, _ := cmd.Flags().GetString("value")

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		// Check if this is a saved query
		if savedQuery, ok := vaultCfg.Queries[queryName]; ok {
			return runSavedQuery(db, savedQuery, queryName)
		}

		// Otherwise, treat as a trait type query
		return runTraitQuery(db, queryName, valueFilter)
	},
}

func runSavedQuery(db *index.Database, q *config.SavedQuery, name string) error {
	if len(q.Traits) == 0 {
		return fmt.Errorf("saved query '%s' has no traits defined", name)
	}

	// For now, query each trait separately and combine results
	// A more sophisticated implementation would do a SQL JOIN
	var allResults []index.TraitResult

	for _, traitType := range q.Traits {
		var filter *string
		if q.Filters != nil {
			if f, ok := q.Filters[traitType]; ok {
				filter = &f
			}
		}

		results, err := db.QueryTraits(traitType, filter)
		if err != nil {
			return fmt.Errorf("failed to query %s: %w", traitType, err)
		}
		allResults = append(allResults, results...)
	}

	if len(allResults) == 0 {
		fmt.Printf("No results for query '%s'.\n", name)
		return nil
	}

	// Group by file and line to deduplicate and show combined traits
	printTraitResults(allResults)
	return nil
}

func runTraitQuery(db *index.Database, traitType string, valueFilter string) error {
	var filter *string
	if valueFilter != "" {
		filter = &valueFilter
	}

	results, err := db.QueryTraits(traitType, filter)
	if err != nil {
		return fmt.Errorf("failed to query: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No @%s traits found.\n", traitType)
		return nil
	}

	printTraitResults(results)
	return nil
}

func printTraitResults(results []index.TraitResult) {
	// Group by content line to show all traits on same content
	type contentKey struct {
		filePath string
		line     int
	}

	grouped := make(map[contentKey][]index.TraitResult)
	var order []contentKey

	for _, r := range results {
		key := contentKey{r.FilePath, r.Line}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], r)
	}

	for _, key := range order {
		traits := grouped[key]
		// Use content from first trait (they should be the same)
		content := traits[0].Content

		// Build trait summary
		var traitStrs []string
		for _, t := range traits {
			if t.Value != nil && *t.Value != "" {
				traitStrs = append(traitStrs, fmt.Sprintf("@%s(%s)", t.TraitType, *t.Value))
			} else {
				traitStrs = append(traitStrs, fmt.Sprintf("@%s", t.TraitType))
			}
		}

		fmt.Printf("• %s\n", content)
		fmt.Printf("  %s  %s:%d\n", strings.Join(traitStrs, " "), key.filePath, key.line)
	}
}

func init() {
	queryCmd.Flags().BoolP("list", "l", false, "List saved queries")
	queryCmd.Flags().String("value", "", "Filter by trait value")
	rootCmd.AddCommand(queryCmd)
}
