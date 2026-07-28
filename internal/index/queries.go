package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
)

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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return nil, err
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
		fields, err := fieldvalue.FieldsFromJSON([]byte(fieldsJSON))
		if err != nil || fields == nil {
			fields = make(map[string]fieldvalue.FieldValue)
		}
		result.Fields = fields
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

// FileLinks returns all indexed Markdown links and images whose target is a
// local file. Existence is intentionally not part of the index and must be
// checked by callers against the current filesystem.
func (d *Database) FileLinks() ([]model.Link, error) {
	return d.queryLinks(`
		SELECT source_id, source_type, file_path, line_number, position_start, position_end,
		       raw_target, display, is_image, scheme, ext, normalized_key
		FROM links
		WHERE scheme = 'file'
		ORDER BY file_path, line_number, position_start, id
	`)
}

// FileLinksByNormalizedKey returns inbound file links for one canonical target
// identity. This is the matching primitive used by move rewrites.
func (d *Database) FileLinksByNormalizedKey(normalizedKey string) ([]model.Link, error) {
	return d.queryLinks(`
		SELECT source_id, source_type, file_path, line_number, position_start, position_end,
		       raw_target, display, is_image, scheme, ext, normalized_key
		FROM links
		WHERE scheme = 'file' AND normalized_key = ?
		ORDER BY file_path, line_number, position_start, id
	`, normalizedKey)
}

func (d *Database) queryLinks(query string, args ...interface{}) ([]model.Link, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Link
	for rows.Next() {
		var result model.Link
		if err := rows.Scan(
			&result.SourceID,
			&result.SourceType,
			&result.FilePath,
			&result.Line,
			&result.PositionStart,
			&result.PositionEnd,
			&result.RawTarget,
			&result.Display,
			&result.IsImage,
			&result.Scheme,
			&result.Ext,
			&result.NormalizedKey,
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
		SELECT r.source_id, COALESCE(o.type, CASE WHEN s.id IS NOT NULL THEN 'section' END), r.target_raw, r.file_path, r.line_number, r.display_text
		FROM refs r
		LEFT JOIN objects o ON r.source_id = o.id
		LEFT JOIN sections s ON r.source_id = s.id
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
		SELECT r.source_id, COALESCE(o.type, CASE WHEN s.id IS NOT NULL THEN 'section' END), r.target_raw, r.file_path, r.line_number, r.position_start, r.position_end, r.display_text
		FROM refs r
		LEFT JOIN objects o ON r.source_id = o.id
		LEFT JOIN sections s ON r.source_id = s.id
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
		if err := rows.Scan(&result.SourceID, &sourceType, &result.TargetRaw, &result.FilePath, &result.Line, &result.PositionStart, &result.PositionEnd, &result.DisplayText); err != nil {
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

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	fields, err := fieldvalue.FieldsFromJSON([]byte(fieldsJSON))
	if err != nil || fields == nil {
		fields = make(map[string]fieldvalue.FieldValue)
	}
	result.Fields = fields

	return &result, nil
}

// GetTrait retrieves a single trait by ID.
func (d *Database) GetTrait(id string) (*model.Trait, error) {
	var result model.Trait
	var value sql.NullString
	err := d.db.QueryRow(
		"SELECT id, trait_type, value, content, file_path, line_number, parent_object_id FROM traits WHERE id = ?",
		id,
	).Scan(&result.ID, &result.TraitType, &value, &result.Content, &result.FilePath, &result.Line, &result.ParentScopeID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if value.Valid {
		s := value.String
		result.SetIndexValueString(&s)
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
