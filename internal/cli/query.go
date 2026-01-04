package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/audit"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <query-string>",
	Short: "Run a query using the Raven query language",
	Long: `Query objects or traits using the Raven query language.

Query types:
  object:<type> [predicates]   Query objects of a type
  trait:<name> [predicates]    Query traits by name

Predicates for object queries:
  .field:value     Field equals value
  .field:*         Field exists
  !.field:value    Field does not equal value
  has:trait        Has a trait
  parent:type      Direct parent is type
  ancestor:type    Any ancestor is type
  child:type       Has child of type

Predicates for trait queries:
  value:val        Trait value equals val
  source:inline    Only inline traits
  on:type          Direct parent object is type
  within:type      Any ancestor object is type

Boolean operators:
  !pred            NOT
  pred1 pred2      AND (space-separated)
  pred1 | pred2    OR

Subqueries use curly braces:
  has:{trait:due value:past}
  on:{object:project .status:active}

Examples:
  rvn query "object:project .status:active"
  rvn query "object:meeting has:due"
  rvn query "trait:due value:past"
  rvn query "trait:highlight on:{object:book .status:reading}"
  rvn query tasks                    # Run saved query
  rvn query --list                   # List saved queries`,
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
			return handleErrorMsg(ErrMissingArgument, "specify a query string", "Run 'rvn query --list' to see saved queries")
		}

		queryStr := args[0]

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		// Check staleness and optionally refresh
		refresh, _ := cmd.Flags().GetBool("refresh")
		if refresh {
			if err := smartReindex(db, vaultPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to refresh index: %v\n", err)
			}
		} else {
			// Check for staleness and warn
			warnIfStale(db, vaultPath)
		}

		// Check if this is a full query string (starts with object: or trait:)
		if strings.HasPrefix(queryStr, "object:") || strings.HasPrefix(queryStr, "trait:") {
			return runFullQuery(db, queryStr, start)
		}

		// Check if this is a saved query
		if savedQuery, ok := vaultCfg.Queries[queryStr]; ok {
			return runSavedQueryWithJSON(db, savedQuery, queryStr, start)
		}

		// Unknown query - provide helpful error
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			"Queries must start with 'object:' or 'trait:', or be a saved query name. Run 'rvn query --list' to see saved queries.")
	},
}

