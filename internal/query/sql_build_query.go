package query

import (
	"fmt"
	"strings"
)

// buildObjectSQL builds SQL for an object query.
func (e *Executor) buildObjectSQL(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	// Type filter
	conditions = append(conditions, "o.type = ?")
	args = append(args, q.TypeName)

	// Build predicate condition
	if q.Predicate != nil {
		cond, predArgs, err := e.buildObjectPredicateSQL(q.Predicate, "o")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	sqlStr := fmt.Sprintf(`
		SELECT o.id, o.type, o.fields, o.file_path, o.line_start, o.parent_id
		FROM objects o
		WHERE %s
		ORDER BY o.file_path, o.line_start
	`, strings.Join(conditions, " AND "))

	return sqlStr, args, nil
}

// buildTraitSQL builds SQL for a trait query.
func (e *Executor) buildTraitSQL(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	// Trait type filter
	conditions = append(conditions, "t.trait_type = ?")
	args = append(args, q.TypeName)

	// Build predicate condition
	if q.Predicate != nil {
		cond, predArgs, err := e.buildTraitPredicateSQL(q.Predicate, "t")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	sqlStr := fmt.Sprintf(`
		SELECT t.id, t.trait_type, t.value, t.content, t.file_path, t.line_number, t.parent_object_id
		FROM traits t
		WHERE %s
		ORDER BY t.file_path, t.line_number
	`, strings.Join(conditions, " AND "))

	return sqlStr, args, nil
}
