package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/model"
)

// QueryTraits queries traits by type with optional value filter.
// Filter syntax supports:
//   - Simple value: "done" → value = 'done'
//   - OR with pipe: "today|tomorrow" → value matches either
//   - NOT with bang: "!done" → value != 'done'
//   - Combined: "!done|!cancelled" → value not in (done, cancelled)
//   - Date filters: "today", "tomorrow", "yesterday", YYYY-MM-DD (also work with | and !)
func (d *Database) QueryTraits(traitType string, valueFilter *string) ([]model.Trait, error) {
	query := `
		SELECT id, trait_type, value, content, file_path, line_number, parent_object_id
		FROM traits
		WHERE trait_type = ?
	`
	args := []interface{}{traitType}

	if valueFilter != nil && *valueFilter != "" {
		condition, filterArgs, err := parseFilterExpressionWithOptions(*valueFilter, "value", DateFilterOptions{
			Now: time.Now(),
		})
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

	var results []model.Trait
	for rows.Next() {
		var result model.Trait
		if err := rows.Scan(&result.ID, &result.TraitType, &result.Value, &result.Content, &result.FilePath, &result.Line, &result.ParentObjectID); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// GetSection returns a heading-derived section by ID.
func (d *Database) GetSection(id string) (*model.Section, error) {
	var section model.Section
	err := d.db.QueryRow(`
		SELECT id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id
		FROM sections
		WHERE id = ?
	`, id).Scan(
		&section.ID,
		&section.FileObjectID,
		&section.FilePath,
		&section.Slug,
		&section.Title,
		&section.Level,
		&section.LineStart,
		&section.LineEnd,
		&section.SubtreeLineEnd,
		&section.ParentSectionID,
	)
	if err == nil {
		return &section, nil
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return nil, err
}

func parseFilterExpressionWithOptions(filter string, fieldExpr string, opts DateFilterOptions) (condition string, args []interface{}, err error) {
	opts = normalizeDateFilterOptions(opts)

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
		partCondition, partArgs, buildErr := buildSingleFilterConditionWithOptions(part, fieldExpr, isNegated, opts)
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

func buildSingleFilterConditionWithOptions(value string, fieldExpr string, isNegated bool, opts DateFilterOptions) (condition string, args []interface{}, err error) {
	// Check if it's a date filter
	if isDateFilter(value) {
		dateCondition, dateArgs, parseErr := ParseDateFilterWithOptions(value, fieldExpr, opts)
		if parseErr != nil {
			return "", nil, parseErr
		}

		if isNegated {
			// Negate the date condition.
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
	trimmed := strings.TrimSpace(filter)
	if dates.IsValidDate(trimmed) {
		return true
	}
	if dates.IsRelativeDateKeyword(trimmed) {
		return true
	}
	return looksLikeDateLiteral(trimmed)
}

// QueryObjects queries objects by type.
func (d *Database) QueryObjects(objectType string) ([]model.Object, error) {
	rows, err := d.db.Query(
		"SELECT id, type, fields, file_path, line_start FROM objects WHERE type = ?",
		objectType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Object
	for rows.Next() {
		var result model.Object
		var fieldsJSON string
		if err := rows.Scan(&result.ID, &result.Type, &fieldsJSON, &result.FilePath, &result.LineStart); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(fieldsJSON), &result.Fields); err != nil || result.Fields == nil {
			result.Fields = make(map[string]interface{})
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// AllObjects returns all indexed file-backed objects.
func (d *Database) AllObjects() ([]model.Object, error) {
	rows, err := d.db.Query(`
		SELECT id, type, fields, file_path, line_start
		FROM objects
		ORDER BY file_path, line_start, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Object
	for rows.Next() {
		var result model.Object
		var fieldsJSON string
		if err := rows.Scan(&result.ID, &result.Type, &fieldsJSON, &result.FilePath, &result.LineStart); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(fieldsJSON), &result.Fields); err != nil || result.Fields == nil {
			result.Fields = make(map[string]interface{})
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// AllSections returns all indexed heading-derived sections.
func (d *Database) AllSections() ([]model.Section, error) {
	rows, err := d.db.Query(`
		SELECT id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id
		FROM sections
		ORDER BY file_path, line_start, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Section
	for rows.Next() {
		var section model.Section
		if err := rows.Scan(
			&section.ID,
			&section.FileObjectID,
			&section.FilePath,
			&section.Slug,
			&section.Title,
			&section.Level,
			&section.LineStart,
			&section.LineEnd,
			&section.SubtreeLineEnd,
			&section.ParentSectionID,
		); err != nil {
			return nil, err
		}
		results = append(results, section)
	}
	return results, rows.Err()
}

// QueryAssets returns indexed asset resources.
func (d *Database) QueryAssets() ([]model.Asset, error) {
	query := `
		SELECT id, file_path, COALESCE(media_type, ''), COALESCE(extension, ''),
		       filename, size_bytes, COALESCE(file_mtime, 0), COALESCE(indexed_at, 0)
		FROM assets
		ORDER BY file_path
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Asset
	for rows.Next() {
		var result model.Asset
		if err := rows.Scan(
			&result.ID,
			&result.FilePath,
			&result.MediaType,
			&result.Extension,
			&result.Filename,
			&result.SizeBytes,
			&result.FileMtime,
			&result.IndexedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// Backlinks returns all objects that reference the given target.
func (d *Database) Backlinks(targetID string) ([]model.Reference, error) {
	return d.BacklinksWithRoots(targetID, "", "")
}

// Outlinks returns all references made by the given source object.
//
// Includes refs whose source_id is a section of the source (source_id LIKE '<source>#%').
func (d *Database) Outlinks(sourceID string) ([]model.Reference, error) {
	query := `
		SELECT r.source_id, o.type, r.target_raw, r.file_path, r.line_number, r.display_text
		FROM refs r
		LEFT JOIN objects o ON r.source_id = o.id
		WHERE r.source_id = ? OR r.source_id LIKE ?
		ORDER BY r.file_path, r.line_number, r.position_start
	`

	rows, err := d.db.Query(query, sourceID, sourceID+"#%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Reference
	for rows.Next() {
		var result model.Reference
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

// BacklinksWithRoots returns all objects that reference the given target,
// including refs that use directory-prefixed paths (e.g., [[objects/people/freya]]).
// This is important for move operations to find all variants of a reference.
func (d *Database) BacklinksWithRoots(targetID, objectRoot, pageRoot string) ([]model.Reference, error) {
	// Build list of target patterns to search for
	patterns := []string{targetID}

	// Add directory-prefixed variants
	if objectRoot != "" {
		patterns = append(patterns, objectRoot+targetID)
	}
	if pageRoot != "" && pageRoot != objectRoot {
		patterns = append(patterns, pageRoot+targetID)
	}

	// Build query with all patterns
	var conditions []string
	var args []interface{}

	for _, pattern := range patterns {
		conditions = append(conditions,
			"r.target_raw = ?",
			"r.target_raw LIKE ?",
			"r.target_id = ?",
			"r.target_id LIKE ?",
		)
		args = append(args, pattern, pattern+"#%", pattern, pattern+"#%")
	}

	query := `
		SELECT r.source_id, o.type, r.target_raw, r.file_path, r.line_number, r.display_text
		FROM refs r
		LEFT JOIN objects o ON r.source_id = o.id
		WHERE ` + strings.Join(conditions, " OR ")

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Reference
	for rows.Next() {
		var result model.Reference
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
func (d *Database) GetObject(id string) (*model.Object, error) {
	var result model.Object
	var fieldsJSON string
	err := d.db.QueryRow(
		"SELECT id, type, fields, file_path, line_start FROM objects WHERE id = ?",
		id,
	).Scan(&result.ID, &result.Type, &fieldsJSON, &result.FilePath, &result.LineStart)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(fieldsJSON), &result.Fields); err != nil || result.Fields == nil {
		result.Fields = make(map[string]interface{})
	}

	return &result, nil
}

// GetTrait retrieves a single trait by ID.
func (d *Database) GetTrait(id string) (*model.Trait, error) {
	var result model.Trait
	err := d.db.QueryRow(
		"SELECT id, trait_type, value, content, file_path, line_number, parent_object_id FROM traits WHERE id = ?",
		id,
	).Scan(&result.ID, &result.TraitType, &result.Value, &result.Content, &result.FilePath, &result.Line, &result.ParentObjectID)

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
		"SELECT DISTINCT file_path FROM objects WHERE type = 'page' ORDER BY file_path",
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

// Search performs a full-text search across all content in the vault.
// The query supports FTS5 query syntax:
//   - Simple words: "meeting notes"
//   - Phrases: '"team meeting"'
//   - Boolean: "meeting AND notes", "meeting OR notes", "meeting NOT private"
//   - Prefix: "meet*"
//
// Results are ranked by relevance (best matches first).
func (d *Database) Search(query string, limit int) ([]model.SearchMatch, error) {
	if limit <= 0 {
		limit = 20
	}

	ftsQuery := BuildFTSSearchQuery(query)

	// Use FTS5 match query with BM25 ranking
	// Search both title and content columns
	// The snippet function extracts matching content with context
	rows, err := d.db.Query(`
		SELECT 
			f.object_id,
			f.title,
			f.file_path,
			CASE WHEN s.id IS NULL THEN 0 ELSE 1 END AS is_section,
			s.file_object_id,
			s.line_start,
			s.line_end,
			s.subtree_line_end,
			snippet(fts_content, 2, '»', '«', '...', 32) as snippet,
			bm25(fts_content) as rank
		FROM fts_content f
		LEFT JOIN sections s ON f.object_id = s.id
		WHERE fts_content MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	var results []model.SearchMatch
	for rows.Next() {
		var result model.SearchMatch
		var isSection int
		var fileObjectID sql.NullString
		var lineStart sql.NullInt64
		var lineEnd sql.NullInt64
		var subtreeLineEnd sql.NullInt64
		if err := rows.Scan(
			&result.ObjectID,
			&result.Title,
			&result.FilePath,
			&isSection,
			&fileObjectID,
			&lineStart,
			&lineEnd,
			&subtreeLineEnd,
			&result.Snippet,
			&result.Rank,
		); err != nil {
			return nil, err
		}
		result.IsSection = isSection != 0
		if fileObjectID.Valid {
			result.FileObjectID = fileObjectID.String
		}
		if lineStart.Valid {
			result.LineStart = int(lineStart.Int64)
		}
		if lineEnd.Valid {
			v := int(lineEnd.Int64)
			result.LineEnd = &v
		}
		if subtreeLineEnd.Valid {
			v := int(subtreeLineEnd.Int64)
			result.SubtreeLineEnd = &v
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// SearchWithType performs a full-text search filtered by object type.
func (d *Database) SearchWithType(query string, objectType string, limit int) ([]model.SearchMatch, error) {
	if limit <= 0 {
		limit = 20
	}

	ftsQuery := BuildFTSSearchQuery(query)

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
	`, ftsQuery, objectType, limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	var results []model.SearchMatch
	for rows.Next() {
		var result model.SearchMatch
		if err := rows.Scan(&result.ObjectID, &result.Title, &result.FilePath, &result.Snippet, &result.Rank); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}
