package query

import (
	"database/sql"
	"strings"
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
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, parent_section_id) VALUES
			('projects/website#tasks', 'projects/website', 'projects/website.md', 'tasks', 'Tasks', 2, 20, NULL),
			('projects/website#design', 'projects/website', 'projects/website.md', 'design', 'Design', 2, 50, NULL),
			('projects/mobile#tasks', 'projects/mobile', 'projects/mobile.md', 'tasks', 'Tasks', 2, 15, NULL),
			('daily/2025-02-01#standup', 'daily/2025-02-01', 'daily/2025-02-01.md', 'standup', 'Standup', 2, 10, NULL),
			('daily/2025-02-01#planning', 'daily/2025-02-01', 'daily/2025-02-01.md', 'planning', 'Planning', 2, 30, NULL);

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

func TestExecuteObjectQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "simple type query",
			query:     "type:project",
			wantCount: 2,
		},
		{
			name:      "type with field filter",
			query:     "type:project .status==active",
			wantCount: 1,
		},
		{
			name:      "negated field filter",
			query:     "type:project !.status==active",
			wantCount: 1,
		},
		{
			name:      "field filter case insensitive",
			query:     "type:project .status==ACTIVE",
			wantCount: 1, // matches "active" case-insensitively
		},
		{
			name:      "field filter mixed case",
			query:     "type:project .status==Active",
			wantCount: 1, // matches "active" case-insensitively
		},
		{
			name:      "field exists with notnull",
			query:     "type:person exists(.email)",
			wantCount: 1,
		},
		{
			name:      "has trait",
			query:     "type:project has(trait:due)",
			wantCount: 1,
		},
		{
			name:      "date virtual field exact",
			query:     "type:date .date==2025-02-01",
			wantCount: 1,
		},
		{
			name:      "date virtual field range",
			query:     "type:date .date>=2025-01-01 .date<=2025-12-31",
			wantCount: 1,
		},
		{
			name:      "project refs person",
			query:     "type:project refs([[people/freya]])",
			wantCount: 1, // Website refs Freya
		},
		{
			name:      "project refs target through section",
			query:     "type:project refs([[projects/website]])",
			wantCount: 1, // Mobile refs website from its tasks section
		},
		{
			name:      "content search simple",
			query:     `type:person content("colleague")`,
			wantCount: 1, // Freya's page mentions "colleague"
		},
		{
			name:      "content search multiple words",
			query:     `type:project content("website redesign")`,
			wantCount: 1, // Website project has both words
		},
		{
			name:      "content search dotted token",
			query:     `type:project content("inputs.project")`,
			wantCount: 1,
		},
		{
			name:      "content search negated",
			query:     `type:person !content("contractor")`,
			wantCount: 1, // Freya doesn't mention contractor, Loki does
		},
		{
			name:      "content search no match",
			query:     `type:project content("nonexistent")`,
			wantCount: 0,
		},
		{
			name:      "content combined with field",
			query:     `type:project .status==active content("colleague")`,
			wantCount: 1, // Website is active and mentions colleague
		},
		// Section containment predicate tests
		{
			name:      "has section",
			query:     "type:project has(section)",
			wantCount: 2, // Both website and mobile have section children
		},
		{
			name:      "has section with title",
			query:     `type:project has(section .title==Tasks)`,
			wantCount: 2,
		},
		{
			name:      "negated has section",
			query:     "type:project !has(section)",
			wantCount: 0, // Both projects have sections
		},
		{
			name:      "date type repeat",
			query:     "type:date",
			wantCount: 1, // daily/2025-02-01 has meetings
		},
		// Contains predicate tests
		{
			name:      "contains todo trait",
			query:     "type:project contains(trait:todo)",
			wantCount: 2, // Both projects have todo traits in nested sections
		},
		{
			name:      "contains todo with value filter",
			query:     "type:project contains(trait:todo .value==todo)",
			wantCount: 2, // Both projects have incomplete todos (trait5 on website, trait8 on mobile)
		},
		{
			name:      "contains todo with content filter",
			query:     `type:project contains(trait:todo content("Build"))`,
			wantCount: 1, // Only website has a matching todo trait line
		},
		{
			name:      "contains todo with refs filter",
			query:     "type:project contains(trait:todo refs([[projects/website]]))",
			wantCount: 1, // Only mobile has a todo trait line that references website
		},
		{
			name:      "contains todo value done",
			query:     "type:project contains(trait:todo .value==done)",
			wantCount: 1, // Only mobile has completed todo
		},
		{
			name:      "contains priority high",
			query:     "type:project contains(trait:priority .value==high)",
			wantCount: 1, // Only website has high priority in subtree
		},
		{
			name:      "negated contains",
			query:     "type:project !contains(trait:todo)",
			wantCount: 0, // Both projects have todos
		},
		{
			name:      "date contains recursive section due",
			query:     "type:date contains(trait:due)",
			wantCount: 1,
		},
		{
			name:      "date contains recursive section highlight",
			query:     "type:date contains(trait:highlight)",
			wantCount: 1,
		},
		{
			name:      "project with direct has vs contains",
			query:     "type:project has(trait:due)",
			wantCount: 1, // Only website has due directly on project (not section)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}
}

func TestExecuteAssetQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name      string
		query     string
		wantIDs   []string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "all assets",
			query:     "asset",
			wantCount: 3,
		},
		{
			name:    "extension equality",
			query:   "asset .extension==pdf",
			wantIDs: []string{"assets/pdfs/paper.pdf"},
		},
		{
			name:    "media type prefix",
			query:   `asset startswith(.media_type, "image/")`,
			wantIDs: []string{"assets/images/diagram.png"},
		},
		{
			name:    "filename contains",
			query:   `asset includes(.filename, "paper")`,
			wantIDs: []string{"assets/pdfs/paper.pdf"},
		},
		{
			name:    "size comparison",
			query:   "asset .size_bytes>1024",
			wantIDs: []string{"assets/images/diagram.png", "assets/pdfs/paper.pdf"},
		},
		{
			name:    "referenced by direct object including sections",
			query:   "asset refd([[projects/website]])",
			wantIDs: []string{"assets/images/diagram.png", "assets/pdfs/paper.pdf"},
		},
		{
			name:    "referenced by object subquery",
			query:   "asset refd(type:project .status==active)",
			wantIDs: []string{"assets/images/diagram.png", "assets/pdfs/paper.pdf"},
		},
		{
			name:    "referenced by trait subquery",
			query:   "asset refd(trait:todo .value==todo)",
			wantIDs: []string{"assets/images/diagram.png"},
		},
		{
			name:    "negated refd",
			query:   "asset !refd([[projects/website]])",
			wantIDs: []string{"assets/raw/data.bin"},
		},
		{
			name:    "refs rejected at execution",
			query:   "asset refs([[projects/website]])",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeAssetQuery(q)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantIDs != nil {
				got := make([]string, 0, len(results))
				for _, r := range results {
					got = append(got, r.ID)
				}
				if strings.Join(got, ",") != strings.Join(tt.wantIDs, ",") {
					t.Fatalf("ids = %#v, want %#v", got, tt.wantIDs)
				}
			} else if len(results) != tt.wantCount {
				t.Fatalf("got %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestExecuteAssetIDAndCountQueries(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)
	q, err := Parse("asset .size_bytes>1000")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	ids, err := executor.executeAssetIDQuery(q, 1, 1)
	if err != nil {
		t.Fatalf("unexpected ID query error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "assets/pdfs/paper.pdf" {
		t.Fatalf("ids = %#v, want assets/pdfs/paper.pdf", ids)
	}

	count, err := executor.executeAssetCountQuery(q)
	if err != nil {
		t.Fatalf("unexpected count query error: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestExecuteTraitQuery_MatchesDirectRefsAcrossRootVariants(t *testing.T) {
	t.Parallel()
	db := setupRefRegressionDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	q, err := Parse("trait:todo .value==todo refs([[project/raven]])")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	results, err := executor.executeTraitQuery(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].FilePath != "daily/2026-02-14.md" || results[0].Line != 5 {
		t.Fatalf("unexpected trait match: %+v", results[0])
	}
}

func TestExecuteObjectQuery_HasAppliesNestedTraitPredicates(t *testing.T) {
	t.Parallel()
	db := setupRefRegressionDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	q, err := Parse("type:date has(trait:todo refs([[projects/website]]))")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	results, err := executor.executeObjectQuery(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID != "daily/2026-02-15" {
		t.Fatalf("unexpected object match: %+v", results[0])
	}
}

func TestExecuteTraitQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "simple trait query",
			query:     "trait:due",
			wantCount: 3,
		},
		{
			name:      "trait with value filter",
			query:     "trait:due .value==2025-06-30",
			wantCount: 1,
		},
		{
			name:      "trait value case insensitive",
			query:     "trait:todo .value==TODO",
			wantCount: 2, // matches "todo" case-insensitively (trait5 and trait8)
		},
		{
			name:      "trait value mixed case",
			query:     "trait:priority .value==HIGH",
			wantCount: 1, // matches "high" case-insensitively
		},
		{
			name:      "trait string function on value",
			query:     `trait:todo includes(.value, "to")`,
			wantCount: 2, // matches todo values on trait5 and trait8
		},
		{
			name:      "trait array any equality",
			query:     `trait:tags any(.value, _ == skills)`,
			wantCount: 1,
		},
		{
			name:      "trait array any string function",
			query:     `trait:tags any(.value, startswith(_, "io"))`,
			wantCount: 1,
		},
		{
			name:      "trait array all inequality",
			query:     `trait:tags all(.value, _ != mobile)`,
			wantCount: 1,
		},
		{
			name:      "trait array none equality",
			query:     `trait:tags none(.value, _ == ios)`,
			wantCount: 1,
		},
		{
			name:      "trait ref array any wikilink",
			query:     `trait:reviewers any(.value, _ == [[people/freya]])`,
			wantCount: 1,
		},
		{
			name:    "trait string function on unsupported field",
			query:   `trait:todo includes(.content, "landing")`,
			wantErr: true,
		},
		{
			name:      "highlight traits",
			query:     "trait:highlight",
			wantCount: 1,
		},
		{
			name:      "in section title",
			query:     "trait:due in(section .title==Standup)",
			wantCount: 1,
		},
		{
			name:      "in project",
			query:     "trait:due in(type:project)",
			wantCount: 1,
		},
		{
			name:      "within date virtual field",
			query:     "trait:due within(type:date .date==2025-02-01)",
			wantCount: 1,
		},
		{
			name:      "refs to specific person",
			query:     "trait:due refs([[people/freya]])",
			wantCount: 1, // trait2 on line 15 has a ref to freya on the same line
		},
		{
			name:      "refs to specific person with .md suffix",
			query:     "trait:due refs([[people/freya.md]])",
			wantCount: 1,
		},
		{
			name:      "refs with type subquery",
			query:     "trait:due refs(type:person)",
			wantCount: 1, // trait2 refs a person on the same line
		},
		{
			name:      "negated refs",
			query:     "trait:due !refs([[people/freya]])",
			wantCount: 2, // trait1 and trait4 don't have freya refs on same line
		},
		{
			name:      "refs to non-existent target",
			query:     "trait:due refs([[people/thor]])",
			wantCount: 0, // No trait has refs to thor on same line
		},
		// Tests for unresolved refs (target_id is NULL, fallback to target_raw)
		{
			name:      "refs with NULL target_id (unresolved) using direct ref",
			query:     "trait:todo refs([[projects/website]])",
			wantCount: 1, // trait8 has unresolved ref to projects/website on line 30
		},
		{
			name:      "refs with NULL target_id (unresolved) using type subquery",
			query:     "trait:todo refs(type:project)",
			wantCount: 1, // trait8 has unresolved ref to a project on line 30
		},
		// Content predicate tests
		{
			name:      "content search simple",
			query:     `trait:due content("Follow up")`,
			wantCount: 1, // trait2 has "Follow up on timeline"
		},
		{
			name:      "content search case insensitive",
			query:     `trait:due content("follow UP")`,
			wantCount: 1, // SQLite LIKE is case-insensitive by default
		},
		{
			name:      "content search no match",
			query:     `trait:due content("nonexistent")`,
			wantCount: 0,
		},
		{
			name:      "content search negated",
			query:     `trait:due !content("Follow up")`,
			wantCount: 2, // trait1 and trait4 don't have "Follow up"
		},
		{
			name:      "content combined with value",
			query:     `trait:todo content("landing page") .value==todo`,
			wantCount: 1, // trait5 has "Build landing page" with .value==todo
		},
		{
			name:      "content combined with in",
			query:     `trait:highlight content("Important") in(section .title==Standup)`,
			wantCount: 1, // trait3 has "Important insight" in the Standup section
		},
		{
			name:      "content search highlight",
			query:     `trait:highlight content("insight")`,
			wantCount: 1, // trait3 has "Important insight"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeTraitQuery(q)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentObjectID)
				}
			}
		})
	}
}

