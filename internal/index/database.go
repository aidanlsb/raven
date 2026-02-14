// Package index handles SQLite database operations.
package index

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// Database is the SQLite database handle.
type Database struct {
	db              *sql.DB
	dailyDirectory  string
	autoResolveRefs bool
}

var (
	// ErrObjectNotFound indicates the requested object ID is not in the index.
	ErrObjectNotFound = errors.New("object not found in index")
	// ErrIndexLocked indicates another process is rebuilding the index.
	ErrIndexLocked = errors.New("index is locked for rebuild")
)

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

	d := &Database{db: db, dailyDirectory: "daily", autoResolveRefs: true}
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

	lock, err := acquireIndexLock(dbDir)
	if err != nil {
		return nil, false, err
	}
	defer lock.Release()

	// Try to open and check schema compatibility
	if _, err := os.Stat(dbPath); err == nil {
		db, err := sql.Open("sqlite", dbPath)
		if err == nil {
			if !isSchemaCompatible(db) {
				db.Close()
				// Schema incompatible - delete and recreate
				if err := removeDatabaseFiles(dbPath); err != nil {
					return nil, false, err
				}
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

type indexLock struct {
	file *os.File
}

func acquireIndexLock(dbDir string) (*indexLock, error) {
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .raven directory: %w", err)
	}

	lockPath := filepath.Join(dbDir, "index.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open index lock: %w", err)
	}

	if err := lockFileExclusiveNonBlocking(lockFile); err != nil {
		lockFile.Close()
		if isWouldBlockError(err) {
			return nil, ErrIndexLocked
		}
		return nil, fmt.Errorf("failed to acquire index lock: %w", err)
	}

	return &indexLock{file: lockFile}, nil
}

func (l *indexLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func removeDatabaseFiles(dbPath string) error {
	paths := []string{dbPath, dbPath + "-wal", dbPath + "-shm"}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove %s: %w", p, err)
		}
	}
	return nil
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

	// Check if field_refs table exists (v9+)
	var fieldRefsTableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='field_refs'").Scan(&fieldRefsTableName)
	if err != nil {
		return false
	}

	// Check if objects table has required columns (v6+: indexed_at, v8+: alias)
	rows2, err := db.Query("PRAGMA table_info(objects)")
	if err != nil {
		return false
	}
	defer rows2.Close()

	hasIndexedAtColumn := false
	hasAliasColumn := false
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
		}
		if name == "alias" {
			hasAliasColumn = true
		}
	}

	return hasIndexedAtColumn && hasAliasColumn
}

