package index

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/sqlutil"
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
	TargetRaw   string
	FilePath    string
	Line        *int
	DisplayText *string
}

// QueryTraits queries traits by type with optional value filter.
// Filter syntax supports:
//   - Simple value: "done" → value = 'done'
//   - OR with pipe: "this-week|past" → value matches either
//   - NOT with bang: "!done" → value != 'done'
//   - Combined: "!done|!cancelled" → value not in (done, cancelled)
//   - Date filters: "today", "this-week", "past", etc. (also work with | and !)
func (d *Database) QueryTraits(traitType string, valueFilter *string) ([]TraitResult, error) {
	query := `
		SELECT id, trait_type, value, content, file_path, line_number, parent_object_id
		FROM traits
		WHERE trait_type = ?
	`
	args := []interface{}{traitType}

	if valueFilter != nil && *valueFilter != "" {
		condition, filterArgs, err := parseFilterExpression(*valueFilter, "value")
		if err != nil {
			return nil, err
		}
		query += " AND " + condition
		args = append(args, filterArgs...)
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

// parseFilterExpression parses a filter expression with support for:
//   - OR using pipe: "a|b" → (expr_a OR expr_b)
//   - NOT using bang: "!a" → NOT expr_a
//   - Date filters are automatically detected and expanded
//
// Returns SQL condition and args.
func parseFilterExpression(filter string, fieldExpr string) (condition string, args []interface{}, err error) {
	// Split on | for OR logic
	parts := strings.Split(filter, "|")

	type filterPart struct {
		cond    string
		args    []interface{}
		negated bool
	}
	var parsed []filterPart
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for NOT prefix
		isNegated := false
		if strings.HasPrefix(part, "!") {
			isNegated = true
			part = strings.TrimPrefix(part, "!")
		}

		// Build condition for this part
		partCondition, partArgs, buildErr := buildSingleFilterCondition(part, fieldExpr, isNegated)
		if buildErr != nil {
			return "", nil, buildErr
		}

		parsed = append(parsed, filterPart{
			cond:    partCondition,
			args:    partArgs,
			negated: isNegated,
		})
	}

	if len(parsed) == 0 {
		return "1=1", nil, nil // No filter, match all
	}

	if len(parsed) == 1 {
		return parsed[0].cond, parsed[0].args, nil
	}

	allNegated := true
	for _, p := range parsed {
		if !p.negated {
			allNegated = false
			break
		}
	}

	// Multiple conditions → OR them together, unless all are negated.
	// For negated filters, "!a|!b" means "not a AND not b" (not-in semantics).
	joiner := " OR "
	if allNegated {
		joiner = " AND "
	}

	var conditions []string
	for _, p := range parsed {
		conditions = append(conditions, p.cond)
		args = append(args, p.args...)
	}

	return "(" + strings.Join(conditions, joiner) + ")", args, nil
}

// buildSingleFilterCondition builds a SQL condition for a single filter value.
func buildSingleFilterCondition(value string, fieldExpr string, isNegated bool) (condition string, args []interface{}, err error) {
	// Check if it's a date filter
	if isDateFilter(value) {
		dateCondition, dateArgs, parseErr := ParseDateFilter(value, fieldExpr)
		if parseErr != nil {
			return "", nil, parseErr
		}

		if isNegated {
			// Negate the date condition
			// For simple comparisons, just flip the operator
			// For range conditions (this-week), wrap in NOT(...)
			return "NOT (" + dateCondition + ")", dateArgs, nil
		}
		return dateCondition, dateArgs, nil
	}

	// Simple value match
	if isNegated {
		return fieldExpr + " != ?", []interface{}{value}, nil
	}
	return fieldExpr + " = ?", []interface{}{value}, nil
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

	inClause, args := sqlutil.InClauseArgs(traitTypes)

	query := fmt.Sprintf(`
		SELECT id, trait_type, value, content, file_path, line_number, parent_object_id
		FROM traits
		WHERE trait_type IN (%s)
	`, inClause)

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
		SELECT r.source_id, o.type, r.target_raw, r.file_path, r.line_number, r.display_text
		FROM refs r
		LEFT JOIN objects o ON r.source_id = o.id
		WHERE r.target_raw = ?
		   OR r.target_raw LIKE ?
		   OR r.target_id = ?
		   OR r.target_id LIKE ?
	`

	rows, err := d.db.Query(query, targetID, targetID+"#%", targetID, targetID+"#%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BacklinkResult
	for rows.Next() {
		var result BacklinkResult
		var sourceType sql.NullString
		if err := rows.Scan(&result.SourceID, &sourceType, &result.TargetRaw, &result.FilePath, &result.Line, &result.DisplayText); err != nil {
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

// SearchResult represents a full-text search result.
type SearchResult struct {
	ObjectID string
	Title    string
	FilePath string
	Snippet  string  // Matched snippet with context
	Rank     float64 // FTS5 ranking score (lower is better match)
}

// Search performs a full-text search across all content in the vault.
// The query supports FTS5 query syntax:
//   - Simple words: "meeting notes"
//   - Phrases: '"team meeting"'
//   - Boolean: "meeting AND notes", "meeting OR notes", "meeting NOT private"
//   - Prefix: "meet*"
//
// Results are ranked by relevance (best matches first).
func (d *Database) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	// Use FTS5 match query with BM25 ranking
	// The snippet function extracts matching content with context
	rows, err := d.db.Query(`
		SELECT 
			object_id,
			title,
			file_path,
			snippet(fts_content, 2, '»', '«', '...', 32) as snippet,
			bm25(fts_content) as rank
		FROM fts_content
		WHERE fts_content MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.ObjectID, &result.Title, &result.FilePath, &result.Snippet, &result.Rank); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// SearchWithType performs a full-text search filtered by object type.
func (d *Database) SearchWithType(query string, objectType string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := d.db.Query(`
		SELECT 
			f.object_id,
			f.title,
			f.file_path,
			snippet(fts_content, 2, '»', '«', '...', 32) as snippet,
			bm25(fts_content) as rank
		FROM fts_content f
		JOIN objects o ON f.object_id = o.id
		WHERE fts_content MATCH ? AND o.type = ?
		ORDER BY rank
		LIMIT ?
	`, query, objectType, limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.ObjectID, &result.Title, &result.FilePath, &result.Snippet, &result.Rank); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}
