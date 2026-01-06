// Package index handles SQLite database operations.
package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// Database is the SQLite database handle.
type Database struct {
	db *sql.DB
}

// DB returns the underlying sql.DB for advanced queries.
func (d *Database) DB() *sql.DB {
	return d.db
}

// Open opens or creates the database.
func Open(vaultPath string) (*Database, error) {
	dbDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .raven directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	d := &Database{db: db}
	if err := d.initialize(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

// OpenWithRebuild opens the database, rebuilding if schema is incompatible.
// Returns (database, wasRebuilt, error).
func OpenWithRebuild(vaultPath string) (*Database, bool, error) {
	dbDir := filepath.Join(vaultPath, ".raven")
	dbPath := filepath.Join(dbDir, "index.db")

	// Try to open and check schema compatibility
	if _, err := os.Stat(dbPath); err == nil {
		db, err := sql.Open("sqlite", dbPath)
		if err == nil {
			if !isSchemaCompatible(db) {
				db.Close()
				// Schema incompatible - delete and recreate
				os.Remove(dbPath)
				os.Remove(dbPath + "-wal")
				os.Remove(dbPath + "-shm")
				// Open fresh
				freshDB, err := Open(vaultPath)
				return freshDB, true, err
			}
			db.Close()
		}
	}

	// Open normally
	db, err := Open(vaultPath)
	return db, false, err
}

// isSchemaCompatible checks if the database schema matches expected structure.
func isSchemaCompatible(db *sql.DB) bool {
	// Check if traits table has 'value' column (new schema)
	// Old schema had 'fields' column instead
	rows, err := db.Query("PRAGMA table_info(traits)")
	if err != nil {
		return false
	}
	defer rows.Close()

	hasValueColumn := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false
		}
		if name == "value" {
			hasValueColumn = true
			break
		}
	}

	if !hasValueColumn {
		return false
	}

	// Check if fts_content table exists (v4+)
	var ftsTableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='fts_content'").Scan(&ftsTableName)
	if err != nil {
		return false
	}

	// Check if objects table has 'indexed_at' column (v6+)
	// This replaced the old created_at/updated_at columns
	rows2, err := db.Query("PRAGMA table_info(objects)")
	if err != nil {
		return false
	}
	defer rows2.Close()

	hasIndexedAtColumn := false
	for rows2.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows2.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false
		}
		if name == "indexed_at" {
			hasIndexedAtColumn = true
			break
		}
	}

	return hasIndexedAtColumn
}