// OpenInMemory opens an in-memory database (for testing).
func OpenInMemory() (*Database, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	d := &Database{db: db, dailyDirectory: "daily", autoResolveRefs: true}
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

// SetDailyDirectory configures the daily notes directory for reference resolution.
func (d *Database) SetDailyDirectory(dailyDir string) {
	if dailyDir == "" {
		d.dailyDirectory = "daily"
		return
	}
	d.dailyDirectory = dailyDir
}

// SetAutoResolveRefs toggles resolve-on-write behavior.
func (d *Database) SetAutoResolveRefs(enabled bool) {
	d.autoResolveRefs = enabled
}

// Analyze runs SQLite's ANALYZE command to update query planner statistics.
// This should be called after bulk indexing operations for optimal query performance.
func (d *Database) Analyze() error {
	_, err := d.db.Exec("ANALYZE")
	return err
}

// CurrentDBVersion is the current database schema version.
// v7: Added composite indexes for trait refs matching and performance PRAGMAs
// v8: Added alias column to objects table for reference aliasing
// v9: Added field_refs table for ref-typed fields
const CurrentDBVersion = 9

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
			alias TEXT,                 -- Optional alias for reference resolution
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

		-- References from ref-typed fields (schema-aware)
		CREATE TABLE IF NOT EXISTS field_refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			target_id TEXT,
			target_raw TEXT NOT NULL,
			resolution_status TEXT NOT NULL, -- resolved | ambiguous | missing
			file_path TEXT NOT NULL,
			line_number INTEGER
		);
		
		-- Indexes for fast queries
		CREATE INDEX IF NOT EXISTS idx_objects_file ON objects(file_path);
		CREATE INDEX IF NOT EXISTS idx_objects_type ON objects(type);
		CREATE INDEX IF NOT EXISTS idx_objects_parent ON objects(parent_id);
		CREATE INDEX IF NOT EXISTS idx_objects_alias ON objects(alias) WHERE alias IS NOT NULL;
		
		CREATE INDEX IF NOT EXISTS idx_traits_file ON traits(file_path);
		CREATE INDEX IF NOT EXISTS idx_traits_type ON traits(trait_type);
		CREATE INDEX IF NOT EXISTS idx_traits_parent ON traits(parent_object_id);
		
		CREATE INDEX IF NOT EXISTS idx_refs_source ON refs(source_id);
		CREATE INDEX IF NOT EXISTS idx_refs_target ON refs(target_id);
		CREATE INDEX IF NOT EXISTS idx_refs_file ON refs(file_path);

		CREATE INDEX IF NOT EXISTS idx_field_refs_source_field ON field_refs(source_id, field_name);
		CREATE INDEX IF NOT EXISTS idx_field_refs_field_target ON field_refs(field_name, target_id);
		CREATE INDEX IF NOT EXISTS idx_field_refs_field_raw ON field_refs(field_name, target_raw);
		CREATE INDEX IF NOT EXISTS idx_field_refs_status ON field_refs(resolution_status);
		CREATE INDEX IF NOT EXISTS idx_field_refs_file ON field_refs(file_path);
		
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
	if err := deleteByFilePath(tx, doc.FilePath); err != nil {
		return err
	}

	now := time.Now().Unix()

	// Use provided mtime or fall back to current time
	mtime := indexedMtime(now, fileMtime)

	if err := indexObjects(tx, doc, mtime, now); err != nil {
		return err
	}
	if err := indexInlineTraits(tx, doc, sch, now); err != nil {
		return err
	}
	if err := indexRefs(tx, doc, sch); err != nil {
		return err
	}
	if err := indexFieldRefs(tx, doc, sch); err != nil {
		return err
	}
	if err := indexDates(tx, doc, sch); err != nil {
		return err
	}
	if err := indexFTS(tx, doc, sch); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if d.autoResolveRefs && d.dailyDirectory != "" {
		if _, err := d.ResolveReferencesForFile(doc.FilePath, d.dailyDirectory); err != nil {
			return err
		}
	}

	return nil
}

func indexedMtime(now, fileMtime int64) int64 {
	mtime := fileMtime
	if mtime == 0 {
		mtime = now
	}
	return mtime
}