func runFullQuery(db *index.Database, queryStr string, start time.Time) error {
	// Parse the query
	q, err := query.Parse(queryStr)
	if err != nil {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("parse error: %v", err), "")
	}

	executor := query.NewExecutor(db.DB())

	elapsed := time.Since(start).Milliseconds()

	if q.Type == query.QueryTypeObject {
		results, err := executor.ExecuteObjectQuery(q)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		if isJSONOutput() {
			items := make([]map[string]interface{}, len(results))
			for i, r := range results {
				items[i] = map[string]interface{}{
					"id":        r.ID,
					"type":      r.Type,
					"fields":    r.Fields,
					"file_path": r.FilePath,
					"line":      r.LineStart,
				}
			}
			outputSuccess(map[string]interface{}{
				"query_type": "object",
				"type":       q.TypeName,
				"items":      items,
			}, &Meta{Count: len(items), QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		if len(results) == 0 {
			fmt.Printf("No objects found for: %s\n", queryStr)
			return nil
		}

		fmt.Printf("# %s (%d)\n\n", q.TypeName, len(results))
		for _, r := range results {
			fmt.Printf("• %s\n", r.ID)
			if len(r.Fields) > 0 {
				var fieldStrs []string
				for k, v := range r.Fields {
					if k != "type" && k != "id" {
						fieldStrs = append(fieldStrs, fmt.Sprintf("%s: %v", k, v))
					}
				}
				if len(fieldStrs) > 0 {
					fmt.Printf("  %s\n", strings.Join(fieldStrs, ", "))
				}
			}
			fmt.Printf("  %s:%d\n", r.FilePath, r.LineStart)
		}
		return nil
	}

	// Trait query
	results, err := executor.ExecuteTraitQuery(q)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	if isJSONOutput() {
		items := make([]map[string]interface{}, len(results))
		for i, r := range results {
			items[i] = map[string]interface{}{
				"id":         r.ID,
				"trait_type": r.TraitType,
				"value":      r.Value,
				"content":    r.Content,
				"file_path":  r.FilePath,
				"line":       r.Line,
				"object_id":  r.ParentObjectID,
				"source":     r.Source,
			}
		}
		outputSuccess(map[string]interface{}{
			"query_type": "trait",
			"trait":      q.TypeName,
			"items":      items,
		}, &Meta{Count: len(items), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	if len(results) == 0 {
		fmt.Printf("No traits found for: %s\n", queryStr)
		return nil
	}

	fmt.Printf("# @%s (%d)\n\n", q.TypeName, len(results))
	for _, r := range results {
		valueStr := ""
		if r.Value != nil && *r.Value != "" {
			valueStr = fmt.Sprintf("(%s)", *r.Value)
		}
		fmt.Printf("• %s\n", r.Content)
		fmt.Printf("  @%s%s  %s:%d\n", r.TraitType, valueStr, r.FilePath, r.Line)
	}
	return nil
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
			desc = strings.Join(parts, ", ")
		}
		fmt.Printf("  %-12s %s\n", name, desc)
	}
	return nil
}

func runSavedQueryWithJSON(db *index.Database, q *config.SavedQuery, name string, start time.Time) error {
	if len(q.Traits) == 0 && len(q.Types) == 0 {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' has no traits or types defined", name), "")
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

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		totalCount := len(result.Traits)
		for _, t := range result.Types {
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

var queryAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a saved query to raven.yaml",
	Long: `Add a new saved query to raven.yaml.

Examples:
  rvn query add overdue --traits due --filter due=past
  rvn query add my-tasks --traits due,status --filter status=todo
  rvn query add people --types person
  rvn query add mixed --types project --traits due --description "Projects with due dates"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		queryName := args[0]

		// Get flags
		traits, _ := cmd.Flags().GetStringSlice("traits")
		types, _ := cmd.Flags().GetStringSlice("types")
		filters, _ := cmd.Flags().GetStringSlice("filter")
		description, _ := cmd.Flags().GetString("description")

		// Validate - must have at least one of traits or types
		if len(traits) == 0 && len(types) == 0 {
			return handleErrorMsg(ErrInvalidInput, "must specify at least one of --traits or --types", "")
		}

		// Parse filters into map
		filterMap := make(map[string]string)
		for _, f := range filters {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) != 2 {
				return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("invalid filter format: %s (expected key=value)", f), "")
			}
			filterMap[parts[0]] = parts[1]
		}

		// Load existing config
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Check if query already exists
		if _, exists := vaultCfg.Queries[queryName]; exists {
			return handleErrorMsg(ErrDuplicateName, fmt.Sprintf("query '%s' already exists", queryName), "Use 'rvn query remove' first to replace it")
		}

		// Build new query
		newQuery := config.SavedQuery{
			Description: description,
		}
		if len(traits) > 0 {
			newQuery.Traits = traits
		}
		if len(types) > 0 {
			newQuery.Types = types
		}
		if len(filterMap) > 0 {
			newQuery.Filters = filterMap
		}

		// Update config
		if vaultCfg.Queries == nil {
			vaultCfg.Queries = make(map[string]*config.SavedQuery)
		}
		vaultCfg.Queries[queryName] = &newQuery

		// Write back to raven.yaml
		if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Audit log
		logger := audit.New(vaultPath, vaultCfg.IsAuditLogEnabled())
		logger.LogCreate("query", queryName, "saved_query", map[string]interface{}{
			"traits":  traits,
			"types":   types,
			"filters": filterMap,
		})

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":        queryName,
				"traits":      traits,
				"types":       types,
				"filters":     filterMap,
				"description": description,
			}, nil)
		} else {
			fmt.Printf("✓ Added query '%s'\n", queryName)
			fmt.Printf("  Run with: rvn query %s\n", queryName)
		}

		return nil
	},
}

var queryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a saved query from raven.yaml",
	Long: `Remove a saved query from raven.yaml.

Examples:
  rvn query remove overdue
  rvn query remove my-tasks`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		queryName := args[0]

		// Load existing config
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Check if query exists
		if _, exists := vaultCfg.Queries[queryName]; !exists {
			return handleErrorMsg(ErrQueryNotFound, fmt.Sprintf("query '%s' not found", queryName), "Run 'rvn query --list' to see available queries")
		}

		// Remove query
		delete(vaultCfg.Queries, queryName)

		// Write back to raven.yaml
		if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
			return handleError(ErrInternal, err, "")
		}

		// Audit log
		logger := audit.New(vaultPath, vaultCfg.IsAuditLogEnabled())
		logger.LogDelete("query", queryName)

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":    queryName,
				"removed": true,
			}, nil)
		} else {
			fmt.Printf("✓ Removed query '%s'\n", queryName)
		}

		return nil
	},
}

// warnIfStale checks if the index has stale files and prints a warning.
// Only warns for non-JSON output to avoid polluting machine-readable results.
func warnIfStale(db *index.Database, vaultPath string) {
	if isJSONOutput() {
		return
	}

	staleness, err := db.CheckStaleness(vaultPath)
	if err != nil {
		return // Silently fail - don't break queries for staleness check errors
	}

	if staleness.IsStale {
		staleCount := len(staleness.StaleFiles)
		if staleCount == 1 {
			fmt.Fprintf(os.Stderr, "⚠ Warning: 1 file may be stale. Run 'rvn reindex --smart' or use '--refresh'.\n")
		} else if staleCount <= 3 {
			fmt.Fprintf(os.Stderr, "⚠ Warning: %d files may be stale: %s\n",
				staleCount, strings.Join(staleness.StaleFiles, ", "))
			fmt.Fprintf(os.Stderr, "  Run 'rvn reindex --smart' or use '--refresh' to update.\n")
		} else {
			fmt.Fprintf(os.Stderr, "⚠ Warning: %d files may be stale. Run 'rvn reindex --smart' or use '--refresh'.\n", staleCount)
		}
		fmt.Fprintln(os.Stderr)
	}
}

// smartReindex performs an incremental reindex of only stale files.
func smartReindex(db *index.Database, vaultPath string) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return err
	}

	var reindexed int
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return nil // Skip files with errors
		}

		// Check if file needs reindexing
		indexedMtime, err := db.GetFileMtime(result.RelativePath)
		if err == nil && indexedMtime > 0 && result.FileMtime <= indexedMtime {
			return nil // File is up-to-date
		}

		// Reindex this file
		if err := db.IndexDocumentWithMtime(result.Document, sch, result.FileMtime); err != nil {
			return nil // Skip files that fail to index
		}

		reindexed++
		return nil
	})

	if err != nil {
		return err
	}

	if reindexed > 0 && !isJSONOutput() {
		fmt.Fprintf(os.Stderr, "Refreshed %d stale file(s)\n\n", reindexed)
	}

	return nil
}

func init() {
	queryCmd.Flags().BoolP("list", "l", false, "List saved queries")
	queryCmd.Flags().Bool("refresh", false, "Refresh stale files before query")

	// query add flags
	queryAddCmd.Flags().StringSlice("traits", nil, "Traits to query (comma-separated)")
	queryAddCmd.Flags().StringSlice("types", nil, "Types to query (comma-separated)")
	queryAddCmd.Flags().StringSlice("filter", nil, "Filter in key=value format (repeatable)")
	queryAddCmd.Flags().String("description", "", "Human-readable description")

	queryCmd.AddCommand(queryAddCmd)
	queryCmd.AddCommand(queryRemoveCmd)
	rootCmd.AddCommand(queryCmd)
}