func TestDirectTargetPredicates(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Type query tests with direct and recursive section containment.
	objectTests := []struct {
		name      string
		query     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "has tasks section",
			query:     "type:project has(section .title==Tasks)",
			wantCount: 2,
		},
		{
			name:      "has design section",
			query:     "type:project has(section .title==Design)",
			wantCount: 1,
		},
		{
			name:      "contains tasks section",
			query:     "type:project contains(section .title==Tasks)",
			wantCount: 2,
		},
		{
			name:      "contains todo trait",
			query:     "type:project contains(trait:todo)",
			wantCount: 2,
		},
		{
			name:      "negated has design section",
			query:     "type:project !has(section .title==Design)",
			wantCount: 1,
		},
		{
			name:      "missing section title returns nothing",
			query:     "type:project has(section .title==Missing)",
			wantCount: 0,
		},
	}

	for _, tt := range objectTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}

	// Trait query tests with [[target]] predicates
	traitTests := []struct {
		name      string
		query     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "in with direct target",
			query:     "trait:todo in([[projects/website#tasks]])",
			wantCount: 1, // trait5 on website#tasks
		},
		{
			name:      "within with direct target",
			query:     "trait:todo within([[projects/website]])",
			wantCount: 1, // trait5 is within website (on website#tasks)
		},
		{
			name:      "within with short reference",
			query:     "trait:todo within([[website]])",
			wantCount: 1, // trait5 is within website
		},
		{
			name:      "in non-existent target returns nothing",
			query:     "trait:todo in([[nonexistent]])",
			wantCount: 0,
		},
		{
			name:      "negated in target",
			query:     "trait:todo !in([[projects/website#tasks]])",
			wantCount: 2, // mobile#tasks has two todos (trait7 and trait8)
		},
	}

	for _, tt := range traitTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeTraitQuery(q)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentObjectID)
				}
			}
		})
	}
}