func indexObjects(tx *sql.Tx, doc *parser.ParsedDocument, mtime, indexedAt int64) error {
	objStmt, err := tx.Prepare(`
		INSERT INTO objects (id, file_path, type, heading, heading_level, fields, line_start, line_end, parent_id, alias, file_mtime, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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

		// Extract alias from fields if present
		var alias *string
		if aliasField, ok := obj.Fields["alias"]; ok {
			if s, ok := aliasField.AsString(); ok && s != "" {
				alias = &s
			}
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
			alias,
			mtime,
			indexedAt,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func indexInlineTraits(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema, indexedAt int64) error {
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
			indexedAt,
		)
		if execErr != nil {
			return execErr
		}
	}

	return nil
}

func indexRefs(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	refStmt, err := tx.Prepare(`
		INSERT INTO refs (source_id, target_id, target_raw, display_text, file_path, line_number, position_start, position_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer refStmt.Close()

	// Collect all refs: parsed refs + refs from schema-typed ref fields
	allRefs := doc.Refs

	// Extract additional refs from ref-typed fields in frontmatter/embedded objects.
	// This allows `company: cursor` to work when the schema declares `company: ref`.
	if sch != nil {
		schemaRefs := extractRefsFromSchemaFields(doc.Objects, sch, doc.FilePath)
		allRefs = mergeRefs(allRefs, schemaRefs)
	}

	for _, ref := range allRefs {
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

	return nil
}

type fieldRefToIndex struct {
	SourceID  string
	FieldName string
	TargetRaw string
	Line      int
}

func indexFieldRefs(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	if sch == nil {
		return nil
	}

	fieldRefs := extractFieldRefsFromSchemaFields(doc.Objects, sch)
	if len(fieldRefs) == 0 {
		return nil
	}

	stmt, err := tx.Prepare(`
		INSERT INTO field_refs (source_id, field_name, target_raw, target_id, resolution_status, file_path, line_number)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range fieldRefs {
		if ref.TargetRaw == "" {
			continue
		}
		_, err = stmt.Exec(
			ref.SourceID,
			ref.FieldName,
			ref.TargetRaw,
			nil,
			"missing",
			doc.FilePath,
			ref.Line,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func indexDates(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
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

	traitIdx := 0
	for _, trait := range doc.Traits {
		// Skip undefined traits - schema is source of truth.
		// traitIdx must only increment for defined traits to match indexInlineTraits.
		if sch != nil {
			if _, defined := sch.Traits[trait.TraitType]; !defined {
				continue
			}
		}

		traitID := fmt.Sprintf("%s:trait:%d", doc.FilePath, traitIdx)
		traitIdx++
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

	return nil
}

func indexFTS(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	ftsStmt, err := tx.Prepare(`
		INSERT INTO fts_content (object_id, title, content, file_path)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	// Pre-split content into lines for section extraction
	lines := strings.Split(doc.RawContent, "\n")

	for _, obj := range doc.Objects {
		// Get title: check schema name_field first, then "title" field, then heading, then object ID
		title := ""
		if sch != nil && obj.ObjectType != "" {
			if typeDef, ok := sch.Types[obj.ObjectType]; ok && typeDef.NameField != "" {
				if nameVal, ok := obj.Fields[typeDef.NameField]; ok {
					if s, ok := nameVal.AsString(); ok {
						title = s
					}
				}
			}
		}
		if title == "" {
			if titleField, ok := obj.Fields["title"]; ok {
				if s, ok := titleField.AsString(); ok {
					title = s
				}
			}
		}
		if title == "" && obj.Heading != nil {
			title = *obj.Heading
		}
		if title == "" {
			// Use object ID as title for file-level objects
			title = obj.ID
		}

		// Get content for this object
		content := ""
		if obj.ParentID == nil {
			// File-level object - index body content (excludes frontmatter)
			content = doc.Body
		} else {
			// Embedded object - extract section content between LineStart and LineEnd
			content = extractSectionContent(lines, obj.LineStart, obj.LineEnd)
		}

		_, err = ftsStmt.Exec(obj.ID, title, content, doc.FilePath)
		if err != nil {
			return err
		}
	}

	return nil
}

// extractSectionContent extracts content for an embedded object from the given line range.
// lineStart and lineEnd are 1-indexed. If lineEnd is nil, extracts to end of file.
func extractSectionContent(lines []string, lineStart int, lineEnd *int) string {
	if lineStart < 1 || lineStart > len(lines) {
		return ""
	}

	// Convert to 0-indexed
	start := lineStart - 1
	end := len(lines)
	if lineEnd != nil && *lineEnd <= len(lines) {
		end = *lineEnd // lineEnd is exclusive (the next section starts here)
	}

	if start >= end {
		return ""
	}

	return strings.Join(lines[start:end], "\n")
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
			candidate := s[:10] // Return just the date part (in case of datetime)
			if dates.IsValidDate(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// RemoveFile removes all data for a file.
func (d *Database) RemoveFile(filePath string) error {
	return deleteByFilePath(d.db, filePath)
}

// ClearAllData removes all indexed data from the database.
// This is used for full reindex to ensure a clean slate.
func (d *Database) ClearAllData() error {
	if _, err := d.db.Exec("DELETE FROM objects"); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM traits"); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM refs"); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM field_refs"); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM date_index"); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM fts_content"); err != nil {
		return err
	}
	return nil
}

// RemoveFilesWithPrefix removes all data for files whose paths start with a given prefix.
// This is used to clean up files in excluded directories like .trash/.
// Returns the number of files removed.
func (d *Database) RemoveFilesWithPrefix(pathPrefix string) (int, error) {
	// Count files that will be removed
	count, err := countDistinctFilesWithPrefix(d.db, pathPrefix)
	if err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, nil
	}

	pattern := pathPrefix + "%"
	if err := deleteByFilePathLike(d.db, pattern); err != nil {
		return 0, err
	}
	return count, nil
}

func countDistinctFilesWithPrefix(db *sql.DB, pathPrefix string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(DISTINCT file_path) FROM objects WHERE file_path LIKE ?", pathPrefix+"%").Scan(&count)
	if err != nil {
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
		if fileMissing(filepath.Join(vaultPath, relPath)) {
			// File was deleted - remove from index
			if err := d.RemoveFile(relPath); err != nil {
				return removed, fmt.Errorf("failed to remove %s: %w", relPath, err)
			}
			removed = append(removed, relPath)
		}
	}

	return removed, nil
}

func fileMissing(fullPath string) bool {
	_, err := os.Stat(fullPath)
	return os.IsNotExist(err)
}

// RemoveDocument removes a document and all related data by its object ID.
func (d *Database) RemoveDocument(objectID string) error {
	// Objects can have IDs like "people/freya" or "daily/2025-02-01#meeting".
	// This method removes the *entire file/document* from the index.
	//
	// Callers may pass an embedded/section ID (with a '#'). In that case we still
	// remove the whole document, since Raven cannot delete embedded objects from
	// the markdown file without rewriting content.
	baseID := baseDocumentID(objectID)

	// Prefer the canonical file_path stored in the DB (important when directory
	// roots are configured and object IDs do not match file paths).
	var filePath string
	err := d.db.QueryRow(
		"SELECT file_path FROM objects WHERE id = ? OR id LIKE ? LIMIT 1",
		baseID,
		baseID+"#%",
	).Scan(&filePath)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrObjectNotFound
		} else {
			return err
		}
	}

	// Delete all objects in this document (file-level + sections/embedded).
	if _, err := d.db.Exec("DELETE FROM objects WHERE id = ? OR id LIKE ?", baseID, baseID+"#%"); err != nil {
		return err
	}

	// Delete related data by file path
	if err := deleteRelatedByFilePath(d.db, filePath); err != nil {
		return err
	}
	return nil
}

