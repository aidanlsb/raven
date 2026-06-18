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
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

// printObjectTable prints object results using the shared retrieval table.
func printObjectTable(results []model.Object, sch *schema.Schema) {
	if len(results) == 0 {
		return
	}

	nameField, fieldColumns := objectTableColumns(results, sch)
	display := ui.NewDisplayContext()
	table := ui.NewResultsTable(display, ui.ObjectLayout(fieldColumns))
	table.SetHeaders(objectTableHeaders(nameField, fieldColumns))

	for i, r := range results {
		cells := make([]string, 0, len(fieldColumns)+3)
		cells = append(cells,
			ui.FormatRowNum(i+1, len(results)),
			objectTableName(r, nameField),
		)

		for _, col := range fieldColumns {
			valStr := formatFieldValueSimple(r.Fields[col])
			if valStr == "" {
				valStr = "-"
			}
			cells = append(cells, valStr)
		}

		location := formatLocationLinkSimpleStyled(r.FilePath, r.LineStart, ui.Muted.Render)
		cells = append(cells, location)

		table.AddRow(ui.ResultRow{
			Num:      i + 1,
			Cells:    cells,
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.LineStart),
		})
	}

	fmt.Println(table.Render())
}

func objectTableColumns(results []model.Object, sch *schema.Schema) (string, []string) {
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
	return nameField, fieldColumns
}

func objectTableHeaders(nameField string, fieldColumns []string) []string {
	nameHeader := "id"
	if nameField != "" {
		nameHeader = nameField
	}

	headers := make([]string, 0, len(fieldColumns)+3)
	headers = append(headers, "#", nameHeader)
	headers = append(headers, fieldColumns...)
	headers = append(headers, "location")
	return headers
}