func TestOrAndGroupPredicates(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Type query tests with OR and groups
	objectTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "AND binds tighter than OR",
			query:     "type:project .status==active .priority==high | .status==paused",
			wantCount: 2, // (active AND high) OR paused
		},
		{
			name:      "OR field values",
			query:     "type:project (.status==active | .status==paused)",
			wantCount: 2, // website (active) and mobile (paused)
		},
		{
			name:      "OR with one match",
			query:     "type:project (.status==active | .status==nonexistent)",
			wantCount: 1, // website only
		},
		{
			name:      "grouped AND with field",
			query:     "type:project (.status==active) .priority==high",
			wantCount: 1, // website has both
		},
		{
			name:      "negated OR",
			query:     "type:project !(.status==active | .status==paused)",
			wantCount: 0, // both projects match the OR, so negation returns none
		},
		{
			name:      "OR priority values",
			query:     "type:project (.priority==high | .priority==medium)",
			wantCount: 2,
		},
		{
			name:      "complex: OR with has",
			query:     "type:project (has(trait:due) | has(trait:todo))",
			wantCount: 1, // website has due directly
		},
	}

	for _, tt := range objectTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}

	// Trait query tests with OR and groups
	traitTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "OR on object types",
			query:     "trait:due (in(type:project) | in(type:person))",
			wantCount: 2, // trait1 on project, trait4 on person
		},
		{
			name:      "OR value filter",
			query:     "trait:todo (.value==todo | .value==done)",
			wantCount: 3, // trait5 (todo), trait7 (done), trait8 (todo)
		},
		{
			name:      "grouped with value",
			query:     "trait:todo (.value==todo) in(section)",
			wantCount: 2, // trait5 and trait8 (both have .value==todo and are on sections)
		},
	}

	for _, tt := range traitTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeTraitQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentObjectID)
				}
			}
		})
	}
}

