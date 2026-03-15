package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
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

		loc := formatLocationLinkSimpleStyled(r.FilePath, r.LineStart, ui.Muted.Render)
		if visible := ui.VisibleLen(loc); visible > locationWidth {
			locationWidth = visible
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
		loc := formatLocationLinkSimpleStyled(r.FilePath, r.LineStart, ui.Muted.Render)
		row += "  " + loc

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

Saved query inputs must be declared with args: in raven.yaml when using {{args.<name>}}.
You can then pass inputs either by position (following args order) or as key=value pairs.

Examples:
  rvn query "object:project .status==active"
  rvn query "object:meeting has(trait:due)"
  rvn query "trait:due .value<today"
  rvn query "trait:todo content(\"my task\")"
  rvn query "trait:highlight on(object:book .status==reading)"
  rvn query tasks                    # Run saved query
  rvn query project-todos raven      # Positional input (args: [project])
  rvn query project-todos project=projects/raven
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

		// MCP sends query_string as a single positional arg. Support
		// "saved-query-name <inputs...>" in that single string.
		args = maybeSplitInlineSavedQueryArgs(args, vaultCfg.Queries)

		queryName := args[0]
		queryStr := ""
		isSavedQuery := false

		if savedQuery, ok := vaultCfg.Queries[queryName]; ok {
			isSavedQuery = true
			declaredArgs, err := querysvc.NormalizeArgs(savedQuery.Args)
			if err != nil {
				return mapQuerySvcError(err)
			}
			if err := querysvc.ValidateInputDeclarations(queryName, savedQuery.Query, declaredArgs); err != nil {
				return mapQuerySvcError(err)
			}
			inputs, err := querysvc.ParseInputs(queryName, args[1:], declaredArgs)
			if err != nil {
				return mapQuerySvcError(err)
			}
			queryStr, err = querysvc.ResolveQueryString(queryName, savedQuery, inputs)
			if err != nil {
				return mapQuerySvcError(err)
			}
		} else {
			// Join multiple args with spaces - allows running without quoting the whole query
			// e.g., `rvn query trait:todo content:"my task"` works the same as
			//       `rvn query 'trait:todo content:"my task"'`
			queryStr = joinQueryArgs(args)
		}

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()
		db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

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
		rt := &readsvc.Runtime{
			VaultPath: vaultPath,
			VaultCfg:  vaultCfg,
			Schema:    sch,
			DB:        db,
		}

		// Get --ids flag
		idsOnly, _ := cmd.Flags().GetBool("ids")
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		countOnly, _ := cmd.Flags().GetBool("count-only")
		if limit < 0 {
			return handleErrorMsg(ErrInvalidInput, "--limit must be >= 0", "Use --limit 0 for no limit")
		}
		if offset < 0 {
			return handleErrorMsg(ErrInvalidInput, "--offset must be >= 0", "Use --offset 0 for no offset")
		}

		// Get --apply flag
		applyArgs, _ := cmd.Flags().GetStringArray("apply")
		confirmApply, _ := cmd.Flags().GetBool("confirm")

		// If --apply is set, run query and apply bulk operation
		if len(applyArgs) > 0 {
			if limit > 0 || offset > 0 || countOnly {
				return handleErrorMsg(
					ErrInvalidInput,
					"--limit, --offset, and --count-only cannot be used with --apply",
					"Remove pagination/count-only flags when using --apply",
				)
			}
			return runQueryWithApply(rt, queryStr, applyArgs, confirmApply, start)
		}

		// Check if this is a full query string (starts with object: or trait:)
		if strings.HasPrefix(queryStr, "object:") || strings.HasPrefix(queryStr, "trait:") {
			return runFullQueryWithOptions(rt, queryStr, start, idsOnly, limit, offset, countOnly)
		}

		if isSavedQuery {
			return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' must start with 'object:' or 'trait:'", queryName), "")
		}

		// Unknown query - provide helpful error
		suggestion := buildUnknownQuerySuggestion(db, queryStr, vaultCfg.GetDailyDirectory(), sch)
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			suggestion)
	},
}