func baseDocumentID(objectID string) string {
	baseID := objectID
	if hash := strings.Index(baseID, "#"); hash >= 0 {
		baseID = baseID[:hash]
	}
	return baseID
}

func deleteRelatedByFilePath(db *sql.DB, filePath string) error {
	if _, err := db.Exec("DELETE FROM traits WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM refs WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM field_refs WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM date_index WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM fts_content WHERE file_path = ?", filePath); err != nil {
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

// AllAliases returns a map from alias to object ID for all objects with aliases.
// This is used for reference resolution where [[alias]] should resolve to the object.
// If multiple objects have the same alias, the first one encountered wins (non-deterministic).
// Use FindDuplicateAliases to detect and report conflicts.
func (d *Database) AllAliases() (map[string]string, error) {
	rows, err := d.db.Query("SELECT alias, id FROM objects WHERE alias IS NOT NULL AND alias != '' ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make(map[string]string)
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, err
		}
		// First one wins (deterministic due to ORDER BY id)
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = id
		}
	}

	return aliases, rows.Err()
}

// ResolverOptions configures resolver creation.
type ResolverOptions struct {
	// DailyDirectory is the directory for daily notes (default: "daily").
	DailyDirectory string

	// Schema enables name_field resolution for semantic matching.
	// When provided, [[The Prose Edda]] can resolve to books/the-prose-edda
	// if the book type has name_field: title.
	Schema *schema.Schema

	// ExtraIDs are additional object IDs to include in the resolver.
	// Useful for hypothetical resolution (e.g., testing if refs will
	// resolve after a move operation).
	ExtraIDs []string
}