func TestBooleanEdgeCasesExecution(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "chained OR: A | B | C",
			query:     "type:project (.status==active | .status==paused | .status==nonexistent)",
			wantCount: 2, // website (active) + mobile (paused)
		},
		{
			name:      "AND of two OR groups",
			query:     "type:project (.status==active | .status==paused) (.priority==high | .priority==medium)",
			wantCount: 2, // website (active+high), mobile (paused+medium)
		},
		{
			name:      "negated OR via NotPredicate",
			query:     "type:project !(.status==active | .status==paused)",
			wantCount: 0, // all match the OR
		},
		{
			name:      "in() as flat OR",
			query:     "type:project oneof(.status, [active,paused])",
			wantCount: 2,
		},
		{
			name:      "negated in()",
			query:     "type:project !oneof(.status, [active,paused])",
			wantCount: 0, // all match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}
}

func TestAtPredicate(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	// Add some co-located traits for testing at:
	_, err := db.Exec(`
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('colocated1', 'projects/website.md', 'projects/website#tasks', 'due', '2025-03-15', 'Build landing page @due(2025-03-15) @priority(high)', 25),
			('colocated2', 'projects/mobile.md', 'projects/mobile#tasks', 'remind', '2025-03-01', 'Setup CI/CD @remind(2025-03-01)', 20);
	`)
	if err != nil {
		t.Fatalf("failed to insert co-located test data: %v", err)
	}

	executor := NewExecutor(db)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "at with co-located trait",
			query:     "trait:due at(trait:priority)",
			wantCount: 1, // colocated1 is on same line as priority
		},
		{
			name:      "at with co-located todo",
			query:     "trait:priority at(trait:todo)",
			wantCount: 1, // trait6 is on same line as trait5 (todo)
		},
		{
			name:      "at no match",
			query:     "trait:remind at(trait:priority)",
			wantCount: 0, // remind trait has no co-located priority
		},
		{
			name:      "negated at",
			query:     "trait:due !at(trait:priority)",
			wantCount: 3, // trait1, trait2, trait4 don't have co-located priority
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeTraitQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s: %s (line: %d)", r.TraitType, r.Content, r.Line)
				}
			}
		})
	}
}

