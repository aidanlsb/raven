package query

import (
	"fmt"
	"strconv"
	"strings"
)

// buildFieldPredicateSQL builds SQL for .field==value predicates.
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	jsonPath := jsonFieldPath(p.Field)

	var cond string
	var args []interface{}

	if p.IsExists {
		cond, args = fieldExistsCond(alias, jsonPath, p.CompareOp == CompareNeq)
	} else if p.CompareOp == CompareNeq {
		cond, args = fieldScalarOrArrayCIEqualsCond(alias, jsonPath, p.Value, true)
	} else if p.CompareOp != CompareEq {
		// Comparison operators: <, >, <=, >=
		op := compareOpToSQL(p.CompareOp)
		// Prefer numeric comparisons when RHS parses as a number. This avoids
		// lexicographic comparisons like "10" < "2".
		if n, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64); err == nil {
			cond = fmt.Sprintf("CAST(json_extract(%s.fields, ?) AS REAL) %s ?", alias, op)
			args = append(args, jsonPath, n)
		} else {
			cond = fmt.Sprintf("json_extract(%s.fields, ?) %s ?", alias, op)
			args = append(args, jsonPath, p.Value)
		}
	} else {
		cond, args = fieldScalarOrArrayCIEqualsCond(alias, jsonPath, p.Value, false)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildStringFuncPredicateSQL builds SQL for string function predicates.
// Handles: includes(.field, "str"), startswith(.field, "str"), endswith(.field, "str"), matches(.field, "pattern")
func (e *Executor) buildStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	jsonPath := jsonFieldPath(p.Field)
	fieldExpr := fmt.Sprintf("json_extract(%s.fields, ?)", alias)

	cond, funcArgs, err := buildStringFuncCondition(p.FuncType, fieldExpr, p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}
	args := append([]interface{}{jsonPath}, funcArgs...)

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildArrayQuantifierPredicateSQL builds SQL for array quantifier predicates.
// Handles: any(.tags, _ == "urgent"), all(.tags, startswith(_, "feature-")), none(.tags, _ == "deprecated")
func (e *Executor) buildArrayQuantifierPredicateSQL(p *ArrayQuantifierPredicate, alias string) (string, []interface{}, error) {
	jsonPath := fmt.Sprintf("$.%s", p.Field)

	var cond string
	var args []interface{}

	// Build the element condition
	elemCond, elemArgs, err := e.buildElementPredicateSQL(p.ElementPred)
	if err != nil {
		return "", nil, err
	}

	switch p.Quantifier {
	case ArrayQuantifierAny:
		// EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE <elemCond>)
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE %s
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)

	case ArrayQuantifierAll:
		// NOT EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE NOT <elemCond>)
		// This means: there is no element that doesn't satisfy the condition
		cond = fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE NOT (%s)
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)

	case ArrayQuantifierNone:
		// NOT EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE <elemCond>)
		cond = fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE %s
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildElementPredicateSQL builds SQL for predicates used within array quantifiers.
// The context is json_each.value representing the current array element.
func (e *Executor) buildElementPredicateSQL(pred Predicate) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *ElementEqualityPredicate:
		return e.buildElementEqualitySQL(p)

	case *StringFuncPredicate:
		if !p.IsElementRef {
			return "", nil, fmt.Errorf("string function in array context must use _ as first argument")
		}
		return e.buildElementStringFuncSQL(p)

	case *OrPredicate:
		leftCond, leftArgs, err := e.buildElementPredicateSQL(p.Left)
		if err != nil {
			return "", nil, err
		}
		rightCond, rightArgs, err := e.buildElementPredicateSQL(p.Right)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf("(%s OR %s)", leftCond, rightCond)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, append(leftArgs, rightArgs...), nil

	case *GroupPredicate:
		var conditions []string
		var args []interface{}
		for _, subPred := range p.Predicates {
			cond, predArgs, err := e.buildElementPredicateSQL(subPred)
			if err != nil {
				return "", nil, err
			}
			conditions = append(conditions, cond)
			args = append(args, predArgs...)
		}
		cond := "(" + strings.Join(conditions, " AND ") + ")"
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil

	default:
		return "", nil, fmt.Errorf("unsupported element predicate type: %T", pred)
	}
}

// buildElementEqualitySQL builds SQL for _ == value or _ != value.
func (e *Executor) buildElementEqualitySQL(p *ElementEqualityPredicate) (string, []interface{}, error) {
	// Reuse the same value-comparison SQL semantics as value predicates:
	// - numeric RHS => numeric compare via CAST(... AS REAL)
	// - string equality => case-insensitive
	vp := &ValuePredicate{
		basePredicate: p.basePredicate,
		Value:         p.Value,
		CompareOp:     p.CompareOp,
	}
	cond, args := buildValueCondition(vp, "json_each.value")
	return cond, args, nil
}

// buildElementStringFuncSQL builds SQL for string functions on array elements.
func (e *Executor) buildElementStringFuncSQL(p *StringFuncPredicate) (string, []interface{}, error) {
	cond, args, err := buildStringFuncCondition(p.FuncType, "json_each.value", p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}