// Resolver builds the canonical resolver for this vault index.
//
// This is the ONE resolver factory that handles all cases:
// - Object IDs (full path + short name resolution)
// - Aliases (e.g., [[The Queen]] → people/freya)
// - Name field values (e.g., [[The Prose Edda]] → books/the-prose-edda) - when Schema provided
// - Date shorthand (e.g., [[2025-02-01]] → daily/2025-02-01)
// - Extra IDs for hypothetical resolution
//
// Use this method for all resolver creation to ensure consistent behavior.
func (d *Database) Resolver(opts ResolverOptions) (*resolver.Resolver, error) {
	dailyDir := defaultDailyDir(opts.DailyDirectory)

	objectIDs, err := d.AllObjectIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get object IDs: %w", err)
	}

	aliases, err := d.AllAliases()
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases: %w", err)
	}

	// Add extra IDs if provided (for hypothetical resolution)
	objectIDs = appendExtraIDs(objectIDs, opts.ExtraIDs)

	// Include name_field values if schema is provided
	resolverOpts := resolver.Options{
		DailyDirectory: dailyDir,
		Aliases:        aliases,
	}
	if opts.Schema != nil {
		nameFieldMap, err := d.AllNameFieldValues(opts.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to get name field values: %w", err)
		}
		resolverOpts.NameFieldMap = nameFieldMap
	}

	return resolver.New(objectIDs, resolverOpts), nil
}

func defaultDailyDir(dailyDir string) string {
	if dailyDir == "" {
		return "daily"
	}
	return dailyDir
}

// appendExtraIDs appends extra IDs to objectIDs, preserving order and de-duplicating.
// Empty extra IDs are ignored.
func appendExtraIDs(objectIDs []string, extraIDs []string) []string {
	if len(extraIDs) == 0 {
		return objectIDs
	}
	seen := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		seen[id] = struct{}{}
	}
	for _, id := range extraIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		objectIDs = append(objectIDs, id)
		seen[id] = struct{}{}
	}
	return objectIDs
}