// runQueryWithApply runs a query and applies a bulk operation to the results.
func runQueryWithApply(rt *readsvc.Runtime, queryStr string, applyArgs []string, confirm bool, start time.Time) error {
	if !strings.HasPrefix(queryStr, "object:") && !strings.HasPrefix(queryStr, "trait:") {
		return handleErrorMsg(ErrQueryInvalid,
			fmt.Sprintf("unknown query: %s", queryStr),
			"Queries must start with 'object:' or 'trait:', or be a saved query name.")
	}

	parsedApply, err := querysvc.ParseApplyCommand(applyArgs)
	if err != nil {
		return mapQuerySvcError(err)
	}
	applyCmd := parsedApply.Command
	applyOperationArgs := parsedApply.Args

	result, err := readsvc.ExecuteQuery(rt, readsvc.ExecuteQueryRequest{
		QueryString: queryStr,
	})
	if err != nil {
		return mapExecuteQueryError(queryStr, err)
	}

	// Handle trait queries separately - they operate on traits, not objects.
	if result.QueryType == "trait" {
		return runTraitQueryWithApply(rt.VaultPath, queryStr, result.Traits, applyCmd, applyOperationArgs, rt.Schema, rt.VaultCfg, confirm)
	}

	// Object query - collect object IDs
	var ids []string
	for _, r := range result.Objects {
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
			}, &Meta{Count: 0, QueryTimeMs: time.Since(start).Milliseconds()})
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
		return applySetFromQuery(rt.VaultPath, ids, applyOperationArgs, warnings, rt.Schema, rt.VaultCfg, confirm)
	case "delete":
		return applyDeleteFromQuery(rt.VaultPath, fileIDs, warnings, rt.VaultCfg, confirm)
	case "add":
		return applyAddFromQuery(rt.VaultPath, fileIDs, applyOperationArgs, warnings, rt.VaultCfg, confirm)
	case "move":
		return applyMoveFromQuery(rt.VaultPath, fileIDs, applyOperationArgs, warnings, rt.VaultCfg, confirm)
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"Supported commands: set, delete, add, move")
	}
}

// runTraitQueryWithApply handles --apply for trait queries.
// Trait queries operate on traits, not objects.
func runTraitQueryWithApply(vaultPath, queryStr string, results []model.Trait, applyCmd string, applyArgs []string, sch *schema.Schema, vaultCfg *config.VaultConfig, confirm bool) error {
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
		err := applyUpdateTraitFromQuery(vaultPath, results, applyArgs, sch, vaultCfg, confirm)
		if err != nil {
			var validationErr *traitValueValidationError
			if errors.As(err, &validationErr) {
				return handleErrorMsg(ErrValidationFailed, validationErr.Error(), validationErr.Suggestion())
			}
		}
		return err
	case "delete", "add", "move", "set":
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("'%s' is not supported for trait queries", applyCmd),
			"For trait queries, use: --apply \"update <new_value>\"")
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"For trait queries, use: --apply \"update <new_value>\"")
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

	// set requires schema-aware validation.
	if sch == nil {
		loadedSchema, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
		}
		sch = loadedSchema
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
	line := formatCaptureLine(text)

	if !confirm {
		if err := previewAddBulk(vaultPath, ids, line, "", warnings, vaultCfg); err != nil {
			return err
		}
		if promptForConfirm("Apply changes?") {
			return applyAddBulk(vaultPath, ids, line, "", warnings, vaultCfg)
		}
		return nil
	}
	return applyAddBulk(vaultPath, ids, line, "", warnings, vaultCfg)
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

