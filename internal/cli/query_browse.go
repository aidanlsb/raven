package cli

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

// effectiveQueryBrowse decides whether browse mode applies. An explicit
// --browse flag always wins (and is validated later, erroring in contexts
// that cannot run a picker). When browse comes from a saved-query default,
// machine-readable contexts suppress it so saved queries stay agent- and
// script-friendly: JSON is an explicit machine mode, and a non-interactive
// terminal (piped output, cron) degrades to the normal non-browse output
// instead of erroring.
func effectiveQueryBrowse(browse, explicit bool) bool {
	if !browse || explicit {
		return browse
	}
	return !isJSONOutput() && canUseInteractiveTerminal()
}

func browseQueryResults(items []picker.Item, headers []string, columns []ui.ColumnDef) error {
	return cliSelector.browseAndOpen(browsePickerOptions{
		Title:                  "Query results",
		Items:                  items,
		Headers:                headers,
		Columns:                columns,
		MissingFilePathMessage: "selected query result has no file path",
	})
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
		if idx := result.IndexValueString(); idx != nil && *idx != result.TraitType {
			value = *idx
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
				result.ParentScopeID,
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

func browseItemsForLinkResults(results []model.Link) []picker.Item {
	items := make([]picker.Item, 0, len(results))
	for _, result := range results {
		label := result.Display
		if label == "" {
			label = result.RawTarget
		}
		detail := result.Scheme
		if result.Ext != "" {
			detail += " ." + result.Ext
		}
		location := fmt.Sprintf("%s:%d", result.FilePath, result.Line)
		items = append(items, picker.Item{
			ID:       result.SourceID,
			Label:    label,
			Detail:   detail,
			Location: location,
			Columns:  []string{label, detail, location},
			SearchText: browseSearchText(
				result.SourceID,
				result.SourceType,
				result.RawTarget,
				result.Display,
				result.Scheme,
				result.Ext,
				result.NormalizedKey,
				location,
			),
			FilePath: result.FilePath,
			Line:     result.Line,
		})
	}
	return items
}

func linkBrowseHeaders() []string {
	return []string{"#", "target", "kind", "location"}
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
