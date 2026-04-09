package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

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
  rvn query saved list               # Manage saved queries`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --pipe/--no-pipe flags
		if pipeFlag, _ := cmd.Flags().GetBool("pipe"); pipeFlag {
			t := true
			SetPipeFormat(&t)
		} else if noPipeFlag, _ := cmd.Flags().GetBool("no-pipe"); noPipeFlag {
			f := false
			SetPipeFormat(&f)
		}

		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "specify a query string", "Run 'rvn query saved list' to see saved queries")
		}

		// Load vault config for saved queries and unknown-query suggestions.
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		// MCP sends query_string as a single positional arg. Support
		// "saved-query-name <inputs...>" in that single string.
		args = maybeSplitInlineSavedQueryArgs(args, vaultCfg.Queries)

		queryName := args[0]
		queryStr := ""
		isSavedQuery := false

		if savedQuery, ok := vaultCfg.Queries[queryName]; ok {
			isSavedQuery = true
			queryStr, err = querysvc.ResolveSavedQuery(queryName, savedQuery, args[1:], nil)
			if err != nil {
				return mapQuerySvcError(err)
			}
		} else {
			// Join multiple args with spaces - allows running without quoting the whole query
			// e.g., `rvn query trait:todo content:"my task"` works the same as
			//       `rvn query 'trait:todo content:"my task"'`
			queryStr = joinQueryArgs(args)
		}

		refresh, _ := cmd.Flags().GetBool("refresh")
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

		// If --apply is set, route through the canonical query handler.
		if len(applyArgs) > 0 {
			return runCanonicalQuery(queryStr, map[string]interface{}{
				"query_string": joinQueryArgs(args),
				"refresh":      refresh,
				"apply":        applyArgs,
				"confirm":      confirmApply,
			})
		}

		if !strings.HasPrefix(queryStr, "object:") && !strings.HasPrefix(queryStr, "trait:") {
			if isSavedQuery {
				return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' must start with 'object:' or 'trait:'", queryName), "")
			}

			db, err := index.Open(vaultPath)
			if err != nil {
				return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
			}
			defer db.Close()
			db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

			sch, schemaErr := schema.Load(vaultPath)
			if schemaErr != nil {
				sch = nil
			}

			suggestion := buildUnknownQuerySuggestion(db, queryStr, vaultCfg.GetDailyDirectory(), sch)
			return handleErrorMsg(ErrQueryInvalid,
				fmt.Sprintf("unknown query: %s", queryStr),
				suggestion)
		}

		return runCanonicalQuery(queryStr, map[string]interface{}{
			"query_string": joinQueryArgs(args),
			"refresh":      refresh,
			"ids":          idsOnly,
			"limit":        limit,
			"offset":       offset,
			"count-only":   countOnly,
		})
	},
}

func runCanonicalQuery(queryStr string, args map[string]interface{}) error {
	result := executeCanonicalQuery(args)
	if hasQueryApply(args) {
		return renderCanonicalQueryApplyResult(args, result)
	}
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(mapQueryCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	data, _ := result.Data.(map[string]interface{})
	if rawQueries, ok := data["queries"]; ok {
		return listSavedQueries(savedQueriesFromResult(rawQueries))
	}

	if total, ok := data["total"]; ok {
		if _, hasItems := data["items"]; !hasItems {
			if _, hasIDs := data["ids"]; !hasIDs {
				fmt.Println(intFromAny(total))
				return nil
			}
		}
	}

	if rawIDs, ok := data["ids"]; ok {
		for _, id := range stringSliceFromAny(rawIDs) {
			fmt.Println(id)
		}
		return nil
	}

	queryType, _ := data["query_type"].(string)
	switch queryType {
	case "object":
		objects := objectResultsFromAny(data["items"])
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForObjectResults(objects))
			return nil
		}
		sch, _ := schema.Load(getVaultPath())
		printQueryObjectResults(queryStr, queryLabelFromData(data, queryStr), objects, sch)
		return nil
	case "trait":
		traits := traitResultsFromAny(data["items"])
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForTraitResults(traits))
			return nil
		}
		printQueryTraitResults(queryStr, queryLabelFromData(data, queryStr), traits)
		return nil
	default:
		return handleErrorMsg(ErrInternal, "unexpected query result shape", "")
	}
}

func executeCanonicalQuery(args map[string]interface{}) commandexec.Result {
	return app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "query",
		VaultPath: getVaultPath(),
		Caller:    commandexec.CallerCLI,
		Args:      args,
	})
}