// AllNameFieldValues returns a map from name_field values to object IDs.
// It queries each type's name_field and extracts the corresponding field value.
func (d *Database) AllNameFieldValues(sch *schema.Schema) (map[string]string, error) {
	nameFieldMap := make(map[string]string)

	if sch == nil {
		return nameFieldMap, nil
	}

	// Build a map of type -> name_field
	typeNameFields := buildTypeNameFields(sch)

	if len(typeNameFields) == 0 {
		return nameFieldMap, nil
	}

	// Query all objects and extract name_field values
	rows, err := d.db.Query(`SELECT id, type, fields FROM objects WHERE type != '' AND fields != '{}'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		id, objType, fieldsJSON, ok := scanNameFieldRow(rows)
		if !ok {
			continue
		}
		nameStr, ok := extractNameFieldValue(typeNameFields, objType, fieldsJSON)
		if !ok {
			continue
		}
		// Preserve existing semantics: last assignment wins (query order unspecified).
		nameFieldMap[nameStr] = id
	}

	return nameFieldMap, rows.Err()
}

func buildTypeNameFields(sch *schema.Schema) map[string]string {
	typeNameFields := make(map[string]string)
	if sch == nil {
		return typeNameFields
	}
	for typeName, typeDef := range sch.Types {
		if typeDef != nil && typeDef.NameField != "" {
			typeNameFields[typeName] = typeDef.NameField
		}
	}
	return typeNameFields
}

func scanNameFieldRow(rows *sql.Rows) (id string, objType string, fieldsJSON string, ok bool) {
	if err := rows.Scan(&id, &objType, &fieldsJSON); err != nil {
		return "", "", "", false
	}
	return id, objType, fieldsJSON, true
}

func extractNameFieldValue(typeNameFields map[string]string, objType string, fieldsJSON string) (string, bool) {
	nameField, ok := typeNameFields[objType]
	if !ok {
		return "", false
	}

	// Parse fields JSON and extract name_field value
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return "", false
	}
	nameValue, ok := fields[nameField]
	if !ok {
		return "", false
	}
	nameStr, ok := nameValue.(string)
	if !ok || nameStr == "" {
		return "", false
	}
	return nameStr, true
}

// DuplicateAlias represents multiple objects sharing the same alias.
type DuplicateAlias struct {
	Alias     string   // The duplicated alias
	ObjectIDs []string // All object IDs using this alias
}

// FindDuplicateAliases finds cases where multiple objects use the same alias.
// This is a validation issue that should be reported to the user.
func (d *Database) FindDuplicateAliases() ([]DuplicateAlias, error) {
	// Find aliases that appear more than once
	rows, err := d.db.Query(`
		SELECT alias, GROUP_CONCAT(id, '|') as ids
		FROM objects 
		WHERE alias IS NOT NULL AND alias != ''
		GROUP BY alias 
		HAVING COUNT(*) > 1
		ORDER BY alias
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var duplicates []DuplicateAlias
	for rows.Next() {
		var alias, idsConcat string
		if err := rows.Scan(&alias, &idsConcat); err != nil {
			return nil, err
		}
		ids := strings.Split(idsConcat, "|")
		duplicates = append(duplicates, DuplicateAlias{
			Alias:     alias,
			ObjectIDs: ids,
		})
	}

	return duplicates, rows.Err()
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
	rows, err := stalenessRows(d.db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		filePath, indexedMtime, err := scanStalenessRow(rows)
		if err != nil {
			return nil, err
		}

		info.TotalFiles++

		stale, checked, err := isFileStaleAgainstIndexedMtime(filepath.Join(vaultPath, filePath), indexedMtime)
		if err != nil {
			// File was deleted or moved - consider stale
			info.StaleFiles = append(info.StaleFiles, filePath)
			info.IsStale = true
			continue
		}
		if checked {
			info.CheckedFiles++
		}
		if stale {
			info.StaleFiles = append(info.StaleFiles, filePath)
			info.IsStale = true
		}
	}

	return info, rows.Err()
}

func stalenessRows(db *sql.DB) (*sql.Rows, error) {
	return db.Query(`
		SELECT DISTINCT file_path, file_mtime 
		FROM objects 
		WHERE parent_id IS NULL
	`)
}

func scanStalenessRow(rows *sql.Rows) (string, sql.NullInt64, error) {
	var filePath string
	var indexedMtime sql.NullInt64
	if err := rows.Scan(&filePath, &indexedMtime); err != nil {
		return "", sql.NullInt64{}, err
	}
	return filePath, indexedMtime, nil
}

// isFileStaleAgainstIndexedMtime compares the current filesystem mtime to the indexed one.
//
// Returns:
// - stale: whether file should be considered stale (including missing indexed mtime)
// - checked: whether the file existed on disk (i.e., mtime was checked)
// - err: non-nil when os.Stat fails (caller decides how to treat)
func isFileStaleAgainstIndexedMtime(fullPath string, indexedMtime sql.NullInt64) (stale bool, checked bool, err error) {
	stat, err := os.Stat(fullPath)
	if err != nil {
		return false, false, err
	}
	checked = true
	currentMtime := stat.ModTime().Unix()
	// If no indexed mtime or current > indexed, file is stale
	if !indexedMtime.Valid || currentMtime > indexedMtime.Int64 {
		return true, checked, nil
	}
	return false, checked, nil
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
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}

	return stat.ModTime().Unix() > indexedMtime, nil
}

// ReferenceResolutionResult contains statistics about reference resolution.
type ReferenceResolutionResult struct {
	Resolved   int // Number of references successfully resolved
	Unresolved int // Number of references that couldn't be resolved
	Ambiguous  int // Number of ambiguous references (multiple matches)
	Total      int // Total number of references processed

	FieldResolved   int // Number of field refs successfully resolved
	FieldUnresolved int // Number of field refs that couldn't be resolved
	FieldAmbiguous  int // Number of ambiguous field refs (multiple matches)
	FieldTotal      int // Total number of field refs processed
}

