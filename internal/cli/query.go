package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

// saveLastResultsFromTraits builds and saves a LastResults from trait query results.
// Returns the built LastResults for use in output formatting.
func saveLastResultsFromTraits(vaultPath, queryStr string, results []model.Trait) *lastresults.LastResults {
	modelResults := make([]model.Result, len(results))
	for i, r := range results {
		modelResults[i] = r
	}

	lr, err := lastresults.NewFromResults(lastresults.SourceQuery, queryStr, "", modelResults)
	if err != nil {
		return nil
	}

	_ = lastresults.Write(vaultPath, lr)
	return lr
}

// saveLastResultsFromObjects builds and saves a LastResults from object query results.
// Returns the built LastResults for use in output formatting.
func saveLastResultsFromObjects(vaultPath, queryStr string, results []model.Object) *lastresults.LastResults {
	modelResults := make([]model.Result, len(results))
	for i, r := range results {
		modelResults[i] = r
	}

	lr, err := lastresults.NewFromResults(lastresults.SourceQuery, queryStr, "", modelResults)
	if err != nil {
		return nil
	}

	_ = lastresults.Write(vaultPath, lr)
	return lr
}

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

// printObjectTable prints object results in a tabular format with field columns
func printObjectTable(results []model.Object, sch *schema.Schema) {
	if len(results) == 0 {
		return
	}

	// Get the type definition to determine columns
	var typeDef *schema.TypeDefinition
	var fieldColumns []string
	nameField := ""

	if len(results) > 0 && sch != nil {
		typeDef = sch.Types[results[0].Type]
	}

	if typeDef != nil {
		nameField = typeDef.NameField
		// Collect field names (excluding name field) in sorted order
		for fieldName := range typeDef.Fields {
			if fieldName != nameField {
				fieldColumns = append(fieldColumns, fieldName)
			}
		}
		sort.Strings(fieldColumns)
	}

	// Calculate number column width
	numWidth := len(fmt.Sprintf("%d", len(results)))
	if numWidth < 2 {
		numWidth = 2
	}

	// Calculate column widths
	nameWidth := 4 // "NAME"
	fieldWidths := make(map[string]int)
	locationWidth := 8 // "LOCATION"

	for _, col := range fieldColumns {
		fieldWidths[col] = len(col)
	}

	for _, r := range results {
		name := filepath.Base(r.ID)
		if len(name) > nameWidth {
			nameWidth = len(name)
		}

		loc := formatLocationLinkSimple(r.FilePath, r.LineStart)
		if len(loc) > locationWidth {
			locationWidth = len(loc)
		}

		for _, col := range fieldColumns {
			valStr := formatFieldValueSimple(r.Fields[col])
			if len(valStr) > fieldWidths[col] {
				fieldWidths[col] = len(valStr)
			}
		}
	}

	// Cap widths to prevent overly wide columns (except location)
	if nameWidth > 25 {
		nameWidth = 25
	}
	for col := range fieldWidths {
		if fieldWidths[col] > 20 {
			fieldWidths[col] = 20
		}
	}
	// Don't cap location width - show full paths for navigation

	// Print header with # column
	header := fmt.Sprintf("%*s", numWidth, "#")
	divider := strings.Repeat("─", numWidth)
	header += "  " + fmt.Sprintf("%-*s", nameWidth, "NAME")
	divider += "  " + strings.Repeat("─", nameWidth)
	for _, col := range fieldColumns {
		header += "  " + fmt.Sprintf("%-*s", fieldWidths[col], strings.ToUpper(col))
		divider += "  " + strings.Repeat("─", fieldWidths[col])
	}
	header += "  " + fmt.Sprintf("%-*s", locationWidth, "LOCATION")
	divider += "  " + strings.Repeat("─", locationWidth)

	fmt.Println(ui.Muted.Render(header))
	fmt.Println(ui.Muted.Render(divider))

	// Print rows with numbers
	for i, r := range results {
		numStr := fmt.Sprintf("%*d", numWidth, i+1)
		name := filepath.Base(r.ID)
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}
		row := ui.Muted.Render(numStr) + "  " + fmt.Sprintf("%-*s", nameWidth, name)

		for _, col := range fieldColumns {
			valStr := formatFieldValueSimple(r.Fields[col])
			if valStr == "" {
				valStr = "-"
			}
			if len(valStr) > fieldWidths[col] {
				valStr = valStr[:fieldWidths[col]-1] + "…"
			}
			row += "  " + fmt.Sprintf("%-*s", fieldWidths[col], valStr)
		}

		// Location is not truncated - show full path for easy navigation
		loc := formatLocationLinkSimple(r.FilePath, r.LineStart)
		row += "  " + ui.Muted.Render(loc)

		fmt.Println(row)
	}
}