func runFullQueryWithOptions(rt *readsvc.Runtime, queryStr string, start time.Time, idsOnly bool, limit, offset int, countOnly bool) error {
	result, err := readsvc.ExecuteQuery(rt, readsvc.ExecuteQueryRequest{
		QueryString: queryStr,
		IDsOnly:     idsOnly,
		Limit:       limit,
		Offset:      offset,
		CountOnly:   countOnly,
	})
	if err != nil {
		return mapExecuteQueryError(queryStr, err)
	}

	elapsed := time.Since(start).Milliseconds()

	if result.QueryType == "object" {
		if countOnly {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"query_type": "object",
					"type":       result.TypeName,
					"total":      result.Total,
				}, &Meta{Count: result.Total, QueryTimeMs: elapsed})
				return nil
			}
			fmt.Println(result.Total)
			return nil
		}

		if !idsOnly {
			readsvc.SaveObjectQueryResults(rt.VaultPath, queryStr, result.Objects)
		}

		if idsOnly {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"ids":      result.IDs,
					"total":    result.Total,
					"returned": result.Returned,
					"offset":   result.Offset,
					"limit":    result.Limit,
				}, &Meta{Count: result.Returned, QueryTimeMs: elapsed})
				return nil
			}
			for _, id := range result.IDs {
				fmt.Println(id)
			}
			return nil
		}

		if isJSONOutput() {
			items := make([]map[string]interface{}, len(result.Objects))
			for i, row := range result.Objects {
				items[i] = map[string]interface{}{
					"num":       result.Offset + i + 1,
					"id":        row.ID,
					"type":      row.Type,
					"fields":    row.Fields,
					"file_path": row.FilePath,
					"line":      row.LineStart,
				}
			}
			outputSuccess(map[string]interface{}{
				"query_type": "object",
				"type":       result.TypeName,
				"items":      items,
				"total":      result.Total,
				"returned":   result.Returned,
				"offset":     result.Offset,
				"limit":      result.Limit,
			}, &Meta{Count: result.Returned, QueryTimeMs: elapsed})
			return nil
		}

		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForObjectResults(result.Objects))
			return nil
		}

		printQueryObjectResults(queryStr, result.TypeName, result.Objects, rt.Schema)
		return nil
	}

	if countOnly {
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"query_type": "trait",
				"trait":      result.TypeName,
				"total":      result.Total,
			}, &Meta{Count: result.Total, QueryTimeMs: elapsed})
			return nil
		}
		fmt.Println(result.Total)
		return nil
	}

	if !idsOnly {
		readsvc.SaveTraitQueryResults(rt.VaultPath, queryStr, result.Traits)
	}

	if idsOnly {
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"ids":      result.IDs,
				"total":    result.Total,
				"returned": result.Returned,
				"offset":   result.Offset,
				"limit":    result.Limit,
			}, &Meta{Count: result.Returned, QueryTimeMs: elapsed})
			return nil
		}
		for _, id := range result.IDs {
			fmt.Println(id)
		}
		return nil
	}

	if isJSONOutput() {
		items := make([]map[string]interface{}, len(result.Traits))
		for i, row := range result.Traits {
			items[i] = map[string]interface{}{
				"num":        result.Offset + i + 1,
				"id":         row.ID,
				"trait_type": row.TraitType,
				"value":      row.Value,
				"content":    row.Content,
				"file_path":  row.FilePath,
				"line":       row.Line,
				"object_id":  row.ParentObjectID,
			}
		}
		outputSuccess(map[string]interface{}{
			"query_type": "trait",
			"trait":      result.TypeName,
			"items":      items,
			"total":      result.Total,
			"returned":   result.Returned,
			"offset":     result.Offset,
			"limit":      result.Limit,
		}, &Meta{Count: result.Returned, QueryTimeMs: elapsed})
		return nil
	}

	if ShouldUsePipeFormat() {
		WritePipeableList(os.Stdout, pipeItemsForTraitResults(result.Traits))
		return nil
	}

	printQueryTraitResults(queryStr, result.TypeName, result.Traits)
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
				Args:        q.Args,
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
		if len(q.Args) > 0 {
			fmt.Printf("  %-16s %s (args: %s)\n", name, desc, strings.Join(q.Args, ", "))
			continue
		}
		fmt.Printf("  %-16s %s\n", name, desc)
	}
	return nil
}

