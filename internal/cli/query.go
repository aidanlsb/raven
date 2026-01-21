package cli

import (
	"errors"
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
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

func dedupePreserveOrder(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

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

	// Calculate max width for name column (use visible length for ANSI-aware sizing)
	maxNameWidth := 4 // minimum width for "NAME"
	for _, row := range rows {
		visibleLen := ui.VisibleLen(row.name)
		if visibleLen > maxNameWidth {
			maxNameWidth = visibleLen
		}
	}

	// Cap name width at a reasonable max to prevent super wide tables
	if maxNameWidth > 40 {
		maxNameWidth = 40
	}

	// Print header
	fmt.Printf("%s  %s\n", ui.Muted.Render(fmt.Sprintf("%-*s", maxNameWidth, "NAME")), ui.Muted.Render("LOCATION"))
	fmt.Printf("%s  %s\n", ui.Muted.Render(strings.Repeat("─", maxNameWidth)), ui.Muted.Render(strings.Repeat("─", 30)))

	// Print rows
	for _, row := range rows {
		name := row.name
		if ui.VisibleLen(name) > maxNameWidth {
			name = name[:maxNameWidth-1] + "…"
		}
		fmt.Printf("%s  %s\n", ui.PadRight(name, maxNameWidth), row.location)
	}
}

// printTraitTable prints trait results in a content-first format
func printTraitTable(rows []traitTableRow) {
	if len(rows) == 0 {
		return
	}

	// Fixed column widths for consistent readable output
	const contentWidth = 55
	const traitWidth = 18

	// Print header
	fmt.Printf("%s  %s  %s\n",
		ui.Muted.Render(fmt.Sprintf("%-*s", contentWidth, "CONTENT")),
		ui.Muted.Render(fmt.Sprintf("%-*s", traitWidth, "TRAITS")),
		ui.Muted.Render("LOCATION"))
	fmt.Printf("%s  %s  %s\n",
		ui.Muted.Render(strings.Repeat("─", contentWidth)),
		ui.Muted.Render(strings.Repeat("─", traitWidth)),
		ui.Muted.Render(strings.Repeat("─", 30)))

	// Print rows
	for _, row := range rows {
		content := truncateText(row.content, contentWidth)
		// Highlight any traits in the content
		content = ui.HighlightTraits(content)
		traits := row.traits

		// Use PadRight to handle ANSI escape codes correctly
		fmt.Printf("%s  %s  %s\n",
			ui.PadRight(content, contentWidth),
			ui.PadRight(traits, traitWidth),
			row.location)
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

var queryCmd = &cobra.Command{
	Use:   "query <query-string>",
	Short: "Run a query using the Raven query language",
	Long: `Query objects or traits using the Raven query language.

Query types:
  object:<type> [predicates]   Query objects of a type
  trait:<name> [predicates]    Query traits by name

Predicates for object queries:
  .field==value    Field equals value
  .field==*        Field exists
  !.field==value   Field does not equal value
  has:{trait:...}  Has a trait matching subquery
  parent:{object:...}   Direct parent matches subquery
  ancestor:{object:...} Any ancestor matches subquery
  child:{object:...}    Has child matching subquery

Predicates for trait queries:
  value==val       Trait value equals val
  on:{object:...}      Direct parent matches subquery
  within:{object:...}  Any ancestor matches subquery

Boolean operators:
  !pred            NOT
  pred1 pred2      AND (space-separated)
  pred1 | pred2    OR

Subqueries use curly braces:
  has:{trait:due value==past}
  on:{object:project .status==active}

Examples:
  rvn query "object:project .status==active"
  rvn query "object:meeting has:{trait:due}"
  rvn query "trait:due value==past"
  rvn query trait:todo content:"my task"
  rvn query "trait:highlight on:{object:book .status==reading}"
  rvn query tasks                    # Run saved query
  rvn query --list                   # List saved queries`,
	Args: cobra.ArbitraryArgs,
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

		// Join multiple args with spaces - allows running without quoting the whole query
		// e.g., `rvn query trait:todo content:"my task"` works the same as
		//       `rvn query 'trait:todo content:"my task"'`
		queryStr := joinQueryArgs(args)

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()
		db.SetDailyDirectory(vaultCfg.DailyDirectory)

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

		// Get --ids flag
		idsOnly, _ := cmd.Flags().GetBool("ids")

		// Get --apply flag
		applyArgs, _ := cmd.Flags().GetStringArray("apply")
		confirmApply, _ := cmd.Flags().GetBool("confirm")

		// If --apply is set, run query and apply bulk operation
		if len(applyArgs) > 0 {
			return runQueryWithApply(db, vaultPath, queryStr, vaultCfg, sch, applyArgs, confirmApply, start)
		}

		// Check if this is a full query string (starts with object: or trait:)
		if strings.HasPrefix(queryStr, "object:") || strings.HasPrefix(queryStr, "trait:") {
			return runFullQueryWithOptions(db, queryStr, start, sch, idsOnly, vaultCfg.DailyDirectory)
		}

		// Check if this is a saved query
		if savedQuery, ok := vaultCfg.Queries[queryStr]; ok {
			return runSavedQueryWithOptions(db, savedQuery, queryStr, start, sch, idsOnly, vaultCfg.DailyDirectory)
		}

		// Unknown query - provide helpful error
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			"Queries must start with 'object:' or 'trait:', or be a saved query name. Run 'rvn query --list' to see saved queries.")
	},
}

// runQueryWithApply runs a query and applies a bulk operation to the results.
func runQueryWithApply(db *index.Database, vaultPath, queryStr string, vaultCfg *config.VaultConfig, sch *schema.Schema, applyArgs []string, confirm bool, start time.Time) error {
	// Resolve the query string (could be saved query)
	actualQueryStr := queryStr
	if savedQuery, ok := vaultCfg.Queries[queryStr]; ok {
		actualQueryStr = savedQuery.Query
	}

	if !strings.HasPrefix(actualQueryStr, "object:") && !strings.HasPrefix(actualQueryStr, "trait:") {
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			"Queries must start with 'object:' or 'trait:', or be a saved query name.")
	}

	// Parse the apply command first to validate
	applyStr := strings.Join(applyArgs, " ")
	applyParts := strings.Fields(applyStr)

	if len(applyParts) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no apply command specified", "Use --apply <command> [args...]")
	}

	applyCmd := applyParts[0]
	applyOperationArgs := applyParts[1:]

	// Parse the query
	q, err := query.Parse(actualQueryStr)
	if err != nil {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("parse error: %v", err), "")
	}

	// Execute the query
	executor := query.NewExecutor(db.DB())
	if res, err := db.Resolver(index.ResolverOptions{DailyDirectory: vaultCfg.DailyDirectory}); err == nil {
		executor.SetResolver(res)
	}

	// Handle trait queries separately - they operate on traits, not objects
	if q.Type == query.QueryTypeTrait {
		return runTraitQueryWithApply(executor, vaultPath, queryStr, q, applyCmd, applyOperationArgs, sch, vaultCfg, confirm)
	}

	// Object query - collect object IDs
	var ids []string
	results, err := executor.ExecuteObjectQueryWithPipeline(q)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	ids = dedupePreserveOrder(ids)

	if len(ids) == 0 {
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"preview": !confirm,
				"action":  applyCmd,
				"items":   []interface{}{},
				"total":   0,
			}, &Meta{Count: 0})
			return nil
		}
		fmt.Printf("No results found for query: %s\n", queryStr)
		return nil
	}

	// Filter embedded IDs for operations that don't support them
	// Note: "set" now supports embedded objects, so we pass all IDs to it
	var fileIDs []string
	var embedded []string
	for _, id := range ids {
		if IsEmbeddedID(id) {
			embedded = append(embedded, id)
		} else {
			fileIDs = append(fileIDs, id)
		}
	}

	// Build warnings for embedded objects (only for commands that don't support them)
	var warnings []Warning
	if applyCmd != "set" {
		if w := BuildEmbeddedSkipWarning(embedded); w != nil {
			warnings = append(warnings, *w)
		}
	}

	// Dispatch to the appropriate bulk operation
	switch applyCmd {
	case "set":
		// Set supports embedded objects, so pass all IDs
		return applySetFromQuery(vaultPath, ids, applyOperationArgs, warnings, sch, vaultCfg, confirm)
	case "delete":
		return applyDeleteFromQuery(vaultPath, fileIDs, warnings, vaultCfg, confirm)
	case "add":
		return applyAddFromQuery(vaultPath, fileIDs, applyOperationArgs, warnings, vaultCfg, confirm)
	case "move":
		return applyMoveFromQuery(vaultPath, fileIDs, applyOperationArgs, warnings, vaultCfg, confirm)
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"Supported commands: set, delete, add, move")
	}
}

