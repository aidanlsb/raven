package indexschema

// CurrentDBVersion is the current database schema version.
//
// Bump this only when SchemaSQL changes the SQLite schema shape. Rebuild
// coordination uses a marker outside the database and does not require a
// version bump.
//
// v7: Added composite indexes for trait refs matching and performance PRAGMAs
// v8: Added alias column to objects table for reference aliasing
// v9: Added field_refs table for ref-typed fields
// v12: Added first-class sections table
// v13: Removed object hierarchy/heading columns; objects are file-backed only
// v14: Added subtree line ranges for heading-derived sections
// v15: Added lightweight outgoing link edges for non-Raven targets
// v16: Removed the obsolete non-Markdown resource table
const CurrentDBVersion = 16

// SchemaSQL is the authoritative SQLite schema shared by the index writer and
// SQL-generating consumers.
const SchemaSQL = `
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

	-- File-backed typed objects
	CREATE TABLE IF NOT EXISTS objects (
		id TEXT PRIMARY KEY,
		file_path TEXT NOT NULL,
		type TEXT NOT NULL,
		fields TEXT NOT NULL DEFAULT '{}',
		line_start INTEGER NOT NULL,
		alias TEXT,                 -- Optional alias for reference resolution
		file_mtime INTEGER,         -- File modification time from filesystem (Unix timestamp)
		indexed_at INTEGER          -- When this row was written to the index
	);

	-- Markdown heading-derived sections. Sections are addressable scopes, not objects.
	CREATE TABLE IF NOT EXISTS sections (
		id TEXT PRIMARY KEY,
		file_object_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		slug TEXT NOT NULL,
		title TEXT NOT NULL,
		level INTEGER NOT NULL,
		line_start INTEGER NOT NULL,
		line_end INTEGER,
		subtree_line_end INTEGER,
		parent_section_id TEXT,
		indexed_at INTEGER
	);

	-- All trait annotations (single-valued)
	CREATE TABLE IF NOT EXISTS traits (
		id TEXT PRIMARY KEY,
		file_path TEXT NOT NULL,
		parent_object_id TEXT NOT NULL,       -- Compatibility name; contains a file object or section scope ID
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

	-- Outgoing Markdown links to non-Raven file and URL targets
	CREATE TABLE IF NOT EXISTS links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id TEXT NOT NULL,
		source_type TEXT NOT NULL,
		file_path TEXT NOT NULL,
		line_number INTEGER NOT NULL,
		position_start INTEGER NOT NULL,
		position_end INTEGER NOT NULL,
		raw_target TEXT NOT NULL,
		display TEXT NOT NULL,
		is_image INTEGER NOT NULL,
		scheme TEXT NOT NULL,
		ext TEXT NOT NULL,
		normalized_key TEXT NOT NULL
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

	-- Keep resolver caches on other database handles/processes coherent,
	-- including when callers mutate resolver inputs through DB().Exec.
	CREATE TRIGGER IF NOT EXISTS trg_objects_resolver_generation_insert
	AFTER INSERT ON objects BEGIN
		INSERT INTO meta (key, value) VALUES ('resolver_generation', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1;
	END;
	CREATE TRIGGER IF NOT EXISTS trg_objects_resolver_generation_update
	AFTER UPDATE ON objects BEGIN
		INSERT INTO meta (key, value) VALUES ('resolver_generation', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1;
	END;
	CREATE TRIGGER IF NOT EXISTS trg_objects_resolver_generation_delete
	AFTER DELETE ON objects BEGIN
		INSERT INTO meta (key, value) VALUES ('resolver_generation', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1;
	END;

	CREATE TRIGGER IF NOT EXISTS trg_sections_resolver_generation_insert
	AFTER INSERT ON sections BEGIN
		INSERT INTO meta (key, value) VALUES ('resolver_generation', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1;
	END;
	CREATE TRIGGER IF NOT EXISTS trg_sections_resolver_generation_update
	AFTER UPDATE ON sections BEGIN
		INSERT INTO meta (key, value) VALUES ('resolver_generation', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1;
	END;
	CREATE TRIGGER IF NOT EXISTS trg_sections_resolver_generation_delete
	AFTER DELETE ON sections BEGIN
		INSERT INTO meta (key, value) VALUES ('resolver_generation', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1;
	END;

	-- Indexes for fast queries
	CREATE INDEX IF NOT EXISTS idx_objects_file ON objects(file_path);
	CREATE INDEX IF NOT EXISTS idx_objects_type ON objects(type);
	CREATE INDEX IF NOT EXISTS idx_objects_alias ON objects(alias) WHERE alias IS NOT NULL;

	CREATE INDEX IF NOT EXISTS idx_sections_file ON sections(file_path);
	CREATE INDEX IF NOT EXISTS idx_sections_file_object ON sections(file_object_id);
	CREATE INDEX IF NOT EXISTS idx_sections_parent ON sections(parent_section_id);

	CREATE INDEX IF NOT EXISTS idx_traits_file ON traits(file_path);
	CREATE INDEX IF NOT EXISTS idx_traits_type ON traits(trait_type);
	CREATE INDEX IF NOT EXISTS idx_traits_parent ON traits(parent_object_id);

	CREATE INDEX IF NOT EXISTS idx_refs_source ON refs(source_id);
	CREATE INDEX IF NOT EXISTS idx_refs_target ON refs(target_id);
	CREATE INDEX IF NOT EXISTS idx_refs_file ON refs(file_path);

	CREATE INDEX IF NOT EXISTS idx_links_source ON links(source_id);
	CREATE INDEX IF NOT EXISTS idx_links_file ON links(file_path);
	CREATE INDEX IF NOT EXISTS idx_links_normalized_key ON links(normalized_key);

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
