package query

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE objects (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			type TEXT NOT NULL,
			fields TEXT NOT NULL DEFAULT '{}',
			line_start INTEGER NOT NULL,
			created_at INTEGER,
			updated_at INTEGER
		);

		CREATE TABLE traits (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			parent_object_id TEXT NOT NULL,
			trait_type TEXT NOT NULL,
			value TEXT,
			content TEXT NOT NULL,
			line_number INTEGER NOT NULL,
			created_at INTEGER
		);

		CREATE TABLE sections (
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

		CREATE TABLE refs (
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

		CREATE TABLE links (
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

		CREATE TABLE field_refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			target_id TEXT,
			target_raw TEXT NOT NULL,
			resolution_status TEXT NOT NULL,
			file_path TEXT NOT NULL,
			line_number INTEGER
		);

		CREATE TABLE assets (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL UNIQUE,
			media_type TEXT,
			extension TEXT,
			filename TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			file_mtime INTEGER,
			indexed_at INTEGER
		);

		CREATE TABLE date_index (
			date TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			file_path TEXT NOT NULL,
			PRIMARY KEY (date, source_type, source_id, field_name)
		);

		CREATE VIRTUAL TABLE fts_content USING fts5(
			object_id,
			title,
			content,
			file_path UNINDEXED,
			tokenize='porter unicode61'
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('projects/website', 'projects/website.md', 'project', '{"status":"active","priority":"high"}', 1),
			('projects/mobile', 'projects/mobile.md', 'project', '{"status":"paused","priority":"medium"}', 1),
			('people/freya', 'people/freya.md', 'person', '{"name":"Freya","email":"freya@asgard.realm"}', 1),
			('people/loki', 'people/loki.md', 'person', '{"name":"Loki"}', 1),
			('daily/2025-02-01', 'daily/2025-02-01.md', 'date', '{}', 1);

		-- Heading-derived sections.
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id) VALUES
			('projects/website#tasks', 'projects/website', 'projects/website.md', 'tasks', 'Tasks', 2, 20, 49, 49, NULL),
			('projects/website#design', 'projects/website', 'projects/website.md', 'design', 'Design', 2, 50, NULL, NULL, NULL),
			('projects/mobile#tasks', 'projects/mobile', 'projects/mobile.md', 'tasks', 'Tasks', 2, 15, NULL, NULL, NULL),
			('daily/2025-02-01#standup', 'daily/2025-02-01', 'daily/2025-02-01.md', 'standup', 'Standup', 2, 10, 29, 29, NULL),
			('daily/2025-02-01#planning', 'daily/2025-02-01', 'daily/2025-02-01.md', 'planning', 'Planning', 2, 30, NULL, NULL, NULL);

		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('trait1', 'projects/website.md', 'projects/website', 'due', '2025-06-30', 'projects/website', 1),
			('trait2', 'daily/2025-02-01.md', 'daily/2025-02-01#standup', 'due', '2025-02-03', 'Follow up on timeline', 15),
			('trait3', 'daily/2025-02-01.md', 'daily/2025-02-01#standup', 'highlight', NULL, 'Important insight', 18),
			('trait4', 'people/freya.md', 'people/freya', 'due', '2025-02-01', 'Send docs', 12),
			-- Traits on nested sections for contains tests
			('trait5', 'projects/website.md', 'projects/website#tasks', 'todo', 'todo', 'Build landing page', 25),
			('trait6', 'projects/website.md', 'projects/website#tasks', 'priority', 'high', 'Build landing page', 25),
			('trait7', 'projects/mobile.md', 'projects/mobile#tasks', 'todo', 'done', 'Setup CI/CD', 20),
			-- Test case for unresolved refs (target_id is NULL)
			('trait8', 'projects/mobile.md', 'projects/mobile#tasks', 'todo', 'todo', 'Cross-project task [[projects/website]]', 30),
			-- Array-valued traits are stored as JSON arrays in traits.value
			('trait9', 'projects/website.md', 'projects/website', 'tags', '["raven","skills"]', 'Review built-in skills', 40),
			('trait10', 'projects/mobile.md', 'projects/mobile', 'tags', '["mobile","ios"]', 'Review mobile tags', 40),
			('trait11', 'people/freya.md', 'people/freya', 'reviewers', '["people/freya","people/loki"]', 'Assigned reviewers', 40);

		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('daily/2025-02-01#standup', 'projects/website', 'projects/website', 'daily/2025-02-01.md', 12),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 13),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 15),
			('daily/2025-02-01#planning', 'projects/mobile', 'projects/mobile', 'daily/2025-02-01.md', 32),
			('daily/2025-02-01#planning', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 33),
			('projects/website', 'people/freya', 'people/freya', 'projects/website.md', 5),
			('projects/website', 'assets/pdfs/paper.pdf', 'assets/pdfs/paper.pdf', 'projects/website.md', 6),
			('projects/website#tasks', 'assets/images/diagram.png', 'assets/images/diagram.png', 'projects/website.md', 26),
			('trait5', 'assets/images/diagram.png', 'assets/images/diagram.png', 'projects/website.md', 25),
			-- Unresolved ref (target_id is NULL) - tests fallback to target_raw matching
			('projects/mobile#tasks', NULL, 'projects/website', 'projects/mobile.md', 30);

		INSERT INTO links (
			source_id, source_type, file_path, line_number, position_start, position_end,
			raw_target, display, is_image, scheme, ext, normalized_key
		) VALUES
			('projects/website', 'project', 'projects/website.md', 6, 0, 30, '../assets/report.pdf', 'report', 0, 'file', 'pdf', 'assets/report.pdf'),
			('projects/website', 'project', 'projects/website.md', 25, 0, 32, '../assets/diagram.png', 'diagram', 1, 'file', 'png', 'assets/diagram.png'),
			('projects/website', 'project', 'projects/website.md', 55, 0, 28, 'https://example.com', 'site', 0, 'url', '', 'https://example.com/'),
			('projects/mobile', 'project', 'projects/mobile.md', 20, 0, 30, '../assets/spec.pdf', 'spec', 0, 'file', 'pdf', 'assets/spec.pdf'),
			('projects/website', 'project', 'projects/website.md', 6, 4, 28, 'docs/brief.pdf', 'Brief', 0, 'file', 'pdf', 'projects/docs/brief.pdf'),
			('projects/website', 'project', 'projects/website.md', 25, 2, 37, 'assets/wireframe.png', 'Wireframe', 1, 'file', 'png', 'projects/assets/wireframe.png'),
			('projects/website', 'project', 'projects/website.md', 55, 8, 51, 'https://example.com/Guide.PDF', 'Guide', 0, 'url', 'pdf', 'https://example.com/Guide.PDF'),
			('projects/mobile', 'project', 'projects/mobile.md', 20, 1, 29, '../manual.pdf', 'Manual', 0, 'file', 'pdf', 'manual.pdf'),
			('people/freya', 'person', 'people/freya.md', 12, 3, 34, 'https://example.com', 'Website', 0, 'url', '', 'https://example.com');

		INSERT INTO assets (id, file_path, media_type, extension, filename, size_bytes, file_mtime, indexed_at) VALUES
			('assets/images/diagram.png', 'assets/images/diagram.png', 'image/png', 'png', 'diagram.png', 2048, 100, 200),
			('assets/pdfs/paper.pdf', 'assets/pdfs/paper.pdf', 'application/pdf', 'pdf', 'paper.pdf', 12345, 100, 200),
			('assets/raw/data.bin', 'assets/raw/data.bin', NULL, 'bin', 'data.bin', 99, 100, 200);

		INSERT INTO fts_content (object_id, title, content, file_path) VALUES
			('projects/website', 'Website Project', 'This is the website redesign project. Freya is a colleague working on this. Optional workflow input inputs.project is documented here.', 'projects/website.md'),
			('projects/mobile', 'Mobile App', 'Mobile application for customers. Currently paused.', 'projects/mobile.md'),
			('people/freya', 'Freya', 'Senior engineer and colleague. Works on platform team.', 'people/freya.md'),
			('people/loki', 'Loki', 'Contractor helping with security review.', 'people/loki.md'),
			('daily/2025-02-01', 'Daily Note', 'Morning standup and planning session.', 'daily/2025-02-01.md'),
			('daily/2025-02-01#standup', 'Standup', 'Weekly standup meeting discussion.', 'daily/2025-02-01.md'),
			('daily/2025-02-01#planning', 'Planning', 'Q2 planning session with the team.', 'daily/2025-02-01.md');
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	return db
}
func setupRefRegressionDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE objects (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			type TEXT NOT NULL,
			fields TEXT NOT NULL DEFAULT '{}',
			line_start INTEGER NOT NULL,
			created_at INTEGER,
			updated_at INTEGER
		);

		CREATE TABLE traits (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			parent_object_id TEXT NOT NULL,
			trait_type TEXT NOT NULL,
			value TEXT,
			content TEXT NOT NULL,
			line_number INTEGER NOT NULL,
			created_at INTEGER
		);

		CREATE TABLE sections (
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

		CREATE TABLE refs (
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

		CREATE TABLE field_refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			target_id TEXT,
			target_raw TEXT NOT NULL,
			resolution_status TEXT NOT NULL,
			file_path TEXT NOT NULL,
			line_number INTEGER
		);

		CREATE TABLE date_index (
			date TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			file_path TEXT NOT NULL,
			PRIMARY KEY (date, source_type, source_id, field_name)
		);

		CREATE VIRTUAL TABLE fts_content USING fts5(
			object_id,
			title,
			content,
			file_path UNINDEXED,
			tokenize='porter unicode61'
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('objects/project/raven', 'objects/project/raven.md', 'project', '{"name":"Raven"}', 1),
			('projects/website', 'projects/website.md', 'project', '{"name":"Website"}', 1),
			('daily/2026-02-14', 'daily/2026-02-14.md', 'date', '{}', 1),
			('daily/2026-02-15', 'daily/2026-02-15.md', 'date', '{}', 1);

		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('trait1', 'daily/2026-02-14.md', 'daily/2026-02-14', 'todo', 'todo', 'Investigate [[project/raven]]', 5),
			('trait2', 'daily/2026-02-15.md', 'daily/2026-02-15', 'todo', 'todo', 'Follow up on [[projects/website]]', 6);

		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('daily/2026-02-14', 'project/raven', 'project/raven', 'daily/2026-02-14.md', 5),
			('daily/2026-02-15', NULL, 'projects/website', 'daily/2026-02-15.md', 6);
	`)
	if err != nil {
		t.Fatalf("failed to insert regression data: %v", err)
	}

	return db
}
