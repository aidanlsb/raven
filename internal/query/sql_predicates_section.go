package query

import (
	"fmt"
	"strconv"
	"strings"
)

func (e *Executor) buildSectionFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	column, ok := sectionFieldColumn(alias, p.Field)
	if !ok {
		return "", nil, fmt.Errorf("unsupported section field predicate: .%s", p.Field)
	}
	if p.IsRefValue {
		return "", nil, fmt.Errorf("section field '.%s' does not support reference values", p.Field)
	}
	if p.IsExists {
		cond := fmt.Sprintf("%s IS NOT NULL", column)
		if p.CompareOp == CompareNeq {
			cond = fmt.Sprintf("%s IS NULL", column)
		}
		if p.Negated() {
			cond = "NOT (" + cond + ")"
		}
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
	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
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
	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

func sectionFieldColumn(alias, field string) (string, bool) {
	switch field {
	case "id":
		return alias + ".id", true
	case "file_object_id":
		return alias + ".file_object_id", true
	case "file_path":
		return alias + ".file_path", true
	case "slug":
		return alias + ".slug", true
	case "title":
		return alias + ".title", true
	case "level":
		return alias + ".level", true
	case "line_start":
		return alias + ".line_start", true
	case "line_end":
		return alias + ".line_end", true
	case "direct_line_end":
		return alias + ".line_end", true
	case "subtree_line_end":
		return alias + ".subtree_line_end", true
	case "parent_section_id":
		return alias + ".parent_section_id", true
	default:
		return "", false
	}
}

func isNumericSectionField(field string) bool {
	return field == "level" || field == "line_start" || field == "line_end" || field == "direct_line_end" || field == "subtree_line_end"
}