func objectTableName(obj model.Object, nameField string) string {
	if nameField != "" {
		if value := formatFieldValueSimple(obj.Fields[nameField]); value != "" {
			return value
		}
	}
	return filepath.Base(obj.ID)
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
	Long: `Query items by type, sections, traits by name, or assets using the Raven query language.

Query roots:
  type:<type> [predicates]    Query items of a type
  section [predicates]        Query heading-derived sections
  trait:<name> [predicates]   Query traits by name
  asset [predicates]          Query indexed asset resources

Predicates for type queries:
  .field==value      Field equals value
  exists(.field)     Field exists (has a value)
  !.field==value     Field does not equal value
  oneof(.field, [...]) Field matches any listed scalar value
  includes(.field, "text") Substring match
  has(trait:...)      Has a directly scoped trait
  has(section...)     Has a directly scoped section
  contains(trait:...) Recursively contains a matching trait
  contains(section...) Recursively contains a matching section
  refs([[target]])      References a specific target
  refs(type:...)      References an item matching nested type query
  refd([[source]])      Referenced by a specific source
  refd(type:...)      Referenced by an item matching nested type query
  refd(trait:...)       Referenced by a trait matching nested trait query
  content("term")       Full-text search on item content

Predicates for trait queries:
  .value==val      Trait value equals val
  oneof(.value, [...]) Value matches any listed scalar value
  in(type:...)       Direct scope matches nested type query
  in(section...)     Direct scope matches nested section query
  within(type:...)   Any scope matches nested type query
  within(section...) Any scope matches nested section query
  at(trait:...)        Co-located with trait matching nested trait query
  refs([[target]])     Line contains reference to target
  refs(type:...)     Line references an item matching nested type query
  content("term")      Line content contains term

Predicates for asset queries:
  .extension==pdf       Asset field equals value
  oneof(.extension, [...]) Asset field matches any listed scalar value
  includes(.filename, "text") Substring match on derived metadata
  startswith(.media_type, "image/")  String match on derived metadata
  .size_bytes>1024      Numeric size comparison
  refd(type:...)        Referenced by matching items
  refd(trait:...)       Referenced by matching trait lines

Boolean operators:
  !pred            NOT
  pred1 pred2      AND (space-separated)
  pred1 | pred2    OR

Saved query inputs must be declared with args: in raven.yaml when using {{args.<name>}}.
You can then pass inputs either by position (following args order) or as key=value pairs.

Use --browse to open an interactive Raven picker with filtering, preview, and
editor handoff for the selected result.


Examples:
  rvn query "type:project .status==active"
  rvn query "type:meeting has(trait:due)"
  rvn query "section .title==Tasks"
  rvn query "trait:due .value<today"
  rvn query "asset .extension==pdf"
  rvn query "asset startswith(.media_type, \"image/\")"
  rvn query "trait:todo content(\"my task\")"
  rvn query "trait:highlight in(type:book .status==reading)"
  rvn query tasks                    # Run saved query
  rvn query project-todos raven      # Positional input (args: [project])
  rvn query project-todos project=projects/raven
  rvn query saved list               # Manage saved queries`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

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
		joinedQueryArgs := joinQueryArgs(args)
		queryStr := ""
		isSavedQuery := false
		var savedOptions *config.QueryOptions

		if savedQuery, ok := vaultCfg.Queries[queryName]; ok && !isAssetQueryString(joinedQueryArgs) && !isSectionQueryString(joinedQueryArgs) {
			isSavedQuery = true
			savedOptions = savedQuery.Options
			queryStr, err = querysvc.ResolveSavedQuery(queryName, savedQuery, args[1:], nil)
			if err != nil {
				return mapQuerySvcError(err)
			}
		} else {
			// Join multiple args with spaces - allows running without quoting the whole query
			// e.g., `rvn query trait:todo 'content("my task")'` works the same as
			//       `rvn query 'trait:todo content("my task")'`
			queryStr = joinedQueryArgs
		}

		refresh := queryBoolFlagValue(cmd, "refresh", savedBoolOption(savedOptions, "refresh"))
		idsOnly := queryBoolFlagValue(cmd, "ids", savedBoolOption(savedOptions, "ids"))
		limit := queryIntFlagValue(cmd, "limit", savedIntOption(savedOptions, "limit"))
		offset := queryIntFlagValue(cmd, "offset", savedIntOption(savedOptions, "offset"))
		countOnly := queryBoolFlagValue(cmd, "count-only", savedBoolOption(savedOptions, "count-only"))
		if limit < 0 {
			return handleErrorMsg(ErrInvalidInput, "--limit must be >= 0", "Use --limit 0 for no limit")
		}
		if offset < 0 {
			return handleErrorMsg(ErrInvalidInput, "--offset must be >= 0", "Use --offset 0 for no offset")
		}

		applyArgs := queryStringArrayFlagValue(cmd, "apply", savedApplyOption(savedOptions))
		confirmApply := queryBoolFlagValue(cmd, "confirm", savedBoolOption(savedOptions, "confirm"))
		browse := queryBoolFlagValue(cmd, "browse", savedBoolOption(savedOptions, "browse"))
		if isJSONOutput() && browse && !cmd.Flags().Changed("browse") {
			// JSON is an explicit machine-readable mode; let it suppress saved
			// interactive defaults so saved queries remain agent/script-friendly.
			browse = false
		}

		pipeOverride := queryPipeOverride(cmd, savedOptions)
		SetPipeFormat(pipeOverride)
		if browse {
			if isJSONOutput() {
				return handleErrorMsg(ErrInvalidInput, "--browse cannot be used with --json", "Remove --browse or --json")
			}
			if idsOnly || countOnly || len(applyArgs) > 0 {
				return handleErrorMsg(ErrInvalidInput, "--browse cannot be used with --ids, --count-only, or --apply", "Run the query without browse for machine-readable or bulk modes")
			}
			if pipeOverride != nil && *pipeOverride {
				return handleErrorMsg(ErrInvalidInput, "--browse cannot be used with --pipe", "Use --no-pipe or remove --browse")
			}
			if !canUseInteractiveTerminal() {
				return handleErrorMsg(ErrInvalidInput, "interactive browse requires an interactive terminal", "Run without --browse in non-interactive contexts")
			}
		}

		// If --apply is set, route through the canonical query handler.
		if len(applyArgs) > 0 {
			return runCanonicalQuery(queryStr, map[string]interface{}{
				"query_string": joinQueryArgs(args),
				"refresh":      refresh,
				"apply":        applyArgs,
				"confirm":      confirmApply,
			})
		}

		if !strings.HasPrefix(queryStr, "type:") && !strings.HasPrefix(queryStr, "trait:") && !isAssetQueryString(queryStr) && !isSectionQueryString(queryStr) {
			if isSavedQuery {
				return handleErrorMsg(ErrQueryInvalid, fmt.Sprintf("saved query '%s' must start with 'type:', 'trait:', 'section', or 'asset'", queryName), "")
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
			"browse":       browse,
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

	queryKind, _ := data["query_kind"].(string)
	browse := boolValue(args["browse"])
	switch queryKind {
	case "type", "object":
		objects := objectResultsFromAny(data["items"])
		if browse {
			if len(objects) == 0 {
				sch, _ := schema.Load(getVaultPath())
				printQueryObjectResults(queryStr, queryLabelFromData(data, queryStr), objects, sch)
				return nil
			}
			sch, _ := schema.Load(getVaultPath())
			return browseQueryResults(browseItemsForObjectResults(objects, sch), objectBrowseHeaders(objects, sch), objectBrowseLayout(objects, sch))
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForObjectResults(objects))
			return nil
		}
		sch, _ := schema.Load(getVaultPath())
		printQueryObjectResults(queryStr, queryLabelFromData(data, queryStr), objects, sch)
		return nil
	case "trait":
		traits := traitResultsFromAny(data["items"])
		if browse {
			if len(traits) == 0 {
				printQueryTraitResults(queryStr, queryLabelFromData(data, queryStr), traits)
				return nil
			}
			return browseQueryResults(browseItemsForTraitResults(traits), traitBrowseHeaders(), ui.TraitLayout())
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForTraitResults(traits))
			return nil
		}
		printQueryTraitResults(queryStr, queryLabelFromData(data, queryStr), traits)
		return nil
	case "asset":
		assets := assetResultsFromAny(data["items"])
		if browse {
			if len(assets) == 0 {
				printQueryAssetResults(queryStr, assets)
				return nil
			}
			return browseQueryResults(browseItemsForAssetResults(assets), assetBrowseHeaders(), ui.AssetLayout())
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForAssetResults(assets))
			return nil
		}
		printQueryAssetResults(queryStr, assets)
		return nil
	case "section":
		sections := sectionResultsFromAny(data["items"])
		if browse {
			if len(sections) == 0 {
				printQuerySectionResults(queryStr, sections)
				return nil
			}
			return browseQueryResults(browseItemsForSectionResults(sections), sectionBrowseHeaders(), ui.SearchLayout())
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForSectionResults(sections))
			return nil
		}
		printQuerySectionResults(queryStr, sections)
		return nil
	default:
		return handleErrorMsg(ErrInternal, "unexpected query result shape", "")
	}
}

