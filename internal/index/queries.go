package index

import (
	"database/sql"
	"fmt"
	"strings"
)

// TraitResult represents a trait query result.
type TraitResult struct {
	ID        string
	TraitType string
	Value     *string // Single value (NULL for boolean traits)
	Content   string
	FilePath  string
	Line      int
	ParentID  string
}

// ObjectResult represents an object query result.
type ObjectResult struct {
	ID        string
	Type      string
	Fields    string // JSON
	FilePath  string
	LineStart int
}

// BacklinkResult represents a backlink query result.
type BacklinkResult struct {
	SourceID    string
	SourceType  string
	FilePath    string
	Line        *int
	DisplayText *string
}

// QueryTraits queries traits by type with optional value filter.
func (d *Database) QueryTraits(traitType string, valueFilter *string) ([]TraitResult, error) {
	query := `
		SELECT id, trait_type, value, content, file_path, line_number, parent_object_id
		FROM traits
		WHERE trait_type = ?
	`
	args := []interface{}{traitType}

	if valueFilter != nil && *valueFilter != "" {
		// Support relative dates for date-typed traits
		if isDateFilter(*valueFilter) {
			condition, dateArgs, _ := ParseDateFilter(*valueFilter, "value")
			query += " AND " + condition
			args = append(args, dateArgs...)
		} else {
			// Simple value match
			query += " AND value = ?"
			args = append(args, *valueFilter)
		}
	}

	query += " ORDER BY value ASC NULLS LAST"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TraitResult
	for rows.Next() {
		var result TraitResult
		if err := rows.Scan(&result.ID, &result.TraitType, &result.Value, &result.Content, &result.FilePath, &result.Line, &result.ParentID); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// isDateFilter checks if a filter string is a date filter.
func isDateFilter(filter string) bool {
	lower := strings.ToLower(filter)
	switch lower {
	case "today", "yesterday", "tomorrow", "this-week", "next-week", "past", "future":
		return true
	}
	// Check for YYYY-MM-DD pattern
	if len(filter) == 10 && filter[4] == '-' && filter[7] == '-' {
		return true
	}
	return false
}

// QueryTraitsMultiple queries multiple trait types at once.
// Useful for compound queries like "items with @due AND @status".
func (d *Database) QueryTraitsMultiple(traitTypes []string) (map[string][]TraitResult, error) {
	if len(traitTypes) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(traitTypes))
	args := make([]interface{}, len(traitTypes))
	for i, t := range traitTypes {
		placeholders[i] = "?"
		args[i] = t
	}

	query := fmt.Sprintf(`
		SELECT id, trait_type, value, content, file_path, line_number, parent_object_id
		FROM traits
		WHERE trait_type IN (%s)
	`, strings.Join(placeholders, ", "))

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string][]TraitResult)
	for rows.Next() {
		var result TraitResult
		if err := rows.Scan(&result.ID, &result.TraitType, &result.Value, &result.Content, &result.FilePath, &result.Line, &result.ParentID); err != nil {
			return nil, err
		}
		results[result.TraitType] = append(results[result.TraitType], result)
	}

	return results, rows.Err()
}

// QueryTraitsOnContent finds all traits on the same content (by file and line).
func (d *Database) QueryTraitsOnContent(filePath string, line int) ([]TraitResult, error) {
	query := `
		SELECT id, trait_type, value, content, file_path, line_number, parent_object_id
		FROM traits
		WHERE file_path = ? AND line_number = ?
	`

	rows, err := d.db.Query(query, filePath, line)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TraitResult
	for rows.Next() {
		var result TraitResult
		if err := rows.Scan(&result.ID, &result.TraitType, &result.Value, &result.Content, &result.FilePath, &result.Line, &result.ParentID); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// QueryObjects queries objects by type.
func (d *Database) QueryObjects(objectType string) ([]ObjectResult, error) {
	rows, err := d.db.Query(
		"SELECT id, type, fields, file_path, line_start FROM objects WHERE type = ?",
		objectType,
	)
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

// Backlinks returns all objects that reference the given target.
func (d *Database) Backlinks(targetID string) ([]BacklinkResult, error) {
	// Support both exact match and date shorthand
	query := `
		SELECT r.source_id, o.type, r.file_path, r.line_number, r.display_text
		FROM refs r
		LEFT JOIN objects o ON r.source_id = o.id
		WHERE r.target_raw = ? OR r.target_id = ?
	`

	rows, err := d.db.Query(query, targetID, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BacklinkResult
	for rows.Next() {
		var result BacklinkResult
		var sourceType sql.NullString
		if err := rows.Scan(&result.SourceID, &sourceType, &result.FilePath, &result.Line, &result.DisplayText); err != nil {
			return nil, err
		}
		if sourceType.Valid {
			result.SourceType = sourceType.String
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

// GetTrait retrieves a single trait by ID.
func (d *Database) GetTrait(id string) (*TraitResult, error) {
	var result TraitResult
	err := d.db.QueryRow(
		"SELECT id, trait_type, value, content, file_path, line_number, parent_object_id FROM traits WHERE id = ?",
		id,
	).Scan(&result.ID, &result.TraitType, &result.Value, &result.Content, &result.FilePath, &result.Line, &result.ParentID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DateIndexResult represents a result from the date index.
type DateIndexResult struct {
	Date       string
	SourceType string // "object" or "trait"
	SourceID   string
	FieldName  string
	FilePath   string
}

// QueryDateIndex returns all objects/traits associated with a specific date.
func (d *Database) QueryDateIndex(date string) ([]DateIndexResult, error) {
	rows, err := d.db.Query(
		"SELECT date, source_type, source_id, field_name, file_path FROM date_index WHERE date = ?",
		date,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DateIndexResult
	for rows.Next() {
		var result DateIndexResult
		if err := rows.Scan(&result.Date, &result.SourceType, &result.SourceID, &result.FieldName, &result.FilePath); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// UntypedPages returns file paths of all objects using the fallback 'page' type.
func (d *Database) UntypedPages() ([]string, error) {
	rows, err := d.db.Query(
		"SELECT DISTINCT file_path FROM objects WHERE type = 'page' AND parent_id IS NULL ORDER BY file_path",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		results = append(results, path)
	}

	return results, rows.Err()
}
