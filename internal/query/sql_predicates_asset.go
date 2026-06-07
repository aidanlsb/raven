package query

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

func (e *Executor) buildAssetFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	column, ok := assetColumnExpr(alias, p.Field)
	if !ok {
		return "", nil, fmt.Errorf("asset has no field '%s'", p.Field)
	}

	if p.IsExists {
		cond := column + " IS NOT NULL"
		if p.CompareOp == CompareNeq {
			cond = column + " IS NULL"
		}
		if p.Negated() {
			cond = "NOT (" + cond + ")"
		}
		return cond, nil, nil
	}

	fieldType := assetFieldTypes[p.Field]
	var cond string
	var args []interface{}

	if fieldType == schema.FieldTypeNumber {
		n, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64)
		if err != nil {
			return "", nil, fmt.Errorf("asset field '.%s' requires a numeric value", p.Field)
		}
		cond = fmt.Sprintf("%s %s ?", column, compareOpToSQL(p.CompareOp))
		args = []interface{}{n}
	} else {
		value := p.Value
		altValue := ""
		if p.IsRefValue && (p.Field == "id" || p.Field == "file_path") && (p.CompareOp == CompareEq || p.CompareOp == CompareNeq) {
			resolved, alt, err := e.resolveRefValue(p.Value)
			if err != nil {
				return "", nil, err
			}
			value = resolved
			altValue = alt
		}

		if p.CompareOp == CompareEq || p.CompareOp == CompareNeq {
			negate := p.CompareOp == CompareNeq
			cond, args = assetStringEqualsCond(column, value, negate)
			if altValue != "" {
				altCond, altArgs := assetStringEqualsCond(column, altValue, negate)
				if negate {
					cond = "(" + cond + " AND " + altCond + ")"
				} else {
					cond = "(" + cond + " OR " + altCond + ")"
				}
				args = append(args, altArgs...)
			}
		} else {
			cond = fmt.Sprintf("%s %s ?", column, compareOpToSQL(p.CompareOp))
			args = []interface{}{value}
		}
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

func assetStringEqualsCond(column, value string, negate bool) (string, []interface{}) {
	op := "="
	if negate {
		op = "!="
	}
	return fmt.Sprintf("LOWER(%s) %s LOWER(?)", column, op), []interface{}{value}
}

func (e *Executor) buildAssetStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	column, ok := assetColumnExpr(alias, p.Field)
	if !ok {
		return "", nil, fmt.Errorf("asset has no field '%s'", p.Field)
	}
	if assetFieldTypes[p.Field] != schema.FieldTypeString {
		return "", nil, fmt.Errorf("string function predicates are not valid for asset field '.%s'", p.Field)
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

func (e *Executor) buildAssetRefdPredicateSQL(p *RefdPredicate, alias string) (string, []interface{}, error) {
	var cond string
	var args []interface{}

	if p.Target != "" {
		sourceID, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE (r.source_id = ? OR r.source_id LIKE ?)
			  AND (r.target_id = %[1]s.id OR r.target_raw = %[1]s.id)
		)`, alias)
		args = append(args, sourceID, sourceID+"#%")
	} else if p.SubQuery != nil {
		if p.SubQuery.Type == QueryTypeObject {
			var err error
			cond, args, err = e.buildAssetRefdObjectSubquerySQL(p, alias)
			if err != nil {
				return "", nil, err
			}
		} else if p.SubQuery.Type == QueryTypeTrait {
			var err error
			cond, args, err = e.buildAssetRefdTraitSubquerySQL(p, alias)
			if err != nil {
				return "", nil, err
			}
		} else {
			return "", nil, fmt.Errorf("asset refd() only supports type or trait subqueries")
		}
	} else {
		return "", nil, fmt.Errorf("refd predicate must have source or subquery")
	}

	if p.Negated() {
		cond = "NOT " + cond
	}
	return cond, args, nil
}

func (e *Executor) buildAssetRefdObjectSubquerySQL(p *RefdPredicate, alias string) (string, []interface{}, error) {
	var sourceConditions []string
	var args []interface{}

	sourceConditions = append(sourceConditions, "src.type = ?")
	args = append(args, p.SubQuery.TypeName)

	if p.SubQuery.Predicate != nil {
		cond, predArgs, err := e.buildObjectPredicateSQL(p.SubQuery.Predicate, "src", p.SubQuery.TypeName)
		if err != nil {
			return "", nil, err
		}
		sourceConditions = append(sourceConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM refs r
		JOIN objects src ON (r.source_id = src.id OR r.source_id LIKE src.id || '#%%')
		WHERE (r.target_id = %[1]s.id OR r.target_raw = %[1]s.id)
		  AND %[2]s
	)`, alias, strings.Join(sourceConditions, " AND "))

	return cond, args, nil
}

func (e *Executor) buildAssetRefdTraitSubquerySQL(p *RefdPredicate, alias string) (string, []interface{}, error) {
	var sourceConditions []string
	var args []interface{}

	sourceConditions = append(sourceConditions, "src_t.trait_type = ?")
	args = append(args, p.SubQuery.TypeName)

	if p.SubQuery.Predicate != nil {
		cond, predArgs, err := e.buildTraitPredicateSQL(p.SubQuery.Predicate, "src_t")
		if err != nil {
			return "", nil, err
		}
		sourceConditions = append(sourceConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM refs r
		JOIN traits src_t ON r.file_path = src_t.file_path
		                 AND r.line_number = src_t.line_number
		WHERE (r.target_id = %[1]s.id OR r.target_raw = %[1]s.id)
		  AND %[2]s
	)`, alias, strings.Join(sourceConditions, " AND "))

	return cond, args, nil
}