func browseItemsForObjectResults(results []model.Object, sch *schema.Schema) []picker.Item {
	nameField, fieldColumns := objectTableColumns(results, sch)
	items := make([]picker.Item, 0, len(results))
	for _, result := range results {
		location := fmt.Sprintf("%s:%d", result.FilePath, result.LineStart)
		label := objectTableName(result, nameField)
		detail := objectBrowseDetail(result, fieldColumns)
		columns := objectBrowseColumns(result, nameField, fieldColumns, location)
		items = append(items, picker.Item{
			ID:       result.ID,
			Label:    label,
			Detail:   detail,
			Location: location,
			Columns:  columns,
			SearchText: browseSearchText(
				result.ID,
				result.Type,
				label,
				detail,
				location,
				result.FilePath,
				strings.Join(columns, " "),
			),
			FilePath: result.FilePath,
			Line:     result.LineStart,
		})
	}
	return items
}

func objectBrowseHeaders(results []model.Object, sch *schema.Schema) []string {
	nameField, fieldColumns := objectTableColumns(results, sch)
	return objectTableHeaders(nameField, fieldColumns)
}

func objectBrowseLayout(results []model.Object, sch *schema.Schema) []ui.ColumnDef {
	_, fieldColumns := objectTableColumns(results, sch)
	return ui.ObjectLayout(fieldColumns)
}

func objectBrowseColumns(obj model.Object, nameField string, fieldColumns []string, location string) []string {
	columns := make([]string, 0, len(fieldColumns)+2)
	columns = append(columns, objectTableName(obj, nameField))
	for _, fieldName := range fieldColumns {
		value := formatFieldValueSimple(obj.Fields[fieldName])
		if value == "" {
			value = "-"
		}
		columns = append(columns, value)
	}
	columns = append(columns, location)
	return columns
}

