package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

// tableRow represents a row in the output table
type tableRow struct {
	name     string // Object name/slug
	location string // File path and line number
}

// traitTableRow represents a row for trait output (content-first)
type traitTableRow struct {
	content  string // The task/content text
	traits   string // Trait annotations like @due(2025-01-01)
	location string // File path and line number
}

// printTable prints rows as a nicely formatted table with two columns
func printTable(rows []tableRow) {
	if len(rows) == 0 {
		return
	}

	// Calculate max width for name column
	maxNameWidth := 4 // minimum width for "NAME"
	for _, row := range rows {
		if len(row.name) > maxNameWidth {
			maxNameWidth = len(row.name)
		}
	}

	// Cap name width at a reasonable max to prevent super wide tables
	if maxNameWidth > 40 {
		maxNameWidth = 40
	}

	// Print header
	fmt.Printf("%-*s  %s\n", maxNameWidth, "NAME", "LOCATION")
	fmt.Printf("%s  %s\n", strings.Repeat("─", maxNameWidth), strings.Repeat("─", 30))

	// Print rows
	for _, row := range rows {
		name := row.name
		if len(name) > maxNameWidth {
			name = name[:maxNameWidth-1] + "…"
		}
		fmt.Printf("%-*s  %s\n", maxNameWidth, name, row.location)
	}
}

// printTraitTable prints trait results in a content-first format
func printTraitTable(rows []traitTableRow) {
	if len(rows) == 0 {
		return
	}

	// Fixed column widths for consistent readable output
	const contentWidth = 65
	const traitWidth = 18

	// Print header
	fmt.Printf("%-*s  %-*s  %s\n", contentWidth, "CONTENT", traitWidth, "TRAITS", "LOCATION")
	fmt.Printf("%s  %s  %s\n",
		strings.Repeat("─", contentWidth),
		strings.Repeat("─", traitWidth),
		strings.Repeat("─", 25))

	// Print rows
	for _, row := range rows {
		content := truncateText(row.content, contentWidth)
		traits := row.traits
		if len(traits) > traitWidth {
			traits = traits[:traitWidth-1] + "…"
		}
		fmt.Printf("%-*s  %-*s  %s\n", contentWidth, content, traitWidth, traits, row.location)
	}
}

// truncateText truncates text at a word boundary if possible
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Try to truncate at a word boundary
	truncated := text[:maxLen-3]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		// Found a space in the second half, use it
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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

		// Load schema for validation
		sch, schemaErr := schema.Load(vaultPath)
		if schemaErr != nil {
			// Schema load failure is not fatal - continue without validation
			sch = nil
		}

		// Check if this is a full query string (starts with object: or trait:)
		if strings.HasPrefix(queryStr, "object:") || strings.HasPrefix(queryStr, "trait:") {
			return runFullQueryWithSchema(db, queryStr, start, sch)
		}

		// Check if this is a saved query
		if savedQuery, ok := vaultCfg.Queries[queryStr]; ok {
			return runSavedQueryWithJSON(db, savedQuery, queryStr, start, sch)
		}

		// Unknown query - provide helpful error
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			"Queries must start with 'object:' or 'trait:', or be a saved query name. Run 'rvn query --list' to see saved queries.")
	},
}

func runFullQuery(db *index.Database, queryStr string, start time.Time) error {
	return runFullQueryWithSchema(db, queryStr, start, nil)
}

func runFullQueryWithSchema(db *index.Database, queryStr string, start time.Time, sch *schema.Schema) error {
	// Parse the query
	q, err := query.Parse(queryStr)
	if err != nil {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("parse error: %v", err), "")
	}

	// Validate against schema if available
	if sch != nil {
		validator := query.NewValidator(sch)
		if err := validator.Validate(q); err != nil {
			if ve, ok := err.(*query.ValidationError); ok {
				return handleErrorMsg(ErrQueryInvalid, ve.Message, ve.Suggestion)
			}
			return handleErrorMsg(ErrQueryInvalid, err.Error(), "")
		}
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

		// Build table rows
		rows := make([]tableRow, len(results))
		for i, r := range results {
			// Use the filename without extension as the name
			name := filepath.Base(r.ID)
			rows[i] = tableRow{
				name:     name,
				location: fmt.Sprintf("%s:%d", r.FilePath, r.LineStart),
			}
		}
		printTable(rows)
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

	// Build table rows
	rows := make([]traitTableRow, len(results))
	for i, r := range results {
		// Build trait string
		traitStr := "@" + r.TraitType
		if r.Value != nil && *r.Value != "" {
			traitStr += fmt.Sprintf("(%s)", *r.Value)
		}

		rows[i] = traitTableRow{
			content:  r.Content,
			traits:   traitStr,
			location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
		}
	}
	printTraitTable(rows)
	return nil
}

func listSavedQueries(vaultCfg *config.VaultConfig, start time.Time) error {
	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		var queries []SavedQueryInfo
		for name, q := range vaultCfg.Queries {
			queries = append(queries, SavedQueryInfo{
				Name:        name,
				Query:       q.Query,
				Description: q.Description,
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
			desc = q.Query
		}
		fmt.Printf("  %-16s %s\n", name, desc)
	}
	return nil
}

func runSavedQueryWithJSON(db *index.Database, q *config.SavedQuery, name string, start time.Time, sch *schema.Schema) error {
	if q.Query == "" {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' has no query defined", name), "")
	}

	// Just run the query string through the normal query parser
	return runFullQueryWithSchema(db, q.Query, start, sch)
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

	// Build table rows
	rows := make([]traitTableRow, 0, len(order))
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

		rows = append(rows, traitTableRow{
			content:  content,
			traits:   strings.Join(traitStrs, " "),
			location: fmt.Sprintf("%s:%d", key.filePath, key.line),
		})
	}

	printTraitTable(rows)
}

var queryAddCmd = &cobra.Command{
	Use:   "add <name> <query-string>",
	Short: "Add a saved query to raven.yaml",
	Long: `Add a new saved query to raven.yaml.

The query string uses the Raven query language (same as 'rvn query "..."').

Examples:
  rvn query add tasks "trait:due"
  rvn query add overdue "trait:due value:past"
  rvn query add active-projects "object:project .status:active"
  rvn query add urgent "trait:due value:this-week|past" --description "Due soon or overdue"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		queryName := args[0]
		queryStr := args[1]
		description, _ := cmd.Flags().GetString("description")

		// Validate the query string by parsing it
		_, err := query.Parse(queryStr)
		if err != nil {
			return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("invalid query: %v", err), "")
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
			Query:       queryStr,
			Description: description,
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

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":        queryName,
				"query":       queryStr,
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
	queryAddCmd.Flags().String("description", "", "Human-readable description")

	queryCmd.AddCommand(queryAddCmd)
	queryCmd.AddCommand(queryRemoveCmd)
	rootCmd.AddCommand(queryCmd)
}
