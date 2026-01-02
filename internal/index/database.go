// Package index handles SQLite database operations.
package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	_ "modernc.org/sqlite"
)

// Database is the SQLite database handle.
type Database struct {
	db *sql.DB
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

	return hasValueColumn
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

// CurrentDBVersion is the current database schema version.
const CurrentDBVersion = 3

// initialize creates the database schema.
func (d *Database) initialize() error {
	schema := `
		-- Enable WAL mode for better concurrency
		PRAGMA journal_mode = WAL;
		
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
			created_at INTEGER,
			updated_at INTEGER
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
			created_at INTEGER
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

		-- Tags table for efficient tag queries
		CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tag TEXT NOT NULL,               -- Tag name (without #)
			object_id TEXT NOT NULL,         -- Object this tag belongs to
			file_path TEXT NOT NULL,
			line_number INTEGER,
			UNIQUE(tag, object_id)
		);

		CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);
		CREATE INDEX IF NOT EXISTS idx_tags_object ON tags(object_id);
		CREATE INDEX IF NOT EXISTS idx_tags_file ON tags(file_path);
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
// The schema is needed to determine which frontmatter fields are traits.
func (d *Database) IndexDocument(doc *parser.ParsedDocument, sch *schema.Schema) error {
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
	if _, err := tx.Exec("DELETE FROM tags WHERE file_path = ?", doc.FilePath); err != nil {
		return err
	}

	now := time.Now().Unix()

	// Insert objects
	objStmt, err := tx.Prepare(`
		INSERT INTO objects (id, file_path, type, heading, heading_level, fields, line_start, line_end, parent_id, created_at, updated_at)
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
			now,
			now,
		)
		if err != nil {
			return err
		}
	}

	// Insert traits (both inline and frontmatter-based)
	traitStmt, err := tx.Prepare(`
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer traitStmt.Close()

	traitIdx := 0

	// First, index frontmatter traits from objects
	for _, obj := range doc.Objects {
		// Only file-level objects (no parent) have frontmatter traits
		if obj.ParentID != nil {
			continue
		}

		typeDef, typeExists := sch.Types[obj.ObjectType]
		if !typeExists || typeDef == nil {
			continue
		}

		// Check each trait the type declares
		for _, traitName := range typeDef.Traits.List() {
			fieldValue, hasField := obj.Fields[traitName]
			if !hasField {
				continue
			}

			traitID := fmt.Sprintf("%s:trait:%d", doc.FilePath, traitIdx)
			traitIdx++

			// Get value as string
			var valueStr interface{}
			if s, ok := fieldValue.AsString(); ok {
				valueStr = s
			}

			// Use object ID as content for frontmatter traits
			_, err = traitStmt.Exec(
				traitID,
				doc.FilePath,
				obj.ID,
				traitName,
				valueStr,
				obj.ID, // Content is the object itself
				obj.LineStart,
				now,
			)
			if err != nil {
				return err
			}
		}
	}

	// Then, index inline traits
	for _, trait := range doc.Traits {
		traitID := fmt.Sprintf("%s:trait:%d", doc.FilePath, traitIdx)
		traitIdx++

		// Get value as string (or nil for boolean traits)
		var valueStr interface{}
		if trait.Value != nil {
			if s, ok := trait.Value.AsString(); ok {
				valueStr = s
			}
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

	// Index tags from all objects
	tagStmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO tags (tag, object_id, file_path, line_number)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer tagStmt.Close()

	for _, obj := range doc.Objects {
		for _, tag := range obj.Tags {
			_, err = tagStmt.Exec(tag, obj.ID, doc.FilePath, obj.LineStart)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// extractDateString extracts a date string from a field value if it's a date type.
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
	if _, err := d.db.Exec("DELETE FROM tags WHERE file_path = ?", filePath); err != nil {
		return err
	}
	return nil
}

// RemoveDocument removes a document and all related data by its object ID.
func (d *Database) RemoveDocument(objectID string) error {
	// Objects can have IDs like "people/alice" or "daily/2025-02-01#meeting"
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
	if _, err := d.db.Exec("DELETE FROM tags WHERE file_path = ?", filePath); err != nil {
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
