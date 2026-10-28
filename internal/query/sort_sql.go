package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

func (e *Executor) buildOrderByClause(q *Query, spec entitySQLSpec) (string, []interface{}, error) {
	if len(q.OrderBy) == 0 {
		return spec.orderBy, nil, nil
	}

	parts := make([]string, 0, len(q.OrderBy)+1)
	var args []interface{}
	for _, sf := range q.OrderBy {
		expr, exprArgs, err := e.buildSortExpr(q, spec, sf)
		if err != nil {
			return "", nil, err
		}
		dir := "ASC"
		if sf.Direction == SortDesc {
			dir = "DESC"
		}
		parts = append(parts, expr+" "+dir+" NULLS LAST")
		args = append(args, exprArgs...)
	}
	parts = append(parts, spec.orderBy)
	return strings.Join(parts, ", "), args, nil
}

func (e *Executor) buildSortExpr(q *Query, spec entitySQLSpec, sf SortField) (string, []interface{}, error) {
	switch q.Type {
	case QueryTypeObject:
		return e.buildObjectSortExpr(q.TypeName, sf)
	case QueryTypeTrait:
		return e.buildTraitSortExpr(q.TypeName, sf)
	case QueryTypeSection:
		return buildSectionSortExpr(sf)
	case QueryTypeLink:
		return buildLinkSortExpr(sf)
	default:
		return "", nil, fmt.Errorf("unsupported query type for sort(): %d", spec.queryType)
	}
}

func (e *Executor) buildObjectSortExpr(typeName string, sf SortField) (string, []interface{}, error) {
	if isDateVirtualField(typeName, sf.Field) {
		return dateVirtualFieldExpr("o"), nil, nil
	}
	jsonPath := jsonFieldPath(sf.Field)
	switch e.objectSortFieldType(typeName, sf.Field) {
	case schema.FieldTypeNumber:
		return "CAST(json_extract(o.fields, ?) AS REAL)", []interface{}{jsonPath}, nil
	default:
		return "json_extract(o.fields, ?) COLLATE NOCASE", []interface{}{jsonPath}, nil
	}
}

func (e *Executor) objectSortFieldType(typeName, fieldName string) schema.FieldType {
	if e.schema == nil || typeName == "" {
		return ""
	}
	typeDef := e.schema.Types[typeName]
	if typeDef == nil || typeDef.Fields == nil {
		return ""
	}
	fieldDef := typeDef.Fields[fieldName]
	if fieldDef == nil {
		return ""
	}
	return fieldDef.Type
}

func (e *Executor) buildTraitSortExpr(traitName string, _ SortField) (string, []interface{}, error) {
	valueExpr := "CASE WHEN t.value IS NULL OR t.value = '' THEN NULL ELSE t.value END"
	if e.schema != nil {
		if traitDef := e.schema.Traits[traitName]; traitDef != nil && traitDef.Type == schema.FieldTypeNumber {
			return "CASE WHEN t.value IS NULL OR t.value = '' THEN NULL ELSE CAST(t.value AS REAL) END", nil, nil
		}
	}
	return valueExpr + " COLLATE NOCASE", nil, nil
}

func buildSectionSortExpr(sf SortField) (string, []interface{}, error) {
	column, ok := sectionFieldColumn("s", sf.Field)
	if !ok {
		return "", nil, fmt.Errorf("unsupported section sort field: .%s", sf.Field)
	}
	if isNumericSectionField(sf.Field) {
		return column, nil, nil
	}
	return column + " COLLATE NOCASE", nil, nil
}

func buildLinkSortExpr(sf SortField) (string, []interface{}, error) {
	column, ok := linkColumnExpr("l", sf.Field)
	if !ok {
		return "", nil, fmt.Errorf("unsupported link sort field: .%s", sf.Field)
	}
	fieldType, _ := linkFieldTypes[sf.Field]
	switch fieldType {
	case schema.FieldTypeNumber, schema.FieldTypeBool:
		return column, nil, nil
	}
	if sf.Field == "raw_target" || sf.Field == "normalized_key" {
		return column, nil, nil
	}
	return column + " COLLATE NOCASE", nil, nil
}