// formatFieldValueSimple formats a field value as a simple string for table display
func formatFieldValueSimple(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return shortenRefIfNeeded(v)
	case []interface{}:
		strs := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				strs = append(strs, shortenRefIfNeeded(s))
			}
		}
		return strings.Join(strs, ", ")
	case bool:
		if v {
			return "yes"
		}
		return "no"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// shortenRefIfNeeded shortens a reference path to just the name if it looks like a ref.
// For paths like "objects/companies/cursor" or "people/alice", returns just "cursor" or "alice".
// Only shortens if the path has multiple segments (contains /).
func shortenRefIfNeeded(s string) string {
	// If it doesn't contain a slash, it's not a path - return as-is
	if !strings.Contains(s, "/") {
		return s
	}

	// Get the last path component (the name)
	name := filepath.Base(s)

	// Remove .md extension if present
	name = strings.TrimSuffix(name, ".md")

	return name
}

var queryCmd = &cobra.Command{
	Use:   "query <query-string>",
	Short: "Run a query using the Raven query language",
	Long: `Query objects or traits using the Raven query language.

Query types:
  object:<type> [predicates]   Query objects of a type
  trait:<name> [predicates]    Query traits by name

Predicates for object queries:
  .field==value      Field equals value
  exists(.field)     Field exists (has a value)
  !.field==value     Field does not equal value
  has(trait:...)        Has a trait matching nested trait query
  encloses(trait:...)   Has a trait in subtree (self or descendants)
  parent(object:...)    Direct parent matches nested object query
  ancestor(object:...)  Any ancestor matches nested object query
  child(object:...)     Has child matching nested object query
  descendant(object:...) Has descendant matching nested object query
  refs([[target]])      References a specific target
  refs(object:...)      References an object matching nested object query
  refd([[source]])      Referenced by a specific source
  refd(object:...)      Referenced by an object matching nested object query
  refd(trait:...)       Referenced by a trait matching nested trait query
  content("term")       Full-text search on object content

Predicates for trait queries:
  .value==val      Trait value equals val
  on(object:...)       Direct parent matches nested object query
  within(object:...)   Any ancestor matches nested object query
  at(trait:...)        Co-located with trait matching nested trait query
  refs([[target]])     Line contains reference to target
  refs(object:...)     Line references an object matching nested object query
  content("term")      Line content contains term

Boolean operators:
  !pred            NOT
  pred1 pred2      AND (space-separated)
  pred1 | pred2    OR

Examples:
  rvn query "object:project .status==active"
  rvn query "object:meeting has(trait:due)"
  rvn query "trait:due .value==past"
  rvn query "trait:todo content(\"my task\")"
  rvn query "trait:highlight on(object:book .status==reading)"
  rvn query tasks                    # Run saved query
  rvn query --list                   # List saved queries`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Handle --pipe/--no-pipe flags
		if pipeFlag, _ := cmd.Flags().GetBool("pipe"); pipeFlag {
			t := true
			SetPipeFormat(&t)
		} else if noPipeFlag, _ := cmd.Flags().GetBool("no-pipe"); noPipeFlag {
			f := false
			SetPipeFormat(&f)
		}

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
			return runFullQueryWithOptions(db, vaultPath, queryStr, start, sch, idsOnly, vaultCfg.DailyDirectory)
		}

		// Check if this is a saved query
		if savedQuery, ok := vaultCfg.Queries[queryStr]; ok {
			return runSavedQueryWithOptions(db, vaultPath, savedQuery, queryStr, start, sch, idsOnly, vaultCfg.DailyDirectory)
		}

		// Unknown query - provide helpful error
		suggestion := buildUnknownQuerySuggestion(db, queryStr, vaultCfg.DailyDirectory, sch)
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			suggestion)
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
	executor.SetSchema(sch)

	// Handle trait queries separately - they operate on traits, not objects
	if q.Type == query.QueryTypeTrait {
		return runTraitQueryWithApply(executor, vaultPath, queryStr, q, applyCmd, applyOperationArgs, sch, vaultCfg, confirm)
	}

	// Object query - collect object IDs
	var ids []string
	results, err := executor.ExecuteObjectQuery(q)
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
	results, err := executor.ExecuteTraitQuery(q)
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
	case "update":
		return applyUpdateTraitFromQuery(vaultPath, results, applyArgs, sch, vaultCfg, confirm)
	case "delete", "add", "move", "set":
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("'%s' is not supported for trait queries", applyCmd),
			"For trait queries, use: --apply \"update value=<new_value>\"")
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"For trait queries, use: --apply \"update value=<new_value>\"")
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
		if err := previewSetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg); err != nil {
			return err
		}
		if promptForConfirm("Apply changes?") {
			return applySetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg)
		}
		return nil
	}
	return applySetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg)
}