func browseItemsForTraitResults(results []model.Trait) []picker.Item {
	items := make([]picker.Item, 0, len(results))
	for _, result := range results {
		value := ""
		if result.Value != nil && *result.Value != result.TraitType {
			value = *result.Value
		}
		detail := "@" + result.TraitType
		if value != "" {
			detail += "(" + value + ")"
		}
		location := fmt.Sprintf("%s:%d", result.FilePath, result.Line)
		label := TruncateContent(result.Content, 160)
		items = append(items, picker.Item{
			ID:       result.ID,
			Label:    label,
			Detail:   detail,
			Location: location,
			Columns:  []string{label, detail, location},
			SearchText: browseSearchText(
				result.ID,
				result.TraitType,
				value,
				result.Content,
				result.ParentObjectID,
				location,
				result.FilePath,
			),
			FilePath: result.FilePath,
			Line:     result.Line,
		})
	}
	return items
}

func traitBrowseHeaders() []string {
	return []string{"#", "content", "trait", "location"}
}

func browseItemsForAssetResults(results []model.Asset) []picker.Item {
	items := make([]picker.Item, 0, len(results))
	for _, result := range results {
		detail := result.MediaType
		if detail == "" {
			detail = "-"
		}
		items = append(items, picker.Item{
			ID:       result.ID,
			Label:    result.FilePath,
			Detail:   detail,
			Location: formatAssetSize(result.SizeBytes),
			Columns:  []string{result.FilePath, detail, formatAssetSize(result.SizeBytes)},
			SearchText: browseSearchText(
				result.ID,
				result.FilePath,
				result.Filename,
				result.Extension,
				result.MediaType,
				formatAssetSize(result.SizeBytes),
			),
			FilePath: result.FilePath,
		})
	}
	return items
}

func assetBrowseHeaders() []string {
	return []string{"#", "path", "media type", "size"}
}

func browseItemsForSectionResults(results []model.Section) []picker.Item {
	items := make([]picker.Item, 0, len(results))
	for _, result := range results {
		location := fmt.Sprintf("%s:%d", result.FilePath, result.LineStart)
		detail := fmt.Sprintf("h%d #%s", result.Level, result.Slug)
		parentSectionID := ""
		if result.ParentSectionID != nil {
			parentSectionID = *result.ParentSectionID
		}
		items = append(items, picker.Item{
			ID:       result.ID,
			Label:    result.Title,
			Detail:   detail,
			Location: location,
			Columns:  []string{result.Title, detail, location},
			SearchText: browseSearchText(
				result.ID,
				result.Title,
				result.Slug,
				detail,
				result.FileObjectID,
				parentSectionID,
				location,
				result.FilePath,
			),
			FilePath: result.FilePath,
			Line:     result.LineStart,
		})
	}
	return items
}

func sectionBrowseHeaders() []string {
	return []string{"#", "title", "heading", "location"}
}

func objectBrowseDetail(obj model.Object, fieldColumns []string) string {
	parts := make([]string, 0, len(fieldColumns))
	for _, fieldName := range fieldColumns {
		value := formatFieldValueSimple(obj.Fields[fieldName])
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", fieldName, value))
	}
	return strings.Join(parts, " ")
}

