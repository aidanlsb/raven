package query

import (
	"fmt"
	"strconv"
	"strings"
)

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
	// .field==* means field exists, .field!=* means field doesn't exist
	if negated {
		return fmt.Sprintf("json_extract(%s.fields, ?) IS NULL", alias), []interface{}{jsonPath}
	}
	return fmt.Sprintf("json_extract(%s.fields, ?) IS NOT NULL", alias), []interface{}{jsonPath}
}

// fieldScalarOrArrayCIEqualsCond returns a condition that matches scalar equality OR array membership
// (case-insensitive). If negate is true, it returns the corresponding "not equals / not in array" form.
func fieldScalarOrArrayCIEqualsCond(alias string, jsonPath string, value string, negate bool) (string, []interface{}) {
	// Prefer numeric equality when the RHS parses as a number.
	// This matches the behavior of value predicates (value==10) and avoids lexicographic surprises.
	if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
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