// applyDeleteFromQuery applies delete operation from query results.
func applyDeleteFromQuery(vaultPath string, ids []string, warnings []Warning, vaultCfg *config.VaultConfig, confirm bool) error {
	if !confirm {
		if err := previewDeleteBulk(vaultPath, ids, warnings, vaultCfg); err != nil {
			return err
		}
		if promptForConfirm("Apply changes?") {
			return applyDeleteBulk(vaultPath, ids, warnings, vaultCfg)
		}
		return nil
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
		if err := previewAddBulk(vaultPath, ids, line, warnings, vaultCfg); err != nil {
			return err
		}
		if promptForConfirm("Apply changes?") {
			return applyAddBulk(vaultPath, ids, line, warnings, vaultCfg)
		}
		return nil
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
		if err := previewMoveBulk(vaultPath, ids, destination, warnings, vaultCfg); err != nil {
			return err
		}
		if promptForConfirm("Apply changes?") {
			return applyMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
		}
		return nil
	}
	return applyMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
}

func runFullQueryWithOptions(db *index.Database, vaultPath, queryStr string, start time.Time, sch *schema.Schema, idsOnly bool, dailyDir string) error {
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
	executor.SetSchema(sch)

	elapsed := time.Since(start).Milliseconds()

	if q.Type == query.QueryTypeObject {
		results, err := executor.ExecuteObjectQuery(q)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		// Save last results for numbered references (skip for --ids mode)
		if !idsOnly {
			saveLastResultsFromObjects(vaultPath, queryStr, results)
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
					"num":       i + 1, // 1-indexed for user reference
					"id":        r.ID,
					"type":      r.Type,
					"fields":    r.Fields,
					"file_path": r.FilePath,
					"line":      r.LineStart,
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

		// Check for pipe mode
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForObjectResults(results))
			return nil
		}

		// Human-readable output
		printQueryObjectResults(queryStr, q.TypeName, results, sch)
		return nil
	}

	// Trait query
	results, err := executor.ExecuteTraitQuery(q)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	// Save last results for numbered references (skip for --ids mode)
	if !idsOnly {
		saveLastResultsFromTraits(vaultPath, queryStr, results)
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
				"num":        i + 1, // 1-indexed for user reference
				"id":         r.ID,
				"trait_type": r.TraitType,
				"value":      r.Value,
				"content":    r.Content,
				"file_path":  r.FilePath,
				"line":       r.Line,
				"object_id":  r.ParentObjectID,
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

	// Check for pipe mode
	if ShouldUsePipeFormat() {
		WritePipeableList(os.Stdout, pipeItemsForTraitResults(results))
		return nil
	}

	// Human-readable output
	printQueryTraitResults(queryStr, q.TypeName, results)
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

func runSavedQueryWithOptions(db *index.Database, vaultPath string, q *config.SavedQuery, name string, start time.Time, sch *schema.Schema, idsOnly bool, dailyDir string) error {
	if q.Query == "" {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' has no query defined", name), "")
	}

	// Just run the query string through the normal query parser
	return runFullQueryWithOptions(db, vaultPath, q.Query, start, sch, idsOnly, dailyDir)
}

var queryAddCmd = &cobra.Command{
	Use:   "add <name> <query-string>",
	Short: "Add a saved query to raven.yaml",
	Long: `Add a new saved query to raven.yaml.

The query string uses the Raven query language (same as 'rvn query "..."').

Examples:
  rvn query add tasks "trait:due"
  rvn query add overdue "trait:due .value==past"
  rvn query add active-projects "object:project .status==active"
  rvn query add urgent "trait:due .value==this-week|past" --description "Due soon or overdue"`,
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
func joinQueryArgs(args []string) string {
	if len(args) == 1 {
		return args[0]
	}

	return strings.Join(args, " ")
}

func init() {
	queryCmd.Flags().BoolP("list", "l", false, "List saved queries")
	queryCmd.Flags().Bool("refresh", false, "Refresh stale files before query")
	queryCmd.Flags().Bool("ids", false, "Output only object/trait IDs, one per line (for piping)")
	queryCmd.Flags().StringArray("apply", nil, "Apply a bulk operation to query results (format: command args...)")
	queryCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")
	queryCmd.Flags().Bool("pipe", false, "Force pipe-friendly output format")
	queryCmd.Flags().Bool("no-pipe", false, "Force human-readable output format")

	// query add flags
	queryAddCmd.Flags().String("description", "", "Human-readable description")

	queryCmd.AddCommand(queryAddCmd)
	queryCmd.AddCommand(queryRemoveCmd)
	rootCmd.AddCommand(queryCmd)
}
