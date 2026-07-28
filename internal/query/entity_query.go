package query

import (
	"database/sql"
	"fmt"
	"strings"
)

// entitySQLSpec captures everything that differs between the four query roots
// (object/trait/section/link) when generating page/id/count SQL. Everything
// else — the WHERE-clause assembly, LIMIT/OFFSET handling, row execution, and
// COUNT execution — is shared, so adding or changing a root is a single-spec
// edit rather than a copy-paste across a dozen functions.
//
// Row scanning stays type-specific (each root maps to a distinct model type),
// so scan functions are supplied by callers rather than stored on the spec.
type entitySQLSpec struct {
	queryType  QueryType
	table      string
	alias      string
	rowColumns string
	idColumn   string
	orderBy    string
	// typeColumn is the discriminator column (e.g. "o.type"). Empty for roots
	// that have no discriminator (sections, links), which use a "1=1" base.
	typeColumn string
}

var (
	objectSpec = entitySQLSpec{
		queryType:  QueryTypeObject,
		table:      "objects",
		alias:      "o",
		rowColumns: "o.id, o.type, o.fields, o.file_path, o.line_start",
		idColumn:   "o.id",
		orderBy:    "o.file_path, o.line_start",
		typeColumn: "o.type",
	}
	traitSpec = entitySQLSpec{
		queryType:  QueryTypeTrait,
		table:      "traits",
		alias:      "t",
		rowColumns: "t.id, t.trait_type, t.value, t.content, t.file_path, t.line_number, t.parent_object_id",
		idColumn:   "t.id",
		orderBy:    "t.file_path, t.line_number",
		typeColumn: "t.trait_type",
	}
	sectionSpec = entitySQLSpec{
		queryType:  QueryTypeSection,
		table:      "sections",
		alias:      "s",
		rowColumns: "s.id, s.file_object_id, s.file_path, s.slug, s.title, s.level, s.line_start, s.line_end, s.subtree_line_end, s.parent_section_id",
		idColumn:   "s.id",
		orderBy:    "s.file_path, s.line_start",
		typeColumn: "",
	}
	linkSpec = entitySQLSpec{
		queryType: QueryTypeLink,
		table:     "links",
		alias:     "l",
		rowColumns: "l.source_id, l.source_type, l.file_path, l.line_number, l.position_start, l.position_end, " +
			"l.raw_target, l.display, l.is_image, l.scheme, l.ext, l.normalized_key",
		// Links have no durable edge ID. In IDs-only mode, project the source
		// object ID once per matching edge.
		idColumn:   "l.source_id",
		orderBy:    "l.file_path, l.line_number, l.position_start, l.id",
		typeColumn: "",
	}
)

func specForQueryType(t QueryType) (entitySQLSpec, error) {
	switch t {
	case QueryTypeObject:
		return objectSpec, nil
	case QueryTypeTrait:
		return traitSpec, nil
	case QueryTypeSection:
		return sectionSpec, nil
	case QueryTypeLink:
		return linkSpec, nil
	default:
		return entitySQLSpec{}, fmt.Errorf("unsupported query type: %d", t)
	}
}

func queryTypeName(t QueryType) string {
	switch t {
	case QueryTypeObject:
		return "type"
	case QueryTypeTrait:
		return "trait"
	case QueryTypeSection:
		return "section"
	case QueryTypeLink:
		return "link"
	default:
		return "unknown"
	}
}

func errUnexpectedQueryType(want, got QueryType) error {
	return fmt.Errorf("expected %s query, got %s query", queryTypeName(want), queryTypeName(got))
}

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