func TestRefdPredicate(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "refd by specific source",
			query:     "type:project refd([[daily/2025-02-01#standup]])",
			wantCount: 1, // website is referenced by standup
		},
		{
			name:      "refd by short reference source",
			query:     "type:project refd([[standup]])",
			wantCount: 1, // website is referenced by standup (via resolver)
		},
		{
			name:      "refd by sections",
			query:     "type:project refd(section)",
			wantCount: 2, // website referenced by standup, mobile by planning
		},
		{
			name:      "person refd by sections",
			query:     "type:person refd(section)",
			wantCount: 1, // freya is referenced by both daily sections
		},
		{
			name:      "person refd by project",
			query:     "type:person refd(type:project)",
			wantCount: 1, // freya is referenced by website
		},
		{
			name:      "negated refd",
			query:     "type:person !refd(section)",
			wantCount: 1, // loki is not referenced by any section
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}
}

func TestComparisonOperators(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Test value comparison operators
	traitTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "value less than",
			query:     "trait:due .value<2025-03-01",
			wantCount: 2, // trait2 (2025-02-03) and trait4 (2025-02-01)
		},
		{
			name:      "value greater than",
			query:     "trait:due .value>2025-03-01",
			wantCount: 1, // trait1 (2025-06-30)
		},
		{
			name:      "value less than or equal",
			query:     "trait:due .value<=2025-02-03",
			wantCount: 2, // trait2 and trait4
		},
		{
			name:      "value greater than or equal",
			query:     "trait:due .value>=2025-02-03",
			wantCount: 2, // trait1 and trait2
		},
	}

	for _, tt := range traitTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeTraitQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s: %v", r.TraitType, r.Value)
				}
			}
		})
	}
}

func TestRefdShorthand(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Test refd with section subquery.
	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "refd section subquery",
			query:     "type:project refd(section)",
			wantCount: 2, // website referenced by standup, mobile by planning
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}
}

func TestHierarchyPredicatesWithSubqueries(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Type query hierarchy tests
	objectTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "has section with field filter",
			query:     "type:project has(section .title==Tasks)",
			wantCount: 2,
		},
		{
			name:      "negated contains section",
			query:     "type:project !contains(section .title==Tasks)",
			wantCount: 0,
		},
		{
			name:      "has section with trait",
			query:     "type:project has(section has(trait:todo))",
			wantCount: 2,
		},
		{
			name:      "has section with no match",
			query:     "type:project has(section has(trait:due))",
			wantCount: 0,
		},
		{
			name:      "contains section with field filter",
			query:     "type:project contains(section .title==Tasks)",
			wantCount: 2,
		},
		{
			name:      "contains trait with section predicate",
			query:     "type:project contains(trait:todo in(section .title==Tasks))",
			wantCount: 2,
		},
		{
			name:      "has section under active project",
			query:     "type:project .status==active has(section .title==Design)",
			wantCount: 1,
		},
		{
			name:      "has section under due project",
			query:     "type:project has(trait:due) has(section .title==Tasks)",
			wantCount: 1,
		},
		{
			name:      "missing section under project",
			query:     "type:project has(section .title==Missing)",
			wantCount: 0,
		},
	}

	for _, tt := range objectTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeObjectQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s (%s)", r.ID, r.Type)
				}
			}
		})
	}

	// Trait query hierarchy tests
	traitTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		// Within with subquery predicates
		{
			name:      "within with field filter",
			query:     "trait:todo within(type:project .status==active)",
			wantCount: 1, // trait5 is within website (active)
		},
		{
			name:      "within with has predicate",
			query:     "trait:todo within(type:project has(trait:due))",
			wantCount: 1, // website has due, trait5 is within it
		},
		{
			name:      "on with field filter",
			query:     "trait:todo in(section .title==Tasks)",
			wantCount: 3, // trait5 on website#tasks, trait7 and trait8 on mobile#tasks
		},
		{
			name:      "within paused project",
			query:     "trait:todo within(type:project .status==paused)",
			wantCount: 2, // trait7 and trait8 are within mobile (paused)
		},
		{
			name:      "highlight within date",
			query:     "trait:highlight within(type:date)",
			wantCount: 1,
		},
	}

	for _, tt := range traitTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.executeTraitQuery(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
				for _, r := range results {
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentObjectID)
				}
			}
		})
	}
}