func renderCanonicalQueryApplyResult(args map[string]interface{}, result commandexec.Result) error {
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if err := renderCanonicalBulkResult(result); err != nil {
		return err
	}

	if !isJSONOutput() && !boolValue(args["confirm"]) && promptForConfirm("Apply changes?") {
		confirmedArgs := copyArgsMap(args)
		confirmedArgs["confirm"] = true
		applyResult := executeCanonicalQuery(confirmedArgs)
		if !applyResult.OK {
			if applyResult.Error != nil {
				return handleErrorWithDetails(applyResult.Error.Code, applyResult.Error.Message, applyResult.Error.Suggestion, applyResult.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}
		return renderCanonicalBulkResult(applyResult)
	}

	return nil
}

func hasQueryApply(args map[string]interface{}) bool {
	return len(stringSliceFromAny(args["apply"])) > 0
}

func copyArgsMap(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}
	out := make(map[string]interface{}, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}

func listSavedQueries(queries []SavedQueryInfo) error {
	fmt.Println(ui.SectionHeader("Saved queries"))
	if len(queries) == 0 {
		fmt.Println(ui.Bullet(ui.Hint("(none defined)")))
		fmt.Printf("\n%s\n", ui.Hint("Define queries in raven.yaml under 'queries:'"))
		return nil
	}
	for _, q := range queries {
		desc := q.Description
		if desc == "" {
			desc = q.Query
		}
		if len(q.Args) > 0 {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s", ui.Bold.Render(q.Name), desc, ui.Hint("(args: "+strings.Join(q.Args, ", ")+")"))))
			continue
		}
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(q.Name), desc)))
	}
	return nil
}

func savedQueriesFromResult(raw interface{}) []SavedQueryInfo {
	if rows, ok := raw.([]map[string]interface{}); ok {
		queries := make([]SavedQueryInfo, 0, len(rows))
		for _, entry := range rows {
			queries = append(queries, SavedQueryInfo{
				Name:        stringValue(entry["name"]),
				Query:       stringValue(entry["query"]),
				Args:        stringSliceFromAny(entry["args"]),
				Description: stringValue(entry["description"]),
			})
		}
		return queries
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	queries := make([]SavedQueryInfo, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		queries = append(queries, SavedQueryInfo{
			Name:        stringValue(entry["name"]),
			Query:       stringValue(entry["query"]),
			Args:        stringSliceFromAny(entry["args"]),
			Description: stringValue(entry["description"]),
		})
	}
	return queries
}

func objectResultsFromAny(raw interface{}) []model.Object {
	if rows, ok := raw.([]map[string]interface{}); ok {
		results := make([]model.Object, 0, len(rows))
		for _, entry := range rows {
			results = append(results, model.Object{
				ID:        stringValue(entry["id"]),
				Type:      stringValue(entry["type"]),
				Fields:    mapValue(entry["fields"]),
				FilePath:  stringValue(entry["file_path"]),
				LineStart: intFromAny(entry["line"]),
			})
		}
		return results
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	results := make([]model.Object, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, model.Object{
			ID:        stringValue(entry["id"]),
			Type:      stringValue(entry["type"]),
			Fields:    mapValue(entry["fields"]),
			FilePath:  stringValue(entry["file_path"]),
			LineStart: intFromAny(entry["line"]),
		})
	}
	return results
}

func traitResultsFromAny(raw interface{}) []model.Trait {
	if rows, ok := raw.([]map[string]interface{}); ok {
		results := make([]model.Trait, 0, len(rows))
		for _, entry := range rows {
			results = append(results, model.Trait{
				ID:             stringValue(entry["id"]),
				TraitType:      stringValue(entry["trait_type"]),
				Value:          stringPointer(entry["value"]),
				Content:        stringValue(entry["content"]),
				FilePath:       stringValue(entry["file_path"]),
				Line:           intFromAny(entry["line"]),
				ParentObjectID: stringValue(entry["object_id"]),
			})
		}
		return results
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	results := make([]model.Trait, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, model.Trait{
			ID:             stringValue(entry["id"]),
			TraitType:      stringValue(entry["trait_type"]),
			Value:          stringPointer(entry["value"]),
			Content:        stringValue(entry["content"]),
			FilePath:       stringValue(entry["file_path"]),
			Line:           intFromAny(entry["line"]),
			ParentObjectID: stringValue(entry["object_id"]),
		})
	}
	return results
}

func queryLabelFromData(data map[string]interface{}, queryStr string) string {
	if label := stringValue(data["type"]); label != "" {
		return label
	}
	if label := stringValue(data["trait"]); label != "" {
		return label
	}
	parsed, err := query.Parse(queryStr)
	if err != nil {
		return ""
	}
	return parsed.TypeName
}

func stringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func stringPointer(raw interface{}) *string {
	switch value := raw.(type) {
	case nil:
		return nil
	case string:
		return &value
	case *string:
		return value
	default:
		return nil
	}
}

func mapValue(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	default:
		return nil
	}
}