// ResolveReferences resolves all unresolved references in the refs table.
// This should be called after all files have been indexed.
// dailyDirectory is used to resolve date shorthand references like [[2025-02-01]].
func (d *Database) ResolveReferences(dailyDirectory string) (*ReferenceResolutionResult, error) {
	result := &ReferenceResolutionResult{}

	res, err := d.Resolver(ResolverOptions{DailyDirectory: dailyDirectory})
	if err != nil {
		return nil, err
	}

	if err := d.resolveReferencesInBatches(res, nil, result); err != nil {
		return nil, err
	}
	if err := d.resolveFieldRefsInBatches(res, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// ResolveReferencesForFile resolves unresolved references for a single file.
//
// This exists to support auto-reindex after CLI mutations without requiring a full
// vault-wide reference resolution pass.
func (d *Database) ResolveReferencesForFile(filePath, dailyDirectory string) (*ReferenceResolutionResult, error) {
	result := &ReferenceResolutionResult{}

	res, err := d.Resolver(ResolverOptions{DailyDirectory: dailyDirectory})
	if err != nil {
		return nil, err
	}

	if err := d.resolveReferencesInBatches(res, &filePath, result); err != nil {
		return nil, err
	}
	if err := d.resolveFieldRefsInBatches(res, &filePath, result); err != nil {
		return nil, err
	}

	return result, nil
}

const resolveRefsBatchSize = 750

type refToResolve struct {
	id        int64
	targetRaw string
}

type fieldRefToResolve struct {
	id        int64
	targetRaw string
}

func (d *Database) resolveReferencesInBatches(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	var lastID int64
	for {
		refs, err := d.fetchUnresolvedRefsBatch(filePath, lastID, resolveRefsBatchSize)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}

		if err := d.resolveRefBatch(res, refs, result); err != nil {
			return err
		}

		lastID = refs[len(refs)-1].id
	}
}

func (d *Database) resolveFieldRefsInBatches(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	var lastID int64
	for {
		refs, err := d.fetchUnresolvedFieldRefsBatch(filePath, lastID, resolveRefsBatchSize)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}

		if err := d.resolveFieldRefBatch(res, refs, result); err != nil {
			return err
		}

		lastID = refs[len(refs)-1].id
	}
}