// runTraitQueryWithApply handles --apply for trait queries.
// Trait queries operate on traits, not objects.
func runTraitQueryWithApply(executor *query.Executor, vaultPath, queryStr string, q *query.Query, applyCmd string, applyArgs []string, sch *schema.Schema, vaultCfg *config.VaultConfig, confirm bool) error {
	// Execute the trait query
	results, err := executor.ExecuteTraitQueryWithPipeline(q)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	if len(results) == 0 {
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"preview": !confirm,
				"action":  applyCmd,
				"items":   []interface{}{},
				"total":   0,
			}, &Meta{Count: 0})
			return nil
		}
		fmt.Printf("No results found for query: %s\n", queryStr)
		return nil
	}

	// Dispatch to trait-specific operations
	switch applyCmd {
	case "set":
		return applySetTraitFromQuery(vaultPath, results, applyArgs, sch, vaultCfg, confirm)
	case "delete", "add", "move":
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("'%s' is not supported for trait queries", applyCmd),
			"For trait queries, use: --apply \"set value=<new_value>\"")
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"For trait queries, use: --apply \"set value=<new_value>\"")
	}
}

// applySetFromQuery applies set operation from query results.
func applySetFromQuery(vaultPath string, ids []string, args []string, warnings []Warning, sch *schema.Schema, vaultCfg *config.VaultConfig, confirm bool) error {
	// Parse field=value arguments
	updates := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("invalid field format: %s", arg),
				"Use format: field=value")
		}
		updates[parts[0]] = parts[1]
	}

	if len(updates) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: --apply set field=value...")
	}

	if !confirm {
		return previewSetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg)
	}
	return applySetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg)
}

