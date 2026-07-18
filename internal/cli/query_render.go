package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

// renderCanonicalQueryHuman renders a successful canonical query response as
// human-readable output (tables, IDs, counts, or an interactive picker). The
// canonical handler already loaded and validated the schema; the schema.Load
// calls here are best-effort display enrichment.
func renderCanonicalQueryHuman(queryStr string, data map[string]interface{}, browse bool) error {
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
	if fv, ok := val.(schema.FieldValue); ok {
		return formatFieldValueSimple(fv.Raw())
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
				Fields:    fieldsFromAny(entry["fields"]),
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
			Fields:    fieldsFromAny(entry["fields"]),
			FilePath:  stringValue(entry["file_path"]),
			LineStart: intFromAny(entry["line"]),
		})
	}
	return results
}

func fieldsFromAny(raw interface{}) map[string]schema.FieldValue {
	m := mapValue(raw)
	if len(m) == 0 {
		return nil
	}
	fields := make(map[string]schema.FieldValue, len(m))
	for key, value := range m {
		fields[key] = schema.FieldValueFromRaw(value)
	}
	return fields
}

func traitResultsFromAny(raw interface{}) []model.Trait {
	if rows, ok := raw.([]map[string]interface{}); ok {
		results := make([]model.Trait, 0, len(rows))
		for _, entry := range rows {
			trait := model.Trait{
				ID:             stringValue(entry["id"]),
				TraitType:      stringValue(entry["trait_type"]),
				Content:        stringValue(entry["content"]),
				FilePath:       stringValue(entry["file_path"]),
				Line:           intFromAny(entry["line"]),
				ParentObjectID: stringValue(entry["object_id"]),
			}
			trait.SetIndexValueString(stringPointer(entry["value"]))
			results = append(results, trait)
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
		trait := model.Trait{
			ID:             stringValue(entry["id"]),
			TraitType:      stringValue(entry["trait_type"]),
			Content:        stringValue(entry["content"]),
			FilePath:       stringValue(entry["file_path"]),
			Line:           intFromAny(entry["line"]),
			ParentObjectID: stringValue(entry["object_id"]),
		}
		trait.SetIndexValueString(stringPointer(entry["value"]))
		results = append(results, trait)
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