// OpenInMemory opens an in-memory database (for testing).
func OpenInMemory() (*Database, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	d := &Database{db: db}
	if err := d.initialize(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

// Close closes the database.
func (d *Database) Close() error {
	return d.db.Close()
}

// Analyze runs SQLite's ANALYZE command to update query planner statistics.
// This should be called after bulk indexing operations for optimal query performance.
func (d *Database) Analyze() error {
	_, err := d.db.Exec("ANALYZE")
	return err
}

// CurrentDBVersion is the current database schema version.
// v7: Added composite indexes for trait refs matching and performance PRAGMAs
const CurrentDBVersion = 7

// initialize creates the database schema.
func (d *Database) initialize() error {
	schema := `
		-- Enable WAL mode for better concurrency
		PRAGMA journal_mode = WAL;
		
		-- Performance optimizations
		PRAGMA synchronous = NORMAL;      -- Faster writes (safe with WAL)
		PRAGMA temp_store = MEMORY;       -- Keep temp tables in memory
		PRAGMA cache_size = -64000;       -- 64MB cache (negative = KB)
		PRAGMA mmap_size = 268435456;     -- 256MB memory-mapped I/O
		
		-- Metadata table for version tracking
		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		
		-- All referenceable objects (files + embedded types)
		CREATE TABLE IF NOT EXISTS objects (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			type TEXT NOT NULL,
			heading TEXT,
			heading_level INTEGER,
			fields TEXT NOT NULL DEFAULT '{}',
			line_start INTEGER NOT NULL,
			line_end INTEGER,
			parent_id TEXT,
			file_mtime INTEGER,         -- File modification time from filesystem (Unix timestamp)
			indexed_at INTEGER          -- When this row was written to the index
		);
		
		-- All trait annotations (single-valued)
		CREATE TABLE IF NOT EXISTS traits (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			parent_object_id TEXT NOT NULL,
			trait_type TEXT NOT NULL,
			value TEXT,                          -- Single trait value (NULL for boolean traits)
			content TEXT NOT NULL,
			line_number INTEGER NOT NULL,
			indexed_at INTEGER          -- When this row was written to the index
		);
		
		-- References between objects
		CREATE TABLE IF NOT EXISTS refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			target_id TEXT,
			target_raw TEXT NOT NULL,
			display_text TEXT,
			file_path TEXT NOT NULL,
			line_number INTEGER,
			position_start INTEGER,
			position_end INTEGER
		);
		
		-- Indexes for fast queries
		CREATE INDEX IF NOT EXISTS idx_objects_file ON objects(file_path);
		CREATE INDEX IF NOT EXISTS idx_objects_type ON objects(type);
		CREATE INDEX IF NOT EXISTS idx_objects_parent ON objects(parent_id);
		
		CREATE INDEX IF NOT EXISTS idx_traits_file ON traits(file_path);
		CREATE INDEX IF NOT EXISTS idx_traits_type ON traits(trait_type);
		CREATE INDEX IF NOT EXISTS idx_traits_parent ON traits(parent_object_id);
		
		CREATE INDEX IF NOT EXISTS idx_refs_source ON refs(source_id);
		CREATE INDEX IF NOT EXISTS idx_refs_target ON refs(target_id);
		CREATE INDEX IF NOT EXISTS idx_refs_file ON refs(file_path);
		
		-- Composite indexes for trait refs matching (content scope rule)
		CREATE INDEX IF NOT EXISTS idx_traits_file_line ON traits(file_path, line_number);
		CREATE INDEX IF NOT EXISTS idx_refs_file_line ON refs(file_path, line_number);
		
		-- Index for faster trait value queries
		CREATE INDEX IF NOT EXISTS idx_traits_type_value ON traits(trait_type, value);
		
		-- Date index for temporal queries
		-- Links dates to objects/traits that have date fields
		CREATE TABLE IF NOT EXISTS date_index (
			date TEXT NOT NULL,              -- YYYY-MM-DD
			source_type TEXT NOT NULL,       -- 'object' or 'trait'
			source_id TEXT NOT NULL,         -- Object or trait ID
			field_name TEXT NOT NULL,        -- Which field (due, date, start, etc.)
			file_path TEXT NOT NULL,
			PRIMARY KEY (date, source_type, source_id, field_name)
		);
		
		CREATE INDEX IF NOT EXISTS idx_date_index_date ON date_index(date);
		CREATE INDEX IF NOT EXISTS idx_date_index_file ON date_index(file_path);

		-- Full-text search index for content search
		CREATE VIRTUAL TABLE IF NOT EXISTS fts_content USING fts5(
			object_id,
			title,
			content,
			file_path UNINDEXED,
			tokenize='porter unicode61'
		);
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Set database version
	_, err = d.db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('version', ?)`,
		fmt.Sprintf("%d", CurrentDBVersion))
	if err != nil {
		return fmt.Errorf("failed to set database version: %w", err)
	}

	return nil
}

// IndexDocument indexes a parsed document (replaces existing data for the file).
// IndexDocument indexes a parsed document into the database.
// The schema is needed to validate field types and trait definitions.
// Deprecated: Use IndexDocumentWithMtime for proper staleness tracking.
func (d *Database) IndexDocument(doc *parser.ParsedDocument, sch *schema.Schema) error {
	return d.IndexDocumentWithMtime(doc, sch, 0)
}

// IndexDocumentWithMtime indexes a parsed document with file modification time tracking.
// fileMtime should be the file's modification time as Unix timestamp (seconds).
// Pass 0 if mtime is unknown (will use current time as fallback).
func (d *Database) IndexDocumentWithMtime(doc *parser.ParsedDocument, sch *schema.Schema, fileMtime int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing data for this file
	if _, err := tx.Exec("DELETE FROM objects WHERE file_path = ?", doc.FilePath); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM traits WHERE file_path = ?", doc.FilePath); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM refs WHERE file_path = ?", doc.FilePath); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM date_index WHERE file_path = ?", doc.FilePath); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM fts_content WHERE file_path = ?", doc.FilePath); err != nil {
		return err
	}

	now := time.Now().Unix()

	// Use provided mtime or fall back to current time
	mtime := fileMtime
	if mtime == 0 {
		mtime = now
	}

	// Insert objects
	objStmt, err := tx.Prepare(`
		INSERT INTO objects (id, file_path, type, heading, heading_level, fields, line_start, line_end, parent_id, file_mtime, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer objStmt.Close()

	for _, obj := range doc.Objects {
		fieldsJSON, err := json.Marshal(fieldsToMap(obj.Fields))
		if err != nil {
			return err
		}

		_, err = objStmt.Exec(
			obj.ID,
			doc.FilePath,
			obj.ObjectType,
			obj.Heading,
			obj.HeadingLevel,
			string(fieldsJSON),
			obj.LineStart,
			obj.LineEnd,
			obj.ParentID,
			mtime,
			now,
		)
		if err != nil {
			return err
		}
	}

	// Insert traits (inline only - traits in content, not frontmatter)
	traitStmt, err := tx.Prepare(`
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer traitStmt.Close()

	traitIdx := 0

	// Index inline traits (only if defined in schema)
	for _, trait := range doc.Traits {
		// Skip undefined traits - schema is source of truth
		if sch != nil {
			if _, defined := sch.Traits[trait.TraitType]; !defined {
				continue // Skip indexing undefined traits
			}
		}

		traitID := fmt.Sprintf("%s:trait:%d", doc.FilePath, traitIdx)
		traitIdx++

		// Get value as string, applying schema defaults for bare traits
		var valueStr interface{}
		if trait.Value != nil {
			if s, ok := trait.Value.AsString(); ok {
				valueStr = s
			}
		} else {
			// Bare trait with no value - check schema for default
			valueStr = getTraitDefault(sch, trait.TraitType)
		}

		_, execErr := traitStmt.Exec(
			traitID,
			doc.FilePath,
			trait.ParentObjectID,
			trait.TraitType,
			valueStr,
			trait.Content,
			trait.Line,
			now,
		)
		if execErr != nil {
			return execErr
		}
	}

	// Insert refs
	refStmt, err := tx.Prepare(`
		INSERT INTO refs (source_id, target_id, target_raw, display_text, file_path, line_number, position_start, position_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer refStmt.Close()

	for _, ref := range doc.Refs {
		_, err = refStmt.Exec(
			ref.SourceID,
			nil, // target_id resolved later
			ref.TargetRaw,
			ref.DisplayText,
			doc.FilePath,
			ref.Line,
			ref.Start,
			ref.End,
		)
		if err != nil {
			return err
		}
	}

	// Index dates from object fields
	dateStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO date_index (date, source_type, source_id, field_name, file_path)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer dateStmt.Close()

	for _, obj := range doc.Objects {
		for fieldName, fieldValue := range obj.Fields {
			if dateStr := extractDateString(fieldValue); dateStr != "" {
				_, err = dateStmt.Exec(dateStr, "object", obj.ID, fieldName, doc.FilePath)
				if err != nil {
					return err
				}
			}
		}
	}

	for idx, trait := range doc.Traits {
		// Skip undefined traits - schema is source of truth
		if sch != nil {
			if _, defined := sch.Traits[trait.TraitType]; !defined {
				continue
			}
		}

		traitID := fmt.Sprintf("%s:trait:%d", doc.FilePath, idx)
		// For single-value traits, check if the value is a date
		if trait.Value != nil {
			if dateStr := extractDateString(*trait.Value); dateStr != "" {
				_, err = dateStmt.Exec(dateStr, "trait", traitID, trait.TraitType, doc.FilePath)
				if err != nil {
					return err
				}
			}
		}
	}

	// Index content for full-text search
	ftsStmt, err := tx.Prepare(`
		INSERT INTO fts_content (object_id, title, content, file_path)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	for _, obj := range doc.Objects {
		// Get title from fields or heading
		title := ""
		if titleField, ok := obj.Fields["title"]; ok {
			if s, ok := titleField.AsString(); ok {
				title = s
			}
		} else if obj.Heading != nil {
			title = *obj.Heading
		} else {
			// Use object ID as title for file-level objects
			title = obj.ID
		}

		// Get content - for now we index the whole file content for file-level objects
		// For embedded objects, we'd need to extract their section content
		content := ""
		if obj.ParentID == nil {
			// File-level object - index full body
			content = doc.RawContent
		}

		_, err = ftsStmt.Exec(obj.ID, title, content, doc.FilePath)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// extractDateString extracts a date string from a field value if it's a date type.
// Only extracts absolute dates in YYYY-MM-DD format.
// Relative keywords (today, tomorrow, etc.) are NOT resolved here because the
// resolved value would become stale on reindex. Instead, relative dates are
// handled at query time.
// Returns empty string if not a date.
func extractDateString(fv schema.FieldValue) string {
	if s, ok := fv.AsString(); ok {
		// Check if it looks like a date (YYYY-MM-DD)
		if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
			return s[:10] // Return just the date part (in case of datetime)
		}
	}
	return ""
}

// RemoveFile removes all data for a file.
func (d *Database) RemoveFile(filePath string) error {
	if _, err := d.db.Exec("DELETE FROM objects WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM traits WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM refs WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM date_index WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM fts_content WHERE file_path = ?", filePath); err != nil {
		return err
	}
	return nil
}

// RemoveFilesWithPrefix removes all data for files whose paths start with a given prefix.
// This is used to clean up files in excluded directories like .trash/.
// Returns the number of files removed.
func (d *Database) RemoveFilesWithPrefix(pathPrefix string) (int, error) {
	// Count files that will be removed
	var count int
	err := d.db.QueryRow("SELECT COUNT(DISTINCT file_path) FROM objects WHERE file_path LIKE ?", pathPrefix+"%").Scan(&count)
	if err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, nil
	}

	pattern := pathPrefix + "%"
	if _, err := d.db.Exec("DELETE FROM objects WHERE file_path LIKE ?", pattern); err != nil {
		return 0, err
	}
	if _, err := d.db.Exec("DELETE FROM traits WHERE file_path LIKE ?", pattern); err != nil {
		return 0, err
	}
	if _, err := d.db.Exec("DELETE FROM refs WHERE file_path LIKE ?", pattern); err != nil {
		return 0, err
	}
	if _, err := d.db.Exec("DELETE FROM date_index WHERE file_path LIKE ?", pattern); err != nil {
		return 0, err
	}
	if _, err := d.db.Exec("DELETE FROM fts_content WHERE file_path LIKE ?", pattern); err != nil {
		return 0, err
	}
	return count, nil
}

// AllIndexedFilePaths returns all distinct file paths currently in the index.
// This is useful for detecting deleted files during incremental reindexing.
func (d *Database) AllIndexedFilePaths() ([]string, error) {
	rows, err := d.db.Query(`
		SELECT DISTINCT file_path FROM objects WHERE parent_id IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// RemoveDeletedFiles removes index entries for files that no longer exist on the filesystem.
// Returns the list of removed file paths.
func (d *Database) RemoveDeletedFiles(vaultPath string) ([]string, error) {
	indexedPaths, err := d.AllIndexedFilePaths()
	if err != nil {
		return nil, fmt.Errorf("failed to get indexed paths: %w", err)
	}

	var removed []string
	for _, relPath := range indexedPaths {
		fullPath := filepath.Join(vaultPath, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// File was deleted - remove from index
			if err := d.RemoveFile(relPath); err != nil {
				return removed, fmt.Errorf("failed to remove %s: %w", relPath, err)
			}
			removed = append(removed, relPath)
		}
	}

	return removed, nil
}

// RemoveDocument removes a document and all related data by its object ID.
func (d *Database) RemoveDocument(objectID string) error {
	// Objects can have IDs like "people/freya" or "daily/2025-02-01#meeting"
	// For the file-level ID, we need to delete by the root object or file path
	filePath := objectID + ".md"

	// Delete all objects whose ID starts with this objectID (handles sections/embedded)
	if _, err := d.db.Exec("DELETE FROM objects WHERE id = ? OR id LIKE ?", objectID, objectID+"#%"); err != nil {
		return err
	}

	// Delete related data by file path
	if _, err := d.db.Exec("DELETE FROM traits WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM refs WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM date_index WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM fts_content WHERE file_path = ?", filePath); err != nil {
		return err
	}
	return nil
}

// Stats returns statistics about the index.
func (d *Database) Stats() (*IndexStats, error) {
	var stats IndexStats

	if err := d.db.QueryRow("SELECT COUNT(*) FROM objects").Scan(&stats.ObjectCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow("SELECT COUNT(*) FROM traits").Scan(&stats.TraitCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow("SELECT COUNT(*) FROM refs").Scan(&stats.RefCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow("SELECT COUNT(DISTINCT file_path) FROM objects").Scan(&stats.FileCount); err != nil {
		return nil, err
	}

	return &stats, nil
}

// IndexStats contains index statistics.
type IndexStats struct {
	ObjectCount int
	TraitCount  int
	RefCount    int
	FileCount   int
}

// AllObjectIDs returns all object IDs (for reference resolution).
func (d *Database) AllObjectIDs() ([]string, error) {
	rows, err := d.db.Query("SELECT id FROM objects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// Helper to convert FieldValue map to interface map for JSON serialization.
func fieldsToMap(fields map[string]schema.FieldValue) map[string]interface{} {
	result := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		result[k] = v.Raw()
	}
	return result
}

// getTraitDefault returns the default value for a trait from the schema.
// For boolean traits with default: true, returns "true".
// For other traits, returns the default value as a string, or nil if no default.
func getTraitDefault(sch *schema.Schema, traitType string) interface{} {
	if sch == nil {
		return nil
	}

	traitDef, exists := sch.Traits[traitType]
	if !exists || traitDef == nil {
		return nil
	}

	// If no default is defined, return nil
	if traitDef.Default == nil {
		// For boolean traits without explicit default, the presence of the trait
		// implies "true" - this is the expected UX for bare boolean traits
		if traitDef.IsBoolean() {
			return "true"
		}
		return nil
	}

	// Convert default value to string for storage
	switch v := traitDef.Default.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int, int64, float64:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// StalenessInfo contains information about index freshness.
type StalenessInfo struct {
	IsStale      bool     // True if any files are stale
	StaleFiles   []string // List of stale file paths (relative to vault)
	TotalFiles   int      // Total number of indexed files
	CheckedFiles int      // Number of files checked
}

// CheckStaleness compares indexed file mtimes against current filesystem mtimes.
// vaultPath is needed to stat files. Returns info about which files are stale.
func (d *Database) CheckStaleness(vaultPath string) (*StalenessInfo, error) {
	info := &StalenessInfo{}

	// Get all unique file paths and their indexed mtimes
	rows, err := d.db.Query(`
		SELECT DISTINCT file_path, file_mtime 
		FROM objects 
		WHERE parent_id IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var filePath string
		var indexedMtime sql.NullInt64

		if err := rows.Scan(&filePath, &indexedMtime); err != nil {
			return nil, err
		}

		info.TotalFiles++

		// Build full path and check current mtime
		fullPath := filepath.Join(vaultPath, filePath)
		stat, err := os.Stat(fullPath)
		if err != nil {
			// File was deleted or moved - consider stale
			info.StaleFiles = append(info.StaleFiles, filePath)
			info.IsStale = true
			continue
		}

		info.CheckedFiles++
		currentMtime := stat.ModTime().Unix()

		// If no indexed mtime or current > indexed, file is stale
		if !indexedMtime.Valid || currentMtime > indexedMtime.Int64 {
			info.StaleFiles = append(info.StaleFiles, filePath)
			info.IsStale = true
		}
	}

	return info, rows.Err()
}

// GetFileMtime returns the indexed mtime for a file, or 0 if not found.
func (d *Database) GetFileMtime(filePath string) (int64, error) {
	var mtime sql.NullInt64
	err := d.db.QueryRow(`
		SELECT file_mtime FROM objects 
		WHERE file_path = ? AND parent_id IS NULL 
		LIMIT 1
	`, filePath).Scan(&mtime)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !mtime.Valid {
		return 0, nil
	}
	return mtime.Int64, nil
}

// IsFileStale checks if a single file needs reindexing.
// Returns true if the file's current mtime is newer than indexed mtime.
func (d *Database) IsFileStale(vaultPath, filePath string) (bool, error) {
	indexedMtime, err := d.GetFileMtime(filePath)
	if err != nil {
		return false, err
	}

	// File not in index - needs indexing
	if indexedMtime == 0 {
		return true, nil
	}

	// Check current file mtime
	fullPath := filepath.Join(vaultPath, filePath)
	stat, err := os.Stat(fullPath)
	if err != nil {
		// File doesn't exist - consider stale (will be cleaned up)
		return true, nil
	}

	return stat.ModTime().Unix() > indexedMtime, nil
}

// ReferenceResolutionResult contains statistics about reference resolution.
type ReferenceResolutionResult struct {
	Resolved   int // Number of references successfully resolved
	Unresolved int // Number of references that couldn't be resolved
	Ambiguous  int // Number of ambiguous references (multiple matches)
	Total      int // Total number of references processed
}

// ResolveReferences resolves all unresolved references in the refs table.
// This should be called after all files have been indexed.
// dailyDirectory is used to resolve date shorthand references like [[2025-02-01]].
func (d *Database) ResolveReferences(dailyDirectory string) (*ReferenceResolutionResult, error) {
	result := &ReferenceResolutionResult{}

	// Get all object IDs for resolution
	objectIDs, err := d.AllObjectIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get object IDs: %w", err)
	}

	// Create resolver with all known object IDs
	res := resolver.NewWithDailyDir(objectIDs, dailyDirectory)

	// Get all unresolved references
	rows, err := d.db.Query(`
		SELECT id, target_raw FROM refs WHERE target_id IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query refs: %w", err)
	}
	defer rows.Close()

	type refToResolve struct {
		id        int64
		targetRaw string
	}
	var refs []refToResolve

	for rows.Next() {
		var r refToResolve
		if err := rows.Scan(&r.id, &r.targetRaw); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result.Total = len(refs)

	// Resolve each reference
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE refs SET target_id = ? WHERE id = ?`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	for _, ref := range refs {
		resolved := res.Resolve(ref.targetRaw)
		if resolved.Ambiguous {
			result.Ambiguous++
			result.Unresolved++
		} else if resolved.TargetID != "" {
			if _, err := stmt.Exec(resolved.TargetID, ref.id); err != nil {
				return nil, err
			}
			result.Resolved++
		} else {
			result.Unresolved++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}