func intFromAny(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

var querySavedCmd = &cobra.Command{
	Use:   "saved",
	Short: "Manage saved queries",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var querySavedListCmd = newCanonicalLeafCommand("query_saved_list", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedList,
})

var querySavedGetCmd = newCanonicalLeafCommand("query_saved_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedGet,
})

var querySavedSetCmd = newCanonicalLeafCommand("query_saved_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildQuerySavedSetArgs,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedSet,
})

var querySavedRemoveCmd = newCanonicalLeafCommand("query_saved_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedRemove,
})

func buildQuerySavedSetArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	declaredArgs, err := normalizeSavedQueryArgsForCommand(cmd)
	if err != nil {
		return nil, err
	}
	description, _ := cmd.Flags().GetString("description")
	return map[string]interface{}{
		"name":         args[0],
		"query_string": args[1],
		"arg":          declaredArgs,
		"description":  description,
	}, nil
}

func handleCanonicalQueryFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapQueryCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func renderQuerySavedList(_ *cobra.Command, result commandexec.Result) error {
	return listSavedQueries(savedQueriesFromResult(canonicalDataMap(result)["queries"]))
}

func renderQuerySavedGet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("Name:"), stringValue(data["name"]))
	fmt.Printf("%s %s\n", ui.Hint("Query:"), stringValue(data["query"]))
	if args := stringSliceFromAny(data["args"]); len(args) > 0 {
		fmt.Printf("%s %s\n", ui.Hint("Args:"), strings.Join(args, ", "))
	} else {
		fmt.Printf("%s %s\n", ui.Hint("Args:"), ui.Hint("(none)"))
	}
	if description := stringValue(data["description"]); description != "" {
		fmt.Printf("%s %s\n", ui.Hint("Description:"), description)
	}
	return nil
}

func renderQuerySavedSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	status := stringValue(data["status"])
	name := stringValue(data["name"])
	switch status {
	case "created":
		fmt.Println(ui.Checkf("Created saved query '%s'", name))
	case "updated":
		fmt.Println(ui.Checkf("Updated saved query '%s'", name))
	default:
		fmt.Println(ui.Starf("Saved query '%s' unchanged", name))
	}
	fmt.Printf("  %s %s\n", ui.Hint("Run with:"), ui.Bold.Render("rvn query "+name))
	return nil
}

func renderQuerySavedRemove(_ *cobra.Command, result commandexec.Result) error {
	fmt.Println(ui.Checkf("Removed query '%s'", stringValue(canonicalDataMap(result)["name"])))
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

	parts, ok := querysvc.SplitInlineInvocation(inline)
	if !ok || len(parts) < 2 {
		return args
	}

	if _, exists := queries[parts[0]]; !exists {
		return args
	}

	return parts
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

func mapQueryCode(code string) string {
	switch code {
	case "MISSING_ARGUMENT":
		return ErrMissingArgument
	case "INVALID_ARGS", "INVALID_INPUT":
		return ErrInvalidInput
	case "QUERY_INVALID":
		return ErrQueryInvalid
	case "QUERY_NOT_FOUND":
		return ErrQueryNotFound
	case "CONFIG_INVALID":
		return ErrConfigInvalid
	case "DATABASE_ERROR":
		return ErrDatabaseError
	default:
		return ErrInternal
	}
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
	case querysvc.CodeConfigInvalid:
		return handleError(ErrConfigInvalid, svcErr, svcErr.Suggestion)
	case querysvc.CodeFileWriteError:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func init() {
	queryCmd.Flags().Bool("refresh", false, "Refresh stale files before query")
	queryCmd.Flags().Bool("ids", false, "Output only object/trait IDs, one per line (for piping)")
	queryCmd.Flags().Int("limit", 0, "Maximum number of query results to return (0 means no limit)")
	queryCmd.Flags().Int("offset", 0, "Zero-based offset for query results")
	queryCmd.Flags().Bool("count-only", false, "Return only the total count of matches (no items or IDs)")
	queryCmd.Flags().StringArray("apply", nil, "Apply a bulk operation to query results (format: command args...)")
	queryCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")
	queryCmd.Flags().Bool("pipe", false, "Force pipe-friendly output for shell pipelines (jq, head, sort)")
	queryCmd.Flags().Bool("no-pipe", false, "Force human-readable output format")

	querySavedCmd.AddCommand(querySavedListCmd)
	querySavedCmd.AddCommand(querySavedGetCmd)
	querySavedCmd.AddCommand(querySavedSetCmd)
	querySavedCmd.AddCommand(querySavedRemoveCmd)
	queryCmd.AddCommand(querySavedCmd)
	rootCmd.AddCommand(queryCmd)
}