var queryAddCmd = &cobra.Command{
	Use:   "add <name> <query-string>",
	Short: "Add a saved query to raven.yaml",
	Long: `Add a new saved query to raven.yaml.

The query string uses the Raven query language (same as 'rvn query "..."').

If the query uses {{args.<name>}}, declare accepted input names with --arg
(repeatable). The order of --arg values defines positional input order.

Examples:
  rvn query add tasks "trait:due"
  rvn query add overdue "trait:due .value<today"
  rvn query add active-projects "object:project .status==active"
  rvn query add project-todos "trait:todo refs([[{{args.project}}]])" --arg project --description "Todos tied to a project"
  rvn query add due-soon "trait:due in(.value,[today,tomorrow])" --description "Due today or tomorrow"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		queryName := args[0]
		queryStr := args[1]
		description, _ := cmd.Flags().GetString("description")
		declaredArgs, err := normalizeSavedQueryArgsForCommand(cmd)
		if err != nil {
			return err
		}

		result, err := querysvc.Add(querysvc.AddRequest{
			VaultPath:   vaultPath,
			Name:        queryName,
			QueryString: queryStr,
			Args:        declaredArgs,
			Description: description,
		})
		if err != nil {
			return mapQuerySvcError(err)
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":        result.Name,
				"query":       result.Query,
				"args":        result.Args,
				"description": result.Description,
			}, nil)
		} else {
			fmt.Println(ui.Checkf("Added query '%s'", result.Name))
			fmt.Printf("  Run with: %s\n", ui.Bold.Render("rvn query "+result.Name))
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
		result, err := querysvc.Remove(querysvc.RemoveRequest{
			VaultPath: vaultPath,
			Name:      queryName,
		})
		if err != nil {
			return mapQuerySvcError(err)
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":    result.Name,
				"removed": result.Removed,
			}, nil)
		} else {
			fmt.Println(ui.Checkf("Removed query '%s'", result.Name))
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

	rt := &readsvc.Runtime{VaultPath: vaultPath, DB: db}
	isStale, staleFiles, err := readsvc.CheckStaleness(rt)
	if err != nil {
		return // Silently fail - don't break queries for staleness check errors
	}

	if isStale {
		staleCount := len(staleFiles)
		if staleCount == 1 {
			fmt.Fprintln(os.Stderr, ui.Warning("1 file may be stale. Run 'rvn reindex' or use '--refresh'."))
		} else if staleCount <= 3 {
			fmt.Fprintln(os.Stderr, ui.Warningf("%d files may be stale: %s",
				staleCount, strings.Join(staleFiles, ", ")))
			fmt.Fprintf(os.Stderr, "  Run 'rvn reindex' or use '--refresh' to update.\n")
		} else {
			fmt.Fprintln(os.Stderr, ui.Warningf("%d files may be stale. Run 'rvn reindex' or use '--refresh'.", staleCount))
		}
		fmt.Fprintln(os.Stderr)
	}
}

// smartReindex performs an incremental reindex of only stale files.
func smartReindex(db *index.Database, vaultPath string) error {
	rt := &readsvc.Runtime{
		VaultPath: vaultPath,
		DB:        db,
	}
	reindexed, err := readsvc.SmartReindex(rt)
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

func maybeSplitInlineSavedQueryArgs(args []string, queries map[string]*config.SavedQuery) []string {
	if len(args) != 1 || len(queries) == 0 {
		return args
	}

	inline := strings.TrimSpace(args[0])
	if inline == "" {
		return args
	}

	// Full query strings should continue through the normal path untouched.
	if strings.HasPrefix(inline, "object:") || strings.HasPrefix(inline, "trait:") {
		return args
	}

	if !strings.ContainsAny(inline, " \t\r\n") {
		return args
	}

	parts, ok := splitInlineSavedQueryInvocation(inline)
	if !ok || len(parts) < 2 {
		return args
	}

	if _, exists := queries[parts[0]]; !exists {
		return args
	}

	return parts
}

// splitInlineSavedQueryInvocation tokenizes one inline query string like:
// "proj-todos raven" or `proj-todos project="raven app"`
// into ["proj-todos", "raven"] / ["proj-todos", "project=raven app"].
//
// Quotes are removed and backslash escapes are resolved (outside single quotes).
// Returns ok=false for invalid quoting/escaping.
func splitInlineSavedQueryInvocation(s string) ([]string, bool) {
	var out []string
	var b strings.Builder

	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}

	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
				continue
			}
			b.WriteRune(r)
			continue
		}

		if inDouble {
			if r == '"' {
				inDouble = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			b.WriteRune(r)
			continue
		}

		switch {
		case r == '\\':
			escaped = true
		case r == '\'':
			inSingle = true
		case r == '"':
			inDouble = true
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
		}
	}

	if escaped || inSingle || inDouble {
		return nil, false
	}

	flush()
	return out, true
}

func normalizeSavedQueryArgsForCommand(cmd *cobra.Command) ([]string, error) {
	rawArgs, err := cmd.Flags().GetStringArray("arg")
	if err != nil {
		return nil, handleError(ErrInternal, err, "")
	}
	normalized, err := querysvc.NormalizeArgs(rawArgs)
	if err != nil {
		return nil, mapQuerySvcError(err)
	}
	return normalized, nil
}

func mapExecuteQueryError(queryStr string, err error) error {
	var validationErr *query.ValidationError
	if errors.As(err, &validationErr) {
		return handleErrorMsg(ErrQueryInvalid, validationErr.Message, validationErr.Suggestion)
	}

	if _, parseErr := query.Parse(queryStr); parseErr != nil {
		return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("parse error: %v", parseErr), "")
	}

	return handleError(ErrDatabaseError, err, "")
}

func mapQuerySvcError(err error) error {
	svcErr, ok := querysvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case querysvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case querysvc.CodeQueryInvalid:
		return handleErrorMsg(ErrQueryInvalid, svcErr.Message, svcErr.Suggestion)
	case querysvc.CodeQueryNotFound:
		return handleErrorMsg(ErrQueryNotFound, svcErr.Message, svcErr.Suggestion)
	case querysvc.CodeDuplicateName:
		return handleErrorMsg(ErrDuplicateName, svcErr.Message, svcErr.Suggestion)
	case querysvc.CodeConfigInvalid:
		return handleError(ErrConfigInvalid, svcErr, svcErr.Suggestion)
	case querysvc.CodeFileWriteError:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func init() {
	queryCmd.Flags().BoolP("list", "l", false, "List saved queries")
	queryCmd.Flags().Bool("refresh", false, "Refresh stale files before query")
	queryCmd.Flags().Bool("ids", false, "Output only object/trait IDs, one per line (for piping)")
	queryCmd.Flags().Int("limit", 0, "Maximum number of query results to return (0 means no limit)")
	queryCmd.Flags().Int("offset", 0, "Zero-based offset for query results")
	queryCmd.Flags().Bool("count-only", false, "Return only the total count of matches (no items or IDs)")
	queryCmd.Flags().StringArray("apply", nil, "Apply a bulk operation to query results (format: command args...)")
	queryCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")
	queryCmd.Flags().Bool("pipe", false, "Force pipe-friendly output format")
	queryCmd.Flags().Bool("no-pipe", false, "Force human-readable output format")

	// query add flags
	queryAddCmd.Flags().String("description", "", "Human-readable description")
	queryAddCmd.Flags().StringArray("arg", nil, "Declare saved query input name (repeatable, sets args order)")

	queryCmd.AddCommand(queryAddCmd)
	queryCmd.AddCommand(queryRemoveCmd)
	rootCmd.AddCommand(queryCmd)
}
