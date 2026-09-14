package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
)

func PrintObjectTable(results []model.Object, sch *schema.Schema, links RetrievalLinks) {
	if len(results) == 0 {
		return
	}

	nameField, fieldColumns := ObjectTableColumns(results, sch)
	display := NewDisplayContext()
	table := NewResultsTable(display, ObjectLayout(fieldColumns))
	table.SetHeaders(ObjectTableHeaders(nameField, fieldColumns))

	for i, r := range results {
		cells := make([]string, 0, len(fieldColumns)+3)
		cells = append(cells,
			FormatRowNum(i+1, len(results)),
			ObjectTableName(r, nameField),
		)

		for _, col := range fieldColumns {
			valStr := FormatFieldValue(r.Fields[col])
			if valStr == "" {
				valStr = "-"
			}
			cells = append(cells, valStr)
		}

		location := links.location(r.FilePath, r.LineStart)
		cells = append(cells, location)

		table.AddRow(ResultRow{
			Num:      i + 1,
			Cells:    cells,
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.LineStart),
		})
	}

	fmt.Println(table.Render())
}

func ObjectTableColumns(results []model.Object, sch *schema.Schema) (string, []string) {
	var typeDef *schema.TypeDefinition
	var fieldColumns []string
	nameField := ""

	if len(results) > 0 && sch != nil {
		typeDef = sch.Types[results[0].Type]
	}

	if typeDef != nil {
		nameField = typeDef.NameField
		for fieldName := range typeDef.Fields {
			if fieldName != nameField {
				fieldColumns = append(fieldColumns, fieldName)
			}
		}
		sort.Strings(fieldColumns)
	}
	return nameField, fieldColumns
}

func ObjectTableHeaders(nameField string, fieldColumns []string) []string {
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

func ObjectTableName(obj model.Object, nameField string) string {
	if nameField != "" {
		if value := FormatFieldValue(obj.Fields[nameField]); value != "" {
			return value
		}
	}
	return filepath.Base(obj.ID)
}

func FormatFieldValue(val interface{}) string {
	if val == nil {
		return ""
	}
	if fv, ok := val.(fieldvalue.FieldValue); ok {
		return FormatFieldValue(fv.Raw())
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

func shortenRefIfNeeded(s string) string {
	if !strings.Contains(s, "/") {
		return s
	}
	name := filepath.Base(s)
	return strings.TrimSuffix(name, ".md")
}