func browseSearchText(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func browseQueryResults(items []picker.Item, headers []string, columns []ui.ColumnDef) error {
	selected, ok, err := picker.Run(items, picker.Options{
		Title:   "Query results",
		Prompt:  "filter",
		Headers: headers,
		Columns: columns,
		Preview: vaultFilePreview(getVaultPath()),
	})
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	if !ok {
		return nil
	}
	if strings.TrimSpace(selected.Item.FilePath) == "" {
		return handleErrorMsg(ErrInternal, "selected query result has no file path", "")
	}
	openFileInEditorAtLine(filepath.Join(getVaultPath(), selected.Item.FilePath), selected.Item.FilePath, selected.Item.Line, false)
	return nil
}

func queryBoolFlagValue(cmd *cobra.Command, name string, saved *bool) bool {
	if cmd.Flags().Changed(name) {
		value, _ := cmd.Flags().GetBool(name)
		return value
	}
	if saved != nil {
		return *saved
	}
	return false
}

func queryIntFlagValue(cmd *cobra.Command, name string, saved *int) int {
	if cmd.Flags().Changed(name) {
		value, _ := cmd.Flags().GetInt(name)
		return value
	}
	if saved != nil {
		return *saved
	}
	return 0
}

func queryStringArrayFlagValue(cmd *cobra.Command, name string, saved []string) []string {
	if cmd.Flags().Changed(name) {
		value, _ := cmd.Flags().GetStringArray(name)
		return value
	}
	return append([]string(nil), saved...)
}

func queryPipeOverride(cmd *cobra.Command, saved *config.QueryOptions) *bool {
	if cmd.Flags().Changed("pipe") {
		value, _ := cmd.Flags().GetBool("pipe")
		return &value
	}
	if noPipeFlag, _ := cmd.Flags().GetBool("no-pipe"); cmd.Flags().Changed("no-pipe") && noPipeFlag {
		value := false
		return &value
	}
	if saved != nil && saved.Pipe != nil {
		value := *saved.Pipe
		return &value
	}
	return nil
}

func savedBoolOption(options *config.QueryOptions, name string) *bool {
	if options == nil {
		return nil
	}
	switch name {
	case "refresh":
		return options.Refresh
	case "ids":
		return options.IDs
	case "count-only":
		return options.CountOnly
	case "confirm":
		return options.Confirm
	case "browse":
		return options.Browse
	default:
		return nil
	}
}

func savedIntOption(options *config.QueryOptions, name string) *int {
	if options == nil {
		return nil
	}
	switch name {
	case "limit":
		return options.Limit
	case "offset":
		return options.Offset
	default:
		return nil
	}
}

func savedApplyOption(options *config.QueryOptions) []string {
	if options == nil {
		return nil
	}
	return append([]string(nil), options.Apply...)
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

func assetResultsFromAny(raw interface{}) []model.Asset {
	if rows, ok := raw.([]map[string]interface{}); ok {
		results := make([]model.Asset, 0, len(rows))
		for _, entry := range rows {
			results = append(results, model.Asset{
				ID:        stringValue(entry["id"]),
				FilePath:  stringValue(entry["file_path"]),
				Filename:  stringValue(entry["filename"]),
				Extension: stringValue(entry["extension"]),
				MediaType: stringValue(entry["media_type"]),
				SizeBytes: int64FromAny(entry["size_bytes"]),
			})
		}
		return results
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	results := make([]model.Asset, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, model.Asset{
			ID:        stringValue(entry["id"]),
			FilePath:  stringValue(entry["file_path"]),
			Filename:  stringValue(entry["filename"]),
			Extension: stringValue(entry["extension"]),
			MediaType: stringValue(entry["media_type"]),
			SizeBytes: int64FromAny(entry["size_bytes"]),
		})
	}
	return results
}

func sectionResultsFromAny(raw interface{}) []model.Section {
	if rows, ok := raw.([]map[string]interface{}); ok {
		results := make([]model.Section, 0, len(rows))
		for _, entry := range rows {
			results = append(results, sectionFromResultMap(entry))
		}
		return results
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	results := make([]model.Section, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, sectionFromResultMap(entry))
	}
	return results
}

func sectionFromResultMap(entry map[string]interface{}) model.Section {
	return model.Section{
		ID:              stringValue(entry["id"]),
		FileObjectID:    stringValue(entry["file_object_id"]),
		FilePath:        stringValue(entry["file_path"]),
		Slug:            stringValue(entry["slug"]),
		Title:           stringValue(entry["title"]),
		Level:           intFromAny(entry["level"]),
		LineStart:       intFromAny(entry["line_start"]),
		LineEnd:         intPointerFromAny(entry["line_end"]),
		SubtreeLineEnd:  intPointerFromAny(entry["subtree_line_end"]),
		ParentSectionID: stringPointer(entry["parent_section_id"]),
	}
}

func queryLabelFromData(data map[string]interface{}, queryStr string) string {
	if stringValue(data["query_kind"]) == "asset" {
		return "asset"
	}
	if stringValue(data["query_kind"]) == "section" {
		return "section"
	}
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

func intPointerFromAny(raw interface{}) *int {
	switch value := raw.(type) {
	case nil:
		return nil
	case int:
		return &value
	case int64:
		v := int(value)
		return &v
	case float64:
		v := int(value)
		return &v
	default:
		return nil
	}
}

func int64FromAny(raw interface{}) int64 {
	switch value := raw.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
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
	argsMap := map[string]interface{}{
		"name":         args[0],
		"query_string": args[1],
		"arg":          declaredArgs,
		"description":  description,
	}
	addSavedQueryOptionArgs(cmd, argsMap)
	return argsMap, nil
}

func addSavedQueryOptionArgs(cmd *cobra.Command, argsMap map[string]interface{}) {
	if cmd.Flags().Changed("refresh") {
		value, _ := cmd.Flags().GetBool("refresh")
		argsMap["refresh"] = value
	}
	if cmd.Flags().Changed("ids") {
		value, _ := cmd.Flags().GetBool("ids")
		argsMap["ids"] = value
	}
	if cmd.Flags().Changed("limit") {
		value, _ := cmd.Flags().GetInt("limit")
		argsMap["limit"] = value
	}
	if cmd.Flags().Changed("offset") {
		value, _ := cmd.Flags().GetInt("offset")
		argsMap["offset"] = value
	}
	if cmd.Flags().Changed("count-only") {
		value, _ := cmd.Flags().GetBool("count-only")
		argsMap["count-only"] = value
	}
	if cmd.Flags().Changed("apply") {
		value, _ := cmd.Flags().GetStringArray("apply")
		argsMap["apply"] = value
	}
	if cmd.Flags().Changed("confirm") {
		value, _ := cmd.Flags().GetBool("confirm")
		argsMap["confirm"] = value
	}
	if cmd.Flags().Changed("pipe") {
		value, _ := cmd.Flags().GetBool("pipe")
		argsMap["pipe"] = value
	} else if cmd.Flags().Changed("no-pipe") {
		noPipe, _ := cmd.Flags().GetBool("no-pipe")
		if noPipe {
			argsMap["pipe"] = false
		}
	}
	if cmd.Flags().Changed("browse") {
		value, _ := cmd.Flags().GetBool("browse")
		argsMap["browse"] = value
	}
}

func savedQueryOptionsFromFlags(cmd *cobra.Command) *config.QueryOptions {
	options := &config.QueryOptions{}
	if cmd.Flags().Changed("refresh") {
		value, _ := cmd.Flags().GetBool("refresh")
		options.Refresh = &value
	}
	if cmd.Flags().Changed("ids") {
		value, _ := cmd.Flags().GetBool("ids")
		options.IDs = &value
	}
	if cmd.Flags().Changed("limit") {
		value, _ := cmd.Flags().GetInt("limit")
		options.Limit = &value
	}
	if cmd.Flags().Changed("offset") {
		value, _ := cmd.Flags().GetInt("offset")
		options.Offset = &value
	}
	if cmd.Flags().Changed("count-only") {
		value, _ := cmd.Flags().GetBool("count-only")
		options.CountOnly = &value
	}
	if cmd.Flags().Changed("apply") {
		value, _ := cmd.Flags().GetStringArray("apply")
		options.Apply = append([]string(nil), value...)
	}
	if cmd.Flags().Changed("confirm") {
		value, _ := cmd.Flags().GetBool("confirm")
		options.Confirm = &value
	}
	if cmd.Flags().Changed("pipe") {
		value, _ := cmd.Flags().GetBool("pipe")
		options.Pipe = &value
	} else if cmd.Flags().Changed("no-pipe") {
		noPipe, _ := cmd.Flags().GetBool("no-pipe")
		if noPipe {
			value := false
			options.Pipe = &value
		}
	}
	if cmd.Flags().Changed("browse") {
		value, _ := cmd.Flags().GetBool("browse")
		options.Browse = &value
	}
	if options.IsEmpty() {
		return nil
	}
	return options
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

func isAssetQueryString(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return trimmed == "asset" || strings.HasPrefix(trimmed, "asset ")
}

func isSectionQueryString(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return trimmed == "section" || strings.HasPrefix(trimmed, "section ")
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
	if strings.HasPrefix(inline, "type:") || strings.HasPrefix(inline, "trait:") || isAssetQueryString(inline) || isSectionQueryString(inline) {
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

func mapQueryCode(code codes.ErrorCode) codes.ErrorCode {
	switch code {
	case codes.ErrMissingArgument:
		return ErrMissingArgument
	case codes.ErrInvalidArgs, codes.ErrInvalidInput:
		return ErrInvalidInput
	case codes.ErrQueryInvalid:
		return ErrQueryInvalid
	case codes.ErrQueryNotFound:
		return ErrQueryNotFound
	case codes.ErrDatabaseVersion:
		return ErrDatabaseVersion
	case codes.ErrConfigInvalid:
		return ErrConfigInvalid
	case codes.ErrDatabase:
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
	queryCmd.Flags().Bool("browse", false, "Interactively browse query results in Raven's picker and open the selected result")

	querySavedCmd.AddCommand(querySavedListCmd)
	querySavedCmd.AddCommand(querySavedGetCmd)
	querySavedCmd.AddCommand(querySavedSetCmd)
	querySavedCmd.AddCommand(querySavedRemoveCmd)
	queryCmd.AddCommand(querySavedCmd)
	rootCmd.AddCommand(queryCmd)
}
