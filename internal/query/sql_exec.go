package query

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/sqlutil"
)

// executeObjectQuery executes an object query and returns matching objects.
// This is internal - external callers should use ExecuteObjectQueryWithPipeline.
func (e *Executor) executeObjectQuery(q *Query) ([]model.Object, error) {
	if q.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object query, got trait query")
	}

	sqlStr, args, err := e.buildObjectSQL(q)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}
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

// executeTraitQuery executes a trait query and returns matching traits.
// This is internal - external callers should use ExecuteTraitQueryWithPipeline.
func (e *Executor) executeTraitQuery(q *Query) ([]model.Trait, error) {
	if q.Type != QueryTypeTrait {
		return nil, fmt.Errorf("expected trait query, got object query")
	}

	sqlStr, args, err := e.buildTraitSQL(q)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sqlStr)
	}

	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Trait, error) {
		var r model.Trait
		if err := rows.Scan(&r.ID, &r.TraitType, &r.Value, &r.Content, &r.FilePath, &r.Line, &r.ParentObjectID); err != nil {
			return model.Trait{}, err
		}
		return r, nil
	})
}
