package query

import (
	"fmt"
	"strings"
)

func appendLimitOffset(sqlStr string, args []interface{}, limit, offset int) (string, []interface{}) {
	switch {
	case limit > 0 && offset > 0:
		sqlStr += "\nLIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	case limit > 0:
		sqlStr += "\nLIMIT ?"
		args = append(args, limit)
	case offset > 0:
		sqlStr += "\nLIMIT -1 OFFSET ?"
		args = append(args, offset)
	}
	return sqlStr, args
}

func (e *Executor) buildObjectWhereClause(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "o.type = ?")
	args = append(args, q.TypeName)

	if err := e.prepareRefFieldAmbiguityChecks(q); err != nil {
		return "", nil, err
	}

	if q.Predicate != nil {
		cond, predArgs, err := e.buildObjectPredicateSQL(q.Predicate, "o", q.TypeName)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return strings.Join(conditions, " AND "), args, nil
}

func (e *Executor) buildTraitWhereClause(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "t.trait_type = ?")
	args = append(args, q.TypeName)

	if q.Predicate != nil {
		cond, predArgs, err := e.buildTraitPredicateSQL(q.Predicate, "t")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return strings.Join(conditions, " AND "), args, nil
}

func (e *Executor) buildAssetWhereClause(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "1=1")

	if q.Predicate != nil {
		cond, predArgs, err := e.buildAssetPredicateSQL(q.Predicate, "a")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return strings.Join(conditions, " AND "), args, nil
}

func (e *Executor) buildSectionWhereClause(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "1=1")

	if q.Predicate != nil {
		cond, predArgs, err := e.buildSectionPredicateSQL(q.Predicate, "s")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return strings.Join(conditions, " AND "), args, nil
}

func (e *Executor) buildObjectPageSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildObjectWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT o.id, o.type, o.fields, o.file_path, o.line_start
		FROM objects o
		WHERE %s
		ORDER BY o.file_path, o.line_start
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildObjectIDSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildObjectWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT o.id
		FROM objects o
		WHERE %s
		ORDER BY o.file_path, o.line_start
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildObjectCountSQL(q *Query) (string, []interface{}, error) {
	whereClause, args, err := e.buildObjectWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM objects o
		WHERE %s
	`, whereClause)
	return sqlStr, args, nil
}

func (e *Executor) buildTraitPageSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildTraitWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT t.id, t.trait_type, t.value, t.content, t.file_path, t.line_number, t.parent_object_id
		FROM traits t
		WHERE %s
		ORDER BY t.file_path, t.line_number
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildTraitIDSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildTraitWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT t.id
		FROM traits t
		WHERE %s
		ORDER BY t.file_path, t.line_number
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildTraitCountSQL(q *Query) (string, []interface{}, error) {
	whereClause, args, err := e.buildTraitWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM traits t
		WHERE %s
	`, whereClause)
	return sqlStr, args, nil
}

func (e *Executor) buildAssetPageSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildAssetWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT a.id, a.file_path, COALESCE(a.media_type, ''), COALESCE(a.extension, ''),
		       a.filename, a.size_bytes, COALESCE(a.file_mtime, 0), COALESCE(a.indexed_at, 0)
		FROM assets a
		WHERE %s
		ORDER BY a.file_path
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildAssetIDSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildAssetWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT a.id
		FROM assets a
		WHERE %s
		ORDER BY a.file_path
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildAssetCountSQL(q *Query) (string, []interface{}, error) {
	whereClause, args, err := e.buildAssetWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM assets a
		WHERE %s
	`, whereClause)
	return sqlStr, args, nil
}

func (e *Executor) buildSectionPageSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildSectionWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT s.id, s.file_object_id, s.file_path, s.slug, s.title, s.level, s.line_start, s.line_end, s.parent_section_id
		FROM sections s
		WHERE %s
		ORDER BY s.file_path, s.line_start
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildSectionIDSQL(q *Query, limit, offset int) (string, []interface{}, error) {
	whereClause, args, err := e.buildSectionWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT s.id
		FROM sections s
		WHERE %s
		ORDER BY s.file_path, s.line_start
	`, whereClause)

	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildSectionCountSQL(q *Query) (string, []interface{}, error) {
	whereClause, args, err := e.buildSectionWhereClause(q)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM sections s
		WHERE %s
	`, whereClause)
	return sqlStr, args, nil
}