// applyDeleteFromQuery applies delete operation from query results.
func applyDeleteFromQuery(vaultPath string, ids []string, warnings []Warning, vaultCfg *config.VaultConfig, confirm bool) error {
	if !confirm {
		return previewDeleteBulk(vaultPath, ids, warnings, vaultCfg)
	}
	return applyDeleteBulk(vaultPath, ids, warnings, vaultCfg)
}

// applyAddFromQuery applies add operation from query results.
func applyAddFromQuery(vaultPath string, ids []string, args []string, warnings []Warning, vaultCfg *config.VaultConfig, confirm bool) error {
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no text to add", "Usage: --apply add <text>")
	}

	text := strings.Join(args, " ")
	captureCfg := vaultCfg.GetCaptureConfig()
	line := formatCaptureLine(text, captureCfg)

	if !confirm {
		return previewAddBulk(vaultPath, ids, line, warnings, vaultCfg)
	}
	return applyAddBulk(vaultPath, ids, line, warnings, vaultCfg)
}

// applyMoveFromQuery applies move operation from query results.
func applyMoveFromQuery(vaultPath string, ids []string, args []string, warnings []Warning, vaultCfg *config.VaultConfig, confirm bool) error {
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no destination provided", "Usage: --apply move <destination-directory/>")
	}

	destination := args[0]
	if !strings.HasSuffix(destination, "/") {
		return handleErrorMsg(ErrInvalidInput,
			"destination must be a directory (end with /)",
			"Example: --apply move archive/projects/")
	}

	if !confirm {
		return previewMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
	}
	return applyMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
}