// buildEntityWhereClause assembles the shared WHERE clause for a root: the
// optional type discriminator plus the (matrix-checked) predicate SQL. Object
// queries additionally run the ref-field ambiguity pre-check here, matching the
// previous per-root behavior.
func (e *Executor) buildEntityWhereClause(q *Query, spec entitySQLSpec) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	if spec.typeColumn != "" {
		conditions = append(conditions, spec.typeColumn+" = ?")
		args = append(args, q.TypeName)
	} else {
		conditions = append(conditions, "1=1")
	}

	if q.Type == QueryTypeObject {
		if err := e.prepareRefFieldAmbiguityChecks(q); err != nil {
			return "", nil, err
		}
	}

	if q.Predicate != nil {
		typeName := ""
		if q.Type == QueryTypeObject {
			typeName = q.TypeName
		}
		cond, predArgs, err := e.buildPredicateSQL(q.Type, q.Predicate, spec.alias, typeName)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return strings.Join(conditions, " AND "), args, nil
}

func (e *Executor) buildEntityPageSQL(q *Query, spec entitySQLSpec, limit, offset int) (string, []interface{}, error) {
	where, args, err := e.buildEntityWhereClause(q, spec)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf("SELECT %s\nFROM %s %s\nWHERE %s\nORDER BY %s",
		spec.rowColumns, spec.table, spec.alias, where, spec.orderBy)
	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildEntityIDSQL(q *Query, spec entitySQLSpec, limit, offset int) (string, []interface{}, error) {
	where, args, err := e.buildEntityWhereClause(q, spec)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf("SELECT %s\nFROM %s %s\nWHERE %s\nORDER BY %s",
		spec.idColumn, spec.table, spec.alias, where, spec.orderBy)
	sqlStr, args = appendLimitOffset(sqlStr, args, limit, offset)
	return sqlStr, args, nil
}

func (e *Executor) buildEntityCountSQL(q *Query, spec entitySQLSpec) (string, []interface{}, error) {
	where, args, err := e.buildEntityWhereClause(q, spec)
	if err != nil {
		return "", nil, err
	}
	sqlStr := fmt.Sprintf("SELECT COUNT(*)\nFROM %s %s\nWHERE %s", spec.table, spec.alias, where)
	return sqlStr, args, nil
}

// queryEntityRows runs a row-returning query and scans it with the supplied
// entity scanner. It is the single query+scan implementation shared by all
// roots and by both the typed executor methods and Run.
func queryEntityRows[T any](e *Executor, sqlStr string, args []interface{}, scan func(*sql.Rows) ([]T, error)) ([]T, error) {
	rows, err := e.db.Query(sqlStr, args...)
	if err != nil {
		return nil, wrapQueryExecError(err)
	}
	return scan(rows)
}

func runEntityPageRows[T any](e *Executor, q *Query, spec entitySQLSpec, limit, offset int, scan func(*sql.Rows) ([]T, error)) ([]T, error) {
	if q.Type != spec.queryType {
		return nil, errUnexpectedQueryType(spec.queryType, q.Type)
	}
	sqlStr, args, err := e.buildEntityPageSQL(q, spec, limit, offset)
	if err != nil {
		return nil, err
	}
	return queryEntityRows(e, sqlStr, args, scan)
}

func runEntityIDs(e *Executor, q *Query, spec entitySQLSpec, limit, offset int) ([]string, error) {
	if q.Type != spec.queryType {
		return nil, errUnexpectedQueryType(spec.queryType, q.Type)
	}
	sqlStr, args, err := e.buildEntityIDSQL(q, spec, limit, offset)
	if err != nil {
		return nil, err
	}
	return queryEntityRows(e, sqlStr, args, scanIDRows)
}

func runEntityCount(e *Executor, q *Query, spec entitySQLSpec) (int, error) {
	if q.Type != spec.queryType {
		return 0, errUnexpectedQueryType(spec.queryType, q.Type)
	}
	sqlStr, args, err := e.buildEntityCountSQL(q, spec)
	if err != nil {
		return 0, err
	}
	count, err := e.executeCountQuery(sqlStr, args)
	if err != nil {
		return 0, wrapQueryExecError(err)
	}
	return count, nil
}
