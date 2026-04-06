package query

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/sqlutil"
)

func scanObjectRows(rows *sql.Rows) ([]model.Object, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Object, error) {
		var r model.Object
		var fieldsJSON string
		if err := rows.Scan(&r.ID, &r.Type, &fieldsJSON, &r.FilePath, &r.LineStart, &r.ParentID); err != nil {
			return model.Object{}, err
		}
		if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
			r.Fields = make(map[string]interface{})
		}
		return r, nil
	})
}

func scanTraitRows(rows *sql.Rows) ([]model.Trait, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Trait, error) {
		var r model.Trait
		if err := rows.Scan(&r.ID, &r.TraitType, &r.Value, &r.Content, &r.FilePath, &r.Line, &r.ParentObjectID); err != nil {
			return model.Trait{}, err
		}
		return r, nil
	})
}

func scanIDRows(rows *sql.Rows) ([]string, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (string, error) {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		return id, nil
	})
}

func (e *Executor) executeCountQuery(sqlStr string, args []interface{}) (int, error) {
	var count int
	if err := e.db.QueryRow(sqlStr, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// executeObjectQuery executes an object query and returns matching objects.
// External callers should use ExecuteObjectQuery.
func (e *Executor) executeObjectQuery(q *Query) ([]model.Object, error) {
	return e.executeObjectPageQuery(q, 0, 0)
}

func (e *Executor) executeObjectPageQuery(q *Query, limit, offset int) ([]model.Object, error) {
	if q.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object query, got trait query")
	}

	sqlStr, args, err := e.buildObjectPageSQL(q, limit, offset)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}
	return scanObjectRows(rows)
}

func (e *Executor) executeObjectIDQuery(q *Query, limit, offset int) ([]string, error) {
	if q.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object query, got trait query")
	}

	sqlStr, args, err := e.buildObjectIDSQL(q, limit, offset)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}
	return scanIDRows(rows)
}

func (e *Executor) executeObjectCountQuery(q *Query) (int, error) {
	if q.Type != QueryTypeObject {
		return 0, fmt.Errorf("expected object query, got trait query")
	}

	sqlStr, args, err := e.buildObjectCountSQL(q)
	if err != nil {
		return 0, err
	}

	count, err := e.executeCountQuery(sqlStr, args)
	if err != nil {
		return 0, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}
	return count, nil
}

// executeTraitQuery executes a trait query and returns matching traits.
// External callers should use ExecuteTraitQuery.
func (e *Executor) executeTraitQuery(q *Query) ([]model.Trait, error) {
	return e.executeTraitPageQuery(q, 0, 0)
}

func (e *Executor) executeTraitPageQuery(q *Query, limit, offset int) ([]model.Trait, error) {
	if q.Type != QueryTypeTrait {
		return nil, fmt.Errorf("expected trait query, got object query")
	}

	sqlStr, args, err := e.buildTraitPageSQL(q, limit, offset)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}

	return scanTraitRows(rows)
}

func (e *Executor) executeTraitIDQuery(q *Query, limit, offset int) ([]string, error) {
	if q.Type != QueryTypeTrait {
		return nil, fmt.Errorf("expected trait query, got object query")
	}

	sqlStr, args, err := e.buildTraitIDSQL(q, limit, offset)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}

	return scanIDRows(rows)
}

func (e *Executor) executeTraitCountQuery(q *Query) (int, error) {
	if q.Type != QueryTypeTrait {
		return 0, fmt.Errorf("expected trait query, got object query")
	}

	sqlStr, args, err := e.buildTraitCountSQL(q)
	if err != nil {
		return 0, err
	}

	count, err := e.executeCountQuery(sqlStr, args)
	if err != nil {
		return 0, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}
	return count, nil
}

// ExecuteObjectQuery executes an object query and returns matching objects.
func (e *Executor) ExecuteObjectQuery(q *Query) ([]model.Object, error) {
	return e.withExecutionNow().executeObjectQuery(q)
}

// ExecuteObjectPageQuery executes an object query with SQL-level pagination.
func (e *Executor) ExecuteObjectPageQuery(q *Query, limit, offset int) ([]model.Object, error) {
	return e.withExecutionNow().executeObjectPageQuery(q, limit, offset)
}

// ExecuteObjectIDQuery executes an object query returning only object IDs.
func (e *Executor) ExecuteObjectIDQuery(q *Query, limit, offset int) ([]string, error) {
	return e.withExecutionNow().executeObjectIDQuery(q, limit, offset)
}

// ExecuteObjectCountQuery executes an object query as COUNT(*).
func (e *Executor) ExecuteObjectCountQuery(q *Query) (int, error) {
	return e.withExecutionNow().executeObjectCountQuery(q)
}

// ExecuteTraitQuery executes a trait query and returns matching traits.
func (e *Executor) ExecuteTraitQuery(q *Query) ([]model.Trait, error) {
	return e.withExecutionNow().executeTraitQuery(q)
}

// ExecuteTraitPageQuery executes a trait query with SQL-level pagination.
func (e *Executor) ExecuteTraitPageQuery(q *Query, limit, offset int) ([]model.Trait, error) {
	return e.withExecutionNow().executeTraitPageQuery(q, limit, offset)
}

// ExecuteTraitIDQuery executes a trait query returning only trait IDs.
func (e *Executor) ExecuteTraitIDQuery(q *Query, limit, offset int) ([]string, error) {
	return e.withExecutionNow().executeTraitIDQuery(q, limit, offset)
}

// ExecuteTraitCountQuery executes a trait query as COUNT(*).
func (e *Executor) ExecuteTraitCountQuery(q *Query) (int, error) {
	return e.withExecutionNow().executeTraitCountQuery(q)
}