func runFullQueryWithOptions(db *index.Database, queryStr string, start time.Time, sch *schema.Schema, idsOnly bool, dailyDir string) error {
	// Parse the query
	q, err := query.Parse(queryStr)
	if err != nil {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("parse error: %v", err), "")
	}

	// Validate against schema if available
	if sch != nil {
		validator := query.NewValidator(sch)
		if err := validator.Validate(q); err != nil {
			var ve *query.ValidationError
			if errors.As(err, &ve) {
				return handleErrorMsg(ErrQueryInvalid, ve.Message, ve.Suggestion)
			}
			return handleErrorMsg(ErrQueryInvalid, err.Error(), "")
		}
	}

	executor := query.NewExecutor(db.DB())
	if res, err := db.Resolver(index.ResolverOptions{DailyDirectory: dailyDir}); err == nil {
		executor.SetResolver(res)
	}

	elapsed := time.Since(start).Milliseconds()

	if q.Type == query.QueryTypeObject {
		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		// --ids mode: output just IDs, one per line
		if idsOnly {
			if isJSONOutput() {
				ids := make([]string, len(results))
				for i, r := range results {
					ids[i] = r.ID
				}
				outputSuccess(map[string]interface{}{
					"ids": ids,
				}, &Meta{Count: len(ids), QueryTimeMs: elapsed})
				return nil
			}
			for _, r := range results {
				fmt.Println(r.ID)
			}
			return nil
		}

		if isJSONOutput() {
			items := make([]map[string]interface{}, len(results))
			for i, r := range results {
				item := map[string]interface{}{
					"id":        r.ID,
					"type":      r.Type,
					"fields":    r.Fields,
					"file_path": r.FilePath,
					"line":      r.LineStart,
				}
				// Include computed values from pipeline if present
				if len(r.Computed) > 0 {
					item["computed"] = r.Computed
				}
				items[i] = item
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
			fmt.Println(ui.Starf("No objects found for: %s", queryStr))
			return nil
		}

		fmt.Printf("%s %s\n\n", ui.Header(q.TypeName), ui.Hint(fmt.Sprintf("(%d)", len(results))))

		// Build table rows
		rows := make([]tableRow, len(results))
		for i, r := range results {
			// Use the filename without extension as the name
			name := filepath.Base(r.ID)
			rows[i] = tableRow{
				name:     name,
				location: formatLocationLinkSimple(r.FilePath, r.LineStart),
			}
		}
		printTable(rows)
		return nil
	}

	// Trait query
	results, err := executor.ExecuteTraitQueryWithPipeline(q)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	// --ids mode: output just trait IDs, one per line
	if idsOnly {
		ids := make([]string, 0, len(results))
		for _, r := range results {
			ids = append(ids, r.ID)
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"ids": ids,
			}, &Meta{Count: len(ids), QueryTimeMs: elapsed})
			return nil
		}
		for _, id := range ids {
			fmt.Println(id)
		}
		return nil
	}

	if isJSONOutput() {
		items := make([]map[string]interface{}, len(results))
		for i, r := range results {
			item := map[string]interface{}{
				"id":         r.ID,
				"trait_type": r.TraitType,
				"value":      r.Value,
				"content":    r.Content,
				"file_path":  r.FilePath,
				"line":       r.Line,
				"object_id":  r.ParentObjectID,
			}
			// Include computed values from pipeline if present
			if len(r.Computed) > 0 {
				item["computed"] = r.Computed
			}
			items[i] = item
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
		fmt.Println(ui.Starf("No traits found for: %s", queryStr))
		return nil
	}

	fmt.Printf("%s %s\n\n", ui.Header("@"+q.TypeName), ui.Hint(fmt.Sprintf("(%d)", len(results))))

	// Build table rows
	rows := make([]traitTableRow, len(results))
	for i, r := range results {
		// Build trait string with syntax highlighting
		value := ""
		if r.Value != nil {
			value = *r.Value
		}
		traitStr := ui.Trait(r.TraitType, value)

		rows[i] = traitTableRow{
			content:  r.Content,
			traits:   traitStr,
			location: formatLocationLinkSimple(r.FilePath, r.Line),
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

func runSavedQueryWithOptions(db *index.Database, q *config.SavedQuery, name string, start time.Time, sch *schema.Schema, idsOnly bool, dailyDir string) error {
	if q.Query == "" {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' has no query defined", name), "")
	}

	// Just run the query string through the normal query parser
	return runFullQueryWithOptions(db, q.Query, start, sch, idsOnly, dailyDir)
}

var queryAddCmd = &cobra.Command{
	Use:   "add <name> <query-string>",
	Short: "Add a saved query to raven.yaml",
	Long: `Add a new saved query to raven.yaml.

The query string uses the Raven query language (same as 'rvn query "..."').

Examples:
  rvn query add tasks "trait:due"
  rvn query add overdue "trait:due value==past"
  rvn query add active-projects "object:project .status==active"
  rvn query add urgent "trait:due value==this-week|past" --description "Due soon or overdue"`,
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
			fmt.Println(ui.Checkf("Added query '%s'", queryName))
			fmt.Printf("  Run with: %s\n", ui.Bold.Render("rvn query "+queryName))
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
			fmt.Println(ui.Checkf("Removed query '%s'", queryName))
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
			fmt.Fprintln(os.Stderr, ui.Warning("1 file may be stale. Run 'rvn reindex' or use '--refresh'."))
		} else if staleCount <= 3 {
			fmt.Fprintln(os.Stderr, ui.Warningf("%d files may be stale: %s",
				staleCount, strings.Join(staleness.StaleFiles, ", ")))
			fmt.Fprintf(os.Stderr, "  Run 'rvn reindex' or use '--refresh' to update.\n")
		} else {
			fmt.Fprintln(os.Stderr, ui.Warningf("%d files may be stale. Run 'rvn reindex' or use '--refresh'.", staleCount))
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
			return nil //nolint:nilerr // skip files with errors
		}

		// Check if file needs reindexing
		indexedMtime, err := db.GetFileMtime(result.RelativePath)
		if err == nil && indexedMtime > 0 && result.FileMtime <= indexedMtime {
			return nil // File is up-to-date
		}

		// Reindex this file
		if err := db.IndexDocumentWithMtime(result.Document, sch, result.FileMtime); err != nil {
			return nil //nolint:nilerr // skip files that fail to index
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

// joinQueryArgs joins command-line arguments into a single query string.
// It handles the case where the shell has already processed quotes, e.g.:
//
//	rvn query trait:todo content:"my task"
//
// becomes args ["trait:todo", "content:my task"] after shell processing.
// We need to re-quote the content value so the parser sees content:"my task".
func joinQueryArgs(args []string) string {
	if len(args) == 1 {
		return args[0]
	}

	result := make([]string, len(args))
	for i, arg := range args {
		// Check if this arg is content: with an unquoted value containing spaces
		// Shell would have processed content:"foo bar" into content:foo bar
		if strings.HasPrefix(arg, "content:") || strings.HasPrefix(arg, "!content:") {
			prefix := "content:"
			if strings.HasPrefix(arg, "!") {
				prefix = "!content:"
			}
			value := strings.TrimPrefix(arg, prefix)
			// If value doesn't start with a quote but contains content, re-quote it
			if value != "" && !strings.HasPrefix(value, "\"") {
				arg = prefix + "\"" + value + "\""
			}
		}
		result[i] = arg
	}

	return strings.Join(result, " ")
}

func init() {
	queryCmd.Flags().BoolP("list", "l", false, "List saved queries")
	queryCmd.Flags().Bool("refresh", false, "Refresh stale files before query")
	queryCmd.Flags().Bool("ids", false, "Output only object/trait IDs, one per line (for piping)")
	queryCmd.Flags().StringArray("apply", nil, "Apply a bulk operation to query results (format: command args...)")
	queryCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")

	// query add flags
	queryAddCmd.Flags().String("description", "", "Human-readable description")

	queryCmd.AddCommand(queryAddCmd)
	queryCmd.AddCommand(queryRemoveCmd)
	rootCmd.AddCommand(queryCmd)
}
