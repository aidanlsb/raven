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

// The internal execute* methods below are thin, non-scoping adapters over the
// generic entity runners in entity_query.go. They keep the typed method surface
// (used by unit tests, the public Execute* wrappers, and Execute) while the
// SQL/scan logic lives in exactly one place.

func (e *Executor) executeObjectQuery(q *Query) ([]model.Object, error) {
	return runEntityPageRows(e, q, objectSpec, 0, 0, scanObjectRows)
}

func (e *Executor) executeObjectPageQuery(q *Query, limit, offset int) ([]model.Object, error) {
	return runEntityPageRows(e, q, objectSpec, limit, offset, scanObjectRows)
}

func (e *Executor) executeObjectIDQuery(q *Query, limit, offset int) ([]string, error) {
	return runEntityIDs(e, q, objectSpec, limit, offset)
}

func (e *Executor) executeObjectCountQuery(q *Query) (int, error) {
	return runEntityCount(e, q, objectSpec)
}

func (e *Executor) executeTraitQuery(q *Query) ([]model.Trait, error) {
	return runEntityPageRows(e, q, traitSpec, 0, 0, scanTraitRows)
}

func (e *Executor) executeTraitPageQuery(q *Query, limit, offset int) ([]model.Trait, error) {
	return runEntityPageRows(e, q, traitSpec, limit, offset, scanTraitRows)
}

func (e *Executor) executeTraitIDQuery(q *Query, limit, offset int) ([]string, error) {
	return runEntityIDs(e, q, traitSpec, limit, offset)
}

func (e *Executor) executeTraitCountQuery(q *Query) (int, error) {
	return runEntityCount(e, q, traitSpec)
}

func (e *Executor) executeSectionQuery(q *Query) ([]model.Section, error) {
	return runEntityPageRows(e, q, sectionSpec, 0, 0, scanSectionRows)
}

func (e *Executor) executeSectionPageQuery(q *Query, limit, offset int) ([]model.Section, error) {
	return runEntityPageRows(e, q, sectionSpec, limit, offset, scanSectionRows)
}

func (e *Executor) executeSectionIDQuery(q *Query, limit, offset int) ([]string, error) {
	return runEntityIDs(e, q, sectionSpec, limit, offset)
}

func (e *Executor) executeSectionCountQuery(q *Query) (int, error) {
	return runEntityCount(e, q, sectionSpec)
}

func (e *Executor) executeLinkQuery(q *Query) ([]model.Link, error) {
	return runEntityPageRows(e, q, linkSpec, 0, 0, scanLinkRows)
}

func (e *Executor) executeLinkPageQuery(q *Query, limit, offset int) ([]model.Link, error) {
	return runEntityPageRows(e, q, linkSpec, limit, offset, scanLinkRows)
}

func (e *Executor) executeLinkIDQuery(q *Query, limit, offset int) ([]string, error) {
	return runEntityIDs(e, q, linkSpec, limit, offset)
}

func (e *Executor) executeLinkCountQuery(q *Query) (int, error) {
	return runEntityCount(e, q, linkSpec)
}

// ExecuteObjectQuery executes a type query and returns matching objects.
func (e *Executor) ExecuteObjectQuery(q *Query) ([]model.Object, error) {
	return e.withExecutionNow().executeObjectQuery(q)
}

// ExecuteObjectPageQuery executes a type query with SQL-level pagination.
func (e *Executor) ExecuteObjectPageQuery(q *Query, limit, offset int) ([]model.Object, error) {
	return e.withExecutionNow().executeObjectPageQuery(q, limit, offset)
}

// ExecuteObjectIDQuery executes a type query returning only item IDs.
func (e *Executor) ExecuteObjectIDQuery(q *Query, limit, offset int) ([]string, error) {
	return e.withExecutionNow().executeObjectIDQuery(q, limit, offset)
}

// ExecuteObjectCountQuery executes a type query as COUNT(*).
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

func (e *Executor) ExecuteSectionQuery(q *Query) ([]model.Section, error) {
	return e.withExecutionNow().executeSectionQuery(q)
}

func (e *Executor) ExecuteSectionPageQuery(q *Query, limit, offset int) ([]model.Section, error) {
	return e.withExecutionNow().executeSectionPageQuery(q, limit, offset)
}

func (e *Executor) ExecuteSectionIDQuery(q *Query, limit, offset int) ([]string, error) {
	return e.withExecutionNow().executeSectionIDQuery(q, limit, offset)
}

func (e *Executor) ExecuteSectionCountQuery(q *Query) (int, error) {
	return e.withExecutionNow().executeSectionCountQuery(q)
}

// ExecuteLinkQuery executes a link query and returns matching outgoing edges.
func (e *Executor) ExecuteLinkQuery(q *Query) ([]model.Link, error) {
	return e.withExecutionNow().executeLinkQuery(q)
}

// ExecuteLinkPageQuery executes a link query with SQL-level pagination.
func (e *Executor) ExecuteLinkPageQuery(q *Query, limit, offset int) ([]model.Link, error) {
	return e.withExecutionNow().executeLinkPageQuery(q, limit, offset)
}

// ExecuteLinkIDQuery executes a link query returning source IDs, one per edge.
func (e *Executor) ExecuteLinkIDQuery(q *Query, limit, offset int) ([]string, error) {
	return e.withExecutionNow().executeLinkIDQuery(q, limit, offset)
}

// ExecuteLinkCountQuery executes a link query as COUNT(*).
func (e *Executor) ExecuteLinkCountQuery(q *Query) (int, error) {
	return e.withExecutionNow().executeLinkCountQuery(q)
}