func (d *Database) fetchUnresolvedRefsBatch(filePath *string, afterID int64, limit int) ([]refToResolve, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if filePath == nil {
		rows, err = d.db.Query(`SELECT id, target_raw FROM refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`, afterID, limit)
	} else {
		rows, err = d.db.Query(`SELECT id, target_raw FROM refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`, *filePath, afterID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query refs: %w", err)
	}
	defer rows.Close()

	refs := make([]refToResolve, 0, limit)
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

	return refs, nil
}

func (d *Database) fetchUnresolvedFieldRefsBatch(filePath *string, afterID int64, limit int) ([]fieldRefToResolve, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if filePath == nil {
		rows, err = d.db.Query(`SELECT id, target_raw FROM field_refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`, afterID, limit)
	} else {
		rows, err = d.db.Query(`SELECT id, target_raw FROM field_refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`, *filePath, afterID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query field refs: %w", err)
	}
	defer rows.Close()

	refs := make([]fieldRefToResolve, 0, limit)
	for rows.Next() {
		var r fieldRefToResolve
		if err := rows.Scan(&r.id, &r.targetRaw); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return refs, nil
}

func (d *Database) resolveRefBatch(res *resolver.Resolver, refs []refToResolve, result *ReferenceResolutionResult) error {
	result.Total += len(refs)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE refs SET target_id = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range refs {
		resolved := res.Resolve(ref.targetRaw)
		if resolved.Ambiguous {
			result.Ambiguous++
			result.Unresolved++
			continue
		}
		if resolved.TargetID != "" {
			if _, err := stmt.Exec(resolved.TargetID, ref.id); err != nil {
				return err
			}
			result.Resolved++
		} else {
			result.Unresolved++
		}
	}

	return tx.Commit()
}

func (d *Database) resolveFieldRefBatch(res *resolver.Resolver, refs []fieldRefToResolve, result *ReferenceResolutionResult) error {
	result.FieldTotal += len(refs)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE field_refs SET target_id = ?, resolution_status = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range refs {
		resolved := res.Resolve(ref.targetRaw)
		if resolved.Ambiguous {
			result.FieldAmbiguous++
			result.FieldUnresolved++
			if _, err := stmt.Exec(nil, "ambiguous", ref.id); err != nil {
				return err
			}
			continue
		}
		if resolved.TargetID != "" {
			if _, err := stmt.Exec(resolved.TargetID, "resolved", ref.id); err != nil {
				return err
			}
			result.FieldResolved++
		} else {
			if _, err := stmt.Exec(nil, "missing", ref.id); err != nil {
				return err
			}
			result.FieldUnresolved++
		}
	}

	return tx.Commit()
}

// extractRefsFromSchemaFields extracts refs from ref-typed fields that are bare strings.
// This enables `company: cursor` to work when the schema declares `company: ref`.
//
// The parser doesn't have schema context, so bare strings like "cursor" are stored as strings.
// At index time, we use the schema to identify ref-typed fields and extract their values as refs.
func extractRefsFromSchemaFields(objects []*parser.ParsedObject, sch *schema.Schema, filePath string) []*parser.ParsedRef {
	var refs []*parser.ParsedRef
	opts := parser.RefExtractOptions{AllowBareStrings: true}

	for _, obj := range objects {
		// Get type definition from schema
		typeDef := sch.Types[obj.ObjectType]
		if typeDef == nil {
			continue
		}

		for fieldName, fieldValue := range obj.Fields {
			fieldDef := typeDef.Fields[fieldName]
			if fieldDef == nil {
				continue
			}

			// Check if field is a ref or ref[] type
			switch fieldDef.Type {
			case schema.FieldTypeRef:
				// Single ref field - extract target from string value
				if targets := parser.ExtractRefsFromFieldValue(fieldValue, opts); len(targets) > 0 {
					refs = append(refs, &parser.ParsedRef{
						SourceID:  obj.ID,
						TargetRaw: targets[0].TargetRaw,
						Line:      obj.LineStart,
					})
				}

			case schema.FieldTypeRefArray:
				// Array of refs - extract targets from each element
				for _, target := range parser.ExtractRefsFromFieldValue(fieldValue, opts) {
					if target.TargetRaw == "" {
						continue
					}
					refs = append(refs, &parser.ParsedRef{
						SourceID:  obj.ID,
						TargetRaw: target.TargetRaw,
						Line:      obj.LineStart,
					})
				}
			}
		}
	}

	return refs
}

func extractFieldRefsFromSchemaFields(objects []*parser.ParsedObject, sch *schema.Schema) []fieldRefToIndex {
	var refs []fieldRefToIndex
	opts := parser.RefExtractOptions{AllowBareStrings: true}

	for _, obj := range objects {
		typeDef := sch.Types[obj.ObjectType]
		if typeDef == nil {
			continue
		}

		for fieldName, fieldValue := range obj.Fields {
			fieldDef := typeDef.Fields[fieldName]
			if fieldDef == nil {
				continue
			}

			switch fieldDef.Type {
			case schema.FieldTypeRef:
				if targets := parser.ExtractRefsFromFieldValue(fieldValue, opts); len(targets) > 0 {
					refs = append(refs, fieldRefToIndex{
						SourceID:  obj.ID,
						FieldName: fieldName,
						TargetRaw: targets[0].TargetRaw,
						Line:      obj.LineStart,
					})
				}
			case schema.FieldTypeRefArray:
				for _, target := range parser.ExtractRefsFromFieldValue(fieldValue, opts) {
					if target.TargetRaw == "" {
						continue
					}
					refs = append(refs, fieldRefToIndex{
						SourceID:  obj.ID,
						FieldName: fieldName,
						TargetRaw: target.TargetRaw,
						Line:      obj.LineStart,
					})
				}
			}
		}
	}

	return refs
}

// mergeRefs merges two ref slices, deduplicating by (sourceID, targetRaw) pairs.
// This prevents double-indexing when a ref is both:
// 1. Found by raw YAML scanning (as [[target]])
// 2. Extracted from a ref-typed field
func mergeRefs(existing, additional []*parser.ParsedRef) []*parser.ParsedRef {
	// Build a set of existing (sourceID, targetRaw) pairs
	seen := make(map[string]bool)
	for _, ref := range existing {
		key := ref.SourceID + "\x00" + ref.TargetRaw
		seen[key] = true
	}

	// Add new refs that aren't duplicates
	result := existing
	for _, ref := range additional {
		key := ref.SourceID + "\x00" + ref.TargetRaw
		if !seen[key] {
			result = append(result, ref)
			seen[key] = true
		}
	}

	return result
}
