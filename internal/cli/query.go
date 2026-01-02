package cli

import (
	"fmt"
	"strings"
	"time"

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
  rvn query tasks --json    # JSON output for agents
  
List saved queries:
  rvn query --list`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Load vault config for saved queries
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Handle --list flag
		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			return listSavedQueries(vaultCfg, start)
		}

		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "specify a query name or trait type", "Run 'rvn query --list' to see saved queries")
		}

		queryName := args[0]
		valueFilter, _ := cmd.Flags().GetString("value")

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		// Check if this is a saved query
		if savedQuery, ok := vaultCfg.Queries[queryName]; ok {
			return runSavedQueryWithJSON(db, savedQuery, queryName, start)
		}

		// Otherwise, treat as a trait type query
		return runTraitQueryWithJSON(db, queryName, valueFilter, start)
	},
}

func listSavedQueries(vaultCfg *config.VaultConfig, start time.Time) error {
	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		var queries []SavedQueryInfo
		for name, q := range vaultCfg.Queries {
			queries = append(queries, SavedQueryInfo{
				Name:        name,
				Description: q.Description,
				Types:       q.Types,
				Traits:      q.Traits,
				Tags:        q.Tags,
				Filters:     q.Filters,
			})
		}
		outputSuccess(map[string]interface{}{
			"queries": queries,
		}, &Meta{Count: len(queries), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Saved queries:")
	if len(vaultCfg.Queries) == 0 {
		fmt.Println("  (none defined)")
		fmt.Println("\nDefine queries in raven.yaml under 'queries:'")
		return nil
	}
	for name, q := range vaultCfg.Queries {
		desc := q.Description
		if desc == "" {
			var parts []string
			if len(q.Types) > 0 {
				parts = append(parts, fmt.Sprintf("types: %v", q.Types))
			}
			if len(q.Traits) > 0 {
				parts = append(parts, fmt.Sprintf("traits: %v", q.Traits))
			}
			if len(q.Tags) > 0 {
				parts = append(parts, fmt.Sprintf("tags: %v", q.Tags))
			}
			desc = strings.Join(parts, ", ")
		}
		fmt.Printf("  %-12s %s\n", name, desc)
	}
	return nil
}

func runSavedQuery(db *index.Database, q *config.SavedQuery, name string) error {
	return runSavedQueryWithJSON(db, q, name, time.Now())
}

func runSavedQueryWithJSON(db *index.Database, q *config.SavedQuery, name string, start time.Time) error {
	if len(q.Traits) == 0 && len(q.Types) == 0 && len(q.Tags) == 0 {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' has no traits, types, or tags defined", name), "")
	}

	var result QueryResult
	result.QueryName = name
	hasResults := false

	// Query types if specified
	if len(q.Types) > 0 {
		for _, typeName := range q.Types {
			results, err := db.QueryObjects(typeName)
			if err != nil {
				return handleError(ErrDatabaseError, err, "")
			}

			if len(results) > 0 {
				hasResults = true
				var items []ObjectResult
				for _, obj := range results {
				items = append(items, ObjectResult{
					ID:        obj.ID,
					Type:      typeName,
					FilePath:  obj.FilePath,
					LineStart: obj.LineStart,
				})
				}
				result.Types = append(result.Types, TypeQueryResult{
					Type:  typeName,
					Items: items,
				})

				if !isJSONOutput() {
					fmt.Printf("## %s (%d)\n\n", typeName, len(results))
					for _, obj := range results {
						fmt.Printf("• %s\n", obj.ID)
						fmt.Printf("  %s:%d\n", obj.FilePath, obj.LineStart)
					}
					fmt.Println()
				}
			}
		}
	}

	// Query traits if specified
	if len(q.Traits) > 0 {
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
				return handleError(ErrDatabaseError, err, "")
			}
			allResults = append(allResults, results...)
		}

		if len(allResults) > 0 {
			hasResults = true
			for _, r := range allResults {
				result.Traits = append(result.Traits, TraitResult{
					ID:          fmt.Sprintf("%s:%d", r.FilePath, r.Line),
					TraitType:   r.TraitType,
					Value:       r.Value,
					Content:     r.Content,
					ContentText: r.Content,
					ObjectID:    r.ParentID,
					FilePath:    r.FilePath,
					Line:        r.Line,
				})
			}
			if !isJSONOutput() {
				printTraitResults(allResults)
			}
		}
	}

	// Query tags if specified
	if len(q.Tags) > 0 {
		var objectIDs []string
		if len(q.Tags) == 1 {
			results, err := db.QueryTags(q.Tags[0])
			if err != nil {
				return handleError(ErrDatabaseError, err, "")
			}
			for _, r := range results {
				objectIDs = append(objectIDs, r.ObjectID)
			}
		} else {
			var err error
			objectIDs, err = db.QueryTagsMultiple(q.Tags)
			if err != nil {
				return handleError(ErrDatabaseError, err, "")
			}
		}

		if len(objectIDs) > 0 {
			hasResults = true
			result.Tags = append(result.Tags, TagQueryResult{
				Tags:  q.Tags,
				Items: objectIDs,
			})

			if !isJSONOutput() {
				if len(q.Tags) == 1 {
					fmt.Printf("## #%s (%d)\n\n", q.Tags[0], len(objectIDs))
				} else {
					fmt.Printf("## %s (%d)\n\n", formatTagList(q.Tags), len(objectIDs))
				}
				for _, id := range objectIDs {
					fmt.Printf("• %s\n", id)
				}
				fmt.Println()
			}
		}
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		totalCount := len(result.Traits)
		for _, t := range result.Types {
			totalCount += len(t.Items)
		}
		for _, t := range result.Tags {
			totalCount += len(t.Items)
		}
		outputSuccess(result, &Meta{Count: totalCount, QueryTimeMs: elapsed})
		return nil
	}

	if !hasResults {
		fmt.Printf("No results for query '%s'.\n", name)
	}

	return nil
}

func formatTagList(tags []string) string {
	var formatted []string
	for _, t := range tags {
		formatted = append(formatted, "#"+t)
	}
	return strings.Join(formatted, " + ")
}

func runTraitQuery(db *index.Database, traitType string, valueFilter string) error {
	return runTraitQueryWithJSON(db, traitType, valueFilter, time.Now())
}

func runTraitQueryWithJSON(db *index.Database, traitType string, valueFilter string, start time.Time) error {
	var filter *string
	if valueFilter != "" {
		filter = &valueFilter
	}

	results, err := db.QueryTraits(traitType, filter)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		items := make([]TraitResult, len(results))
		for i, r := range results {
			items[i] = TraitResult{
				ID:          fmt.Sprintf("%s:%d", r.FilePath, r.Line),
				TraitType:   r.TraitType,
				Value:       r.Value,
				Content:     r.Content,
				ContentText: r.Content,
				ObjectID:    r.ParentID,
				FilePath:    r.FilePath,
				Line:        r.Line,
			}
		}
		outputSuccess(map[string]interface{}{
			"trait": traitType,
			"items": items,
		}, &Meta{Count: len(items), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
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
