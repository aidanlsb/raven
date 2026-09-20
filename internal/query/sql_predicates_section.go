package query

import (
	"fmt"
	"strconv"
	"strings"
)

type sectionFieldInfo struct {
	name    string
	column  string
	numeric bool
}

// sectionFields is the single vocabulary for section built-in columns.
// Suggestion text is derived from this list so validator messages cannot drift
// from the SQL column map.
var sectionFields = []sectionFieldInfo{
	{name: "id", column: "id"},
	{name: "file_object_id", column: "file_object_id"},
	{name: "file_path", column: "file_path"},
	{name: "slug", column: "slug"},
	{name: "title", column: "title"},
	{name: "level", column: "level", numeric: true},
	{name: "line_start", column: "line_start", numeric: true},
	{name: "line_end", column: "line_end", numeric: true},
	{name: "direct_line_end", column: "line_end", numeric: true},
	{name: "subtree_line_end", column: "subtree_line_end", numeric: true},
	{name: "parent_section_id", column: "parent_section_id"},
}

func lookupSectionField(field string) (sectionFieldInfo, bool) {
	for _, info := range sectionFields {
		if info.name == field {
			return info, true
		}
	}
	return sectionFieldInfo{}, false
}

func availableSectionFields() []string {
	names := make([]string, len(sectionFields))
	for i, info := range sectionFields {
		names[i] = info.name
	}
	return names
}

func sectionFieldColumn(alias, field string) (string, bool) {
	info, ok := lookupSectionField(field)
	if !ok {
		return "", false
	}
	return alias + "." + info.column, true
}

func isNumericSectionField(field string) bool {
	info, ok := lookupSectionField(field)
	return ok && info.numeric
}

func (e *Executor) buildSectionFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	column, ok := sectionFieldColumn(alias, p.Field)
	if !ok {
		return "", nil, fmt.Errorf("unsupported section field predicate: .%s", p.Field)
	}
	if p.IsRefValue {
		return "", nil, fmt.Errorf("section field '.%s' does not support reference values", p.Field)
	}
	if p.IsExists {
		cond := wrapNot(column+" IS NOT NULL", p.Negated())
		return cond, nil, nil
	}

	var cond string
	var args []interface{}
	op := compareOpToSQL(p.CompareOp)
	if isNumericSectionField(p.Field) {
		n, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64)
		if err != nil {
			return "", nil, fmt.Errorf("section field '.%s' requires a numeric value", p.Field)
		}
		cond = fmt.Sprintf("%s %s ?", column, op)
		args = []interface{}{n}
	} else if p.CompareOp == CompareEq || p.CompareOp == CompareNeq {
		cond = fmt.Sprintf("LOWER(%s) %s LOWER(?)", column, op)
		args = []interface{}{p.Value}
	} else {
		cond = fmt.Sprintf("%s %s ?", column, op)
		args = []interface{}{p.Value}
	}
	return wrapNot(cond, p.Negated()), args, nil
}

func (e *Executor) buildSectionStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	if p.IsElementRef {
		return "", nil, fmt.Errorf("section string functions require a section field")
	}
	column, ok := sectionFieldColumn(alias, p.Field)
	if !ok {
		return "", nil, fmt.Errorf("unsupported section string function field: .%s", p.Field)
	}
	if isNumericSectionField(p.Field) {
		return "", nil, fmt.Errorf("section field '.%s' is numeric and does not support string functions", p.Field)
	}
	cond, args, err := buildStringFuncCondition(p.FuncType, column, p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}
	return wrapNot(cond, p.Negated()), args, nil
}
