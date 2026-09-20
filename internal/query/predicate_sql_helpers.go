package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/indexschema"
)

func wrapNot(cond string, negated bool) string {
	if negated {
		return "NOT (" + cond + ")"
	}
	return cond
}

func compileBooleanPredicate(pred Predicate, leaf func(Predicate) (string, []interface{}, error)) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *OrPredicate:
		var conditions []string
		var args []interface{}
		for _, subPred := range p.Predicates {
			cond, predArgs, err := compileBooleanPredicate(subPred, leaf)
			if err != nil {
				return "", nil, err
			}
			conditions = append(conditions, cond)
			args = append(args, predArgs...)
		}
		return "(" + strings.Join(conditions, " OR ") + ")", args, nil
	case *NotPredicate:
		cond, args, err := compileBooleanPredicate(p.Inner, leaf)
		if err != nil {
			return "", nil, err
		}
		return "NOT (" + cond + ")", args, nil
	case *GroupPredicate:
		var conditions []string
		var args []interface{}
		for _, subPred := range p.Predicates {
			cond, predArgs, err := compileBooleanPredicate(subPred, leaf)
			if err != nil {
				return "", nil, err
			}
			conditions = append(conditions, cond)
			args = append(args, predArgs...)
		}
		return "(" + strings.Join(conditions, " AND ") + ")", args, nil
	default:
		return leaf(pred)
	}
}

func compareOpToSQL(op CompareOp) string {
	switch op {
	case CompareNeq:
		return "!="
	case CompareLt:
		return "<"
	case CompareGt:
		return ">"
	case CompareLte:
		return "<="
	case CompareGte:
		return ">="
	default:
		return "="
	}
}

// tryTemporalCompareSQL compiles a date/datetime comparison when value is temporal.
// A non-temporal value returns ok=false and a nil error so callers can fall back.
// An invalid temporal value returns an error; callers must fail the query rather
// than falling back to string comparison.
func tryTemporalCompareSQL(value string, compareOp CompareOp, fieldExpr string, now time.Time) (string, []interface{}, bool, error) {
	return indexschema.TryParseTemporalComparisonWithOptions(
		value,
		compareOpToSQL(compareOp),
		fieldExpr,
		indexschema.DateFilterOptions{Now: now},
	)
}

func likeCond(expr string, wrapLower bool) string {
	// Always include ESCAPE so callers can safely escape % and _.
	if wrapLower {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?) ESCAPE '\\'", expr)
	}
	return fmt.Sprintf("%s LIKE ? ESCAPE '\\'", expr)
}

func jsonFieldPath(field string) string {
	// Field names are validated by the parser; keep it simple and parameterize the path.
	return "$." + field
}

func fieldExistsCond(alias string, jsonPath string, negated bool) (string, []interface{}) {
	if negated {
		return fmt.Sprintf("json_extract(%s.fields, ?) IS NULL", alias), []interface{}{jsonPath}
	}
	return fmt.Sprintf("json_extract(%s.fields, ?) IS NOT NULL", alias), []interface{}{jsonPath}
}

type fieldEqualityMode int

const (
	fieldEqualityModeFallback fieldEqualityMode = iota
	fieldEqualityModeScalar
	fieldEqualityModeArray
)

// fieldScalarOrArrayCIEqualsCond returns a condition that matches scalar equality OR array membership
// (case-insensitive). If negate is true, it returns the corresponding "not equals / not in array" form.
func fieldScalarOrArrayCIEqualsCond(alias string, jsonPath string, value string, negate bool, mode fieldEqualityMode) (string, []interface{}) {
	// Prefer numeric equality when the RHS parses as a number.
	// This matches the behavior of value predicates (value==10) and avoids lexicographic surprises.
	if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		switch mode {
		case fieldEqualityModeScalar:
			return fieldScalarNumericEqualsCond(alias, jsonPath, n, negate)
		case fieldEqualityModeArray:
			return fieldArrayNumericEqualsCond(alias, jsonPath, n, negate)
		}

		if negate {
			return fmt.Sprintf(`(
				CAST(json_extract(%s.fields, ?) AS REAL) != ? AND
				NOT EXISTS (
					SELECT 1 FROM json_each(%s.fields, ?)
					WHERE CAST(json_each.value AS REAL) = ?
				)
			)`, alias, alias), []interface{}{jsonPath, n, jsonPath, n}
		}

		return fmt.Sprintf(`(
			CAST(json_extract(%s.fields, ?) AS REAL) = ? OR
			EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE CAST(json_each.value AS REAL) = ?
			)
		)`, alias, alias), []interface{}{jsonPath, n, jsonPath, n}
	}

	switch mode {
	case fieldEqualityModeScalar:
		return fieldScalarCIEqualsCond(alias, jsonPath, value, negate)
	case fieldEqualityModeArray:
		return fieldArrayCIEqualsCond(alias, jsonPath, value, negate)
	}

	// Equality: LOWER(json_extract(...)) = LOWER(?) OR EXISTS json_each(...) = LOWER(?)
	// Not equals: LOWER(json_extract(...)) != LOWER(?) AND NOT EXISTS json_each(...) = LOWER(?)
	if negate {
		return fmt.Sprintf(`(
			LOWER(json_extract(%s.fields, ?)) != LOWER(?) AND
			NOT EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE LOWER(json_each.value) = LOWER(?)
			)
		)`, alias, alias), []interface{}{jsonPath, value, jsonPath, value}
	}

	return fmt.Sprintf(`(
		LOWER(json_extract(%s.fields, ?)) = LOWER(?) OR
		EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE LOWER(json_each.value) = LOWER(?)
		)
	)`, alias, alias), []interface{}{jsonPath, value, jsonPath, value}
}

func fieldScalarNumericEqualsCond(alias, jsonPath string, value float64, negate bool) (string, []interface{}) {
	op := "="
	if negate {
		op = "!="
	}
	return fmt.Sprintf("CAST(json_extract(%s.fields, ?) AS REAL) %s ?", alias, op), []interface{}{jsonPath, value}
}

func fieldArrayNumericEqualsCond(alias, jsonPath string, value float64, negate bool) (string, []interface{}) {
	if negate {
		return fmt.Sprintf(`(
			json_extract(%s.fields, ?) IS NOT NULL AND
			NOT EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE CAST(json_each.value AS REAL) = ?
			)
		)`, alias, alias), []interface{}{jsonPath, jsonPath, value}
	}

	return fmt.Sprintf(`EXISTS (
		SELECT 1 FROM json_each(%s.fields, ?)
		WHERE CAST(json_each.value AS REAL) = ?
	)`, alias), []interface{}{jsonPath, value}
}

func fieldScalarCIEqualsCond(alias, jsonPath, value string, negate bool) (string, []interface{}) {
	op := "="
	if negate {
		op = "!="
	}
	return fmt.Sprintf("LOWER(json_extract(%s.fields, ?)) %s LOWER(?)", alias, op), []interface{}{jsonPath, value}
}

func fieldArrayCIEqualsCond(alias, jsonPath, value string, negate bool) (string, []interface{}) {
	if negate {
		return fmt.Sprintf(`(
			json_extract(%s.fields, ?) IS NOT NULL AND
			NOT EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE LOWER(json_each.value) = LOWER(?)
			)
		)`, alias, alias), []interface{}{jsonPath, jsonPath, value}
	}

	return fmt.Sprintf(`EXISTS (
		SELECT 1 FROM json_each(%s.fields, ?)
		WHERE LOWER(json_each.value) = LOWER(?)
	)`, alias), []interface{}{jsonPath, value}
}
