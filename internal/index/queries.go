package index

import (
	"database/sql"
	"fmt"
	"strings"
)

// TaskResult represents a task query result.
type TaskResult struct {
	ID       string
	Content  string
	Fields   string // JSON
	Line     int
	FilePath string
	ParentID string
}

// QueryTasks queries tasks with optional filters.
func (d *Database) QueryTasks(statusFilter, dueFilter *string, includeDone bool) ([]TaskResult, error) {
	query := `
		SELECT t.id, t.content, t.fields, t.line_number, t.file_path, t.parent_object_id
		FROM traits t
		WHERE t.trait_type = 'task'
	`

	var conditions []string
	var args []interface{}

	if !includeDone {
		conditions = append(conditions, "(json_extract(t.fields, '$.status') IS NULL OR json_extract(t.fields, '$.status') != 'done')")
	}

	if statusFilter != nil && *statusFilter != "" {
		conditions = append(conditions, "json_extract(t.fields, '$.status') = ?")
		args = append(args, *statusFilter)
	}

	// TODO: Add due date filtering with date comparison

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY json_extract(t.fields, '$.due') ASC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []TaskResult
	for rows.Next() {
		var task TaskResult
		if err := rows.Scan(&task.ID, &task.Content, &task.Fields, &task.Line, &task.FilePath, &task.ParentID); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

// BacklinkResult represents a backlink query result.
type BacklinkResult struct {
	SourceID    string
	FilePath    string
	Line        *int
	DisplayText *string
}

// Backlinks returns backlinks to a target.
func (d *Database) Backlinks(target string) ([]BacklinkResult, error) {
	// Match both exact and partial paths
	partial := "%/" + target

	rows, err := d.db.Query(`
		SELECT r.source_id, r.file_path, r.line_number, r.display_text
		FROM refs r
		WHERE r.target_raw = ? OR r.target_raw LIKE ?
	`, target, partial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BacklinkResult
	for rows.Next() {
		var result BacklinkResult
		if err := rows.Scan(&result.SourceID, &result.FilePath, &result.Line, &result.DisplayText); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// UntypedPages returns pages with type='page' (the fallback type).
func (d *Database) UntypedPages() ([]string, error) {
	rows, err := d.db.Query("SELECT id FROM objects WHERE type = 'page' AND parent_id IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		pages = append(pages, id)
	}

	return pages, rows.Err()
}

// QueryBuilder helps build complex queries.
type QueryBuilder struct {
	ObjectType   *string
	TraitType    *string
	FieldFilters map[string]string
	ParentType   *string
	Tag          *string
}

// NewQueryBuilder creates a new QueryBuilder.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		FieldFilters: make(map[string]string),
	}
}

// Parse parses a query string like "type:meeting attendees:[[alice]]".
func (qb *QueryBuilder) Parse(query string) *QueryBuilder {
	for _, part := range strings.Fields(query) {
		if idx := strings.Index(part, ":"); idx > 0 {
			key := part[:idx]
			value := part[idx+1:]

			switch key {
			case "type":
				qb.ObjectType = &value
			case "trait":
				qb.TraitType = &value
			case "tags":
				qb.Tag = &value
			case "parent.type":
				qb.ParentType = &value
			default:
				qb.FieldFilters[key] = value
			}
		}
	}

	return qb
}

// BuildWhereClause builds a SQL WHERE clause.
func (qb *QueryBuilder) BuildWhereClause() (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if qb.ObjectType != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *qb.ObjectType)
	}

	if qb.Tag != nil {
		conditions = append(conditions, "json_extract(fields, '$.tags') LIKE ?")
		args = append(args, fmt.Sprintf("%%\"%s%%", *qb.Tag))
	}

	for field, value := range qb.FieldFilters {
		conditions = append(conditions, fmt.Sprintf("json_extract(fields, '$.%s') = ?", field))
		args = append(args, value)
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

// QueryObjects queries objects with the current filters.
func (d *Database) QueryObjects(qb *QueryBuilder) ([]ObjectResult, error) {
	whereClause, args := qb.BuildWhereClause()

	query := "SELECT id, type, fields, file_path, line_start FROM objects"
	if whereClause != "" {
		query += " " + whereClause
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ObjectResult
	for rows.Next() {
		var result ObjectResult
		if err := rows.Scan(&result.ID, &result.Type, &result.Fields, &result.FilePath, &result.LineStart); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// ObjectResult represents an object query result.
type ObjectResult struct {
	ID        string
	Type      string
	Fields    string // JSON
	FilePath  string
	LineStart int
}

// QueryTraits queries traits with the given filters.
func (d *Database) QueryTraits(traitType string, fieldFilters map[string]string) ([]TraitResult, error) {
	query := "SELECT id, trait_type, content, fields, file_path, line_number, parent_object_id FROM traits WHERE trait_type = ?"
	args := []interface{}{traitType}

	for field, value := range fieldFilters {
		query += fmt.Sprintf(" AND json_extract(fields, '$.%s') = ?", field)
		args = append(args, value)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TraitResult
	for rows.Next() {
		var result TraitResult
		if err := rows.Scan(&result.ID, &result.TraitType, &result.Content, &result.Fields, &result.FilePath, &result.Line, &result.ParentID); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// TraitResult represents a trait query result.
type TraitResult struct {
	ID        string
	TraitType string
	Content   string
	Fields    string // JSON
	FilePath  string
	Line      int
	ParentID  string
}

// QueryTraitsByType queries traits of a given type with optional status and due filters.
// statusFilter can be a single status or comma-separated list (e.g., "todo,in_progress").
func (d *Database) QueryTraitsByType(traitType string, statusFilter, dueFilter *string) ([]TraitResult, error) {
	query := `
		SELECT id, trait_type, content, fields, file_path, line_number, parent_object_id
		FROM traits
		WHERE trait_type = ?
	`
	args := []interface{}{traitType}

	if statusFilter != nil && *statusFilter != "" {
		statuses := strings.Split(*statusFilter, ",")
		if len(statuses) == 1 {
			query += " AND json_extract(fields, '$.status') = ?"
			args = append(args, statuses[0])
		} else {
			placeholders := make([]string, len(statuses))
			for i, s := range statuses {
				placeholders[i] = "?"
				args = append(args, strings.TrimSpace(s))
			}
			query += " AND json_extract(fields, '$.status') IN (" + strings.Join(placeholders, ", ") + ")"
		}
	}

	if dueFilter != nil && *dueFilter != "" {
		// TODO: Handle relative dates like "today", "this-week"
		query += " AND json_extract(fields, '$.due') = ?"
		args = append(args, *dueFilter)
	}

	query += " ORDER BY json_extract(fields, '$.due') ASC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TraitResult
	for rows.Next() {
		var result TraitResult
		if err := rows.Scan(&result.ID, &result.TraitType, &result.Content, &result.Fields, &result.FilePath, &result.Line, &result.ParentID); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// GetObject retrieves a single object by ID.
func (d *Database) GetObject(id string) (*ObjectResult, error) {
	var result ObjectResult
	err := d.db.QueryRow(
		"SELECT id, type, fields, file_path, line_start FROM objects WHERE id = ?",
		id,
	).Scan(&result.ID, &result.Type, &result.Fields, &result.FilePath, &result.LineStart)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}
