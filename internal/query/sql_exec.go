package query

import (
	"database/sql"
	"fmt"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/sqlutil"
)

// wrapQueryExecError wraps a database execution error with a concise, stable
// message. The generated SQL is intentionally omitted so it does not leak into
// user/agent-facing DATABASE_ERROR messages; the underlying DB error is
// preserved via %w for programmatic inspection.
func wrapQueryExecError(err error) error {
	return fmt.Errorf("query failed: %w", err)
}

func scanObjectRows(rows *sql.Rows) ([]model.Object, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Object, error) {
		var r model.Object
		var fieldsJSON string
		if err := rows.Scan(&r.ID, &r.Type, &fieldsJSON, &r.FilePath, &r.LineStart); err != nil {
			return model.Object{}, err
		}
		fields, err := fieldvalue.FieldsFromJSON([]byte(fieldsJSON))
		if err != nil || fields == nil {
			fields = make(map[string]fieldvalue.FieldValue)
		}
		r.Fields = fields
		return r, nil
	})
}

func scanTraitRows(rows *sql.Rows) ([]model.Trait, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Trait, error) {
		var r model.Trait
		var value sql.NullString
		if err := rows.Scan(&r.ID, &r.TraitType, &value, &r.Content, &r.FilePath, &r.Line, &r.ParentScopeID); err != nil {
			return model.Trait{}, err
		}
		if value.Valid {
			s := value.String
			r.SetIndexValueString(&s)
		}
		return r, nil
	})
}

func scanSectionRows(rows *sql.Rows) ([]model.Section, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Section, error) {
		var r model.Section
		if err := rows.Scan(
			&r.ID,
			&r.FileObjectID,
			&r.FilePath,
			&r.Slug,
			&r.Title,
			&r.Level,
			&r.LineStart,
			&r.LineEnd,
			&r.SubtreeLineEnd,
			&r.ParentSectionID,
		); err != nil {
			return model.Section{}, err
		}
		return r, nil
	})
}

func scanLinkRows(rows *sql.Rows) ([]model.Link, error) {
	return sqlutil.ScanRows(rows, func(rows *sql.Rows) (model.Link, error) {
		var r model.Link
		if err := rows.Scan(
			&r.SourceID,
			&r.SourceType,
			&r.FilePath,
			&r.Line,
			&r.PositionStart,
			&r.PositionEnd,
			&r.RawTarget,
			&r.Display,
			&r.IsImage,
			&r.Scheme,
			&r.Ext,
			&r.NormalizedKey,
		); err != nil {
			return model.Link{}, err
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
