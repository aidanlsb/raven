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
			heading TEXT,
			heading_level INTEGER,
			fields TEXT NOT NULL DEFAULT '{}',
			line_start INTEGER NOT NULL,
			line_end INTEGER,
			parent_id TEXT,
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

		CREATE TABLE refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			target_raw TEXT NOT NULL,
			display_text TEXT,
			file_path TEXT NOT NULL,
			line_number INTEGER,
			position_start INTEGER,
			position_end INTEGER
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
			('daily/2025-02-01', 'daily/2025-02-01.md', 'date', '{}', 1),
			('daily/2025-02-01#standup', 'daily/2025-02-01.md', 'meeting', '{"time":"09:00"}', 10),
			('daily/2025-02-01#planning', 'daily/2025-02-01.md', 'meeting', '{"time":"14:00"}', 30);
		
		UPDATE objects SET parent_id = 'daily/2025-02-01' WHERE id = 'daily/2025-02-01#standup';
		UPDATE objects SET parent_id = 'daily/2025-02-01' WHERE id = 'daily/2025-02-01#planning';

		-- Add deeper hierarchy for descendant/contains tests
		INSERT INTO objects (id, file_path, type, fields, line_start, parent_id) VALUES
			('projects/website#tasks', 'projects/website.md', 'section', '{"title":"Tasks"}', 20, 'projects/website'),
			('projects/website#design', 'projects/website.md', 'section', '{"title":"Design"}', 50, 'projects/website'),
			('projects/mobile#tasks', 'projects/mobile.md', 'section', '{"title":"Tasks"}', 15, 'projects/mobile');

		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('trait1', 'projects/website.md', 'projects/website', 'due', '2025-06-30', 'projects/website', 1),
			('trait2', 'daily/2025-02-01.md', 'daily/2025-02-01#standup', 'due', '2025-02-03', 'Follow up on timeline', 15),
			('trait3', 'daily/2025-02-01.md', 'daily/2025-02-01#standup', 'highlight', NULL, 'Important insight', 18),
			('trait4', 'people/freya.md', 'people/freya', 'due', '2025-02-01', 'Send docs', 12),
			-- Traits on nested sections for contains tests
			('trait5', 'projects/website.md', 'projects/website#tasks', 'todo', 'todo', 'Build landing page', 25),
			('trait6', 'projects/website.md', 'projects/website#tasks', 'priority', 'high', 'Build landing page', 25),
			('trait7', 'projects/mobile.md', 'projects/mobile#tasks', 'todo', 'done', 'Setup CI/CD', 20);

		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('daily/2025-02-01#standup', 'projects/website', 'projects/website', 'daily/2025-02-01.md', 12),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 13),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 15),
			('daily/2025-02-01#planning', 'projects/mobile', 'projects/mobile', 'daily/2025-02-01.md', 32),
			('daily/2025-02-01#planning', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 33),
			('projects/website', 'people/freya', 'people/freya', 'projects/website.md', 5);

		INSERT INTO fts_content (object_id, title, content, file_path) VALUES
			('projects/website', 'Website Project', 'This is the website redesign project. Freya is a colleague working on this.', 'projects/website.md'),
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

func TestExecuteObjectQuery(t *testing.T) {
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
			query:     "object:project",
			wantCount: 2,
		},
		{
			name:      "type with field filter",
			query:     "object:project .status:active",
			wantCount: 1,
		},
		{
			name:      "negated field filter",
			query:     "object:project !.status:active",
			wantCount: 1,
		},
		{
			name:      "field exists",
			query:     "object:person .email:*",
			wantCount: 1,
		},
		{
			name:      "has trait",
			query:     "object:project has:due",
			wantCount: 1,
		},
		{
			name:      "meeting type",
			query:     "object:meeting",
			wantCount: 2, // standup and planning
		},
		{
			name:      "meeting has due",
			query:     "object:meeting has:due",
			wantCount: 1,
		},
		{
			name:      "parent type",
			query:     "object:meeting parent:date",
			wantCount: 2, // Both standup and planning
		},
		{
			name:      "refs to specific target",
			query:     "object:meeting refs:[[projects/website]]",
			wantCount: 1, // Only standup refs website
		},
		{
			name:      "refs to person",
			query:     "object:meeting refs:[[people/freya]]",
			wantCount: 2, // Both meetings ref Freya
		},
		{
			name:      "refs with subquery",
			query:     "object:meeting refs:{object:project .status:active}",
			wantCount: 1, // Only standup refs active project (website)
		},
		{
			name:      "negated refs",
			query:     "object:meeting !refs:[[projects/website]]",
			wantCount: 1, // Planning doesn't ref website
		},
		{
			name:      "project refs person",
			query:     "object:project refs:[[people/freya]]",
			wantCount: 1, // Website refs Freya
		},
		{
			name:      "content search simple",
			query:     `object:person content:"colleague"`,
			wantCount: 1, // Freya's page mentions "colleague"
		},
		{
			name:      "content search multiple words",
			query:     `object:project content:"website redesign"`,
			wantCount: 1, // Website project has both words
		},
		{
			name:      "content search negated",
			query:     `object:person !content:"contractor"`,
			wantCount: 1, // Freya doesn't mention contractor, Loki does
		},
		{
			name:      "content search no match",
			query:     `object:project content:"nonexistent"`,
			wantCount: 0,
		},
		{
			name:      "content combined with field",
			query:     `object:project .status:active content:"colleague"`,
			wantCount: 1, // Website is active and mentions colleague
		},
		// Descendant predicate tests
		{
			name:      "descendant section",
			query:     "object:project descendant:section",
			wantCount: 2, // Both website and mobile have section children
		},
		{
			name:      "descendant section with title",
			query:     `object:project descendant:{object:section}`,
			wantCount: 2,
		},
		{
			name:      "negated descendant",
			query:     "object:project !descendant:section",
			wantCount: 0, // Both projects have sections
		},
		{
			name:      "date has descendant meeting",
			query:     "object:date descendant:meeting",
			wantCount: 1, // daily/2025-02-01 has meetings
		},
		// Contains predicate tests
		{
			name:      "contains todo trait",
			query:     "object:project contains:{trait:todo}",
			wantCount: 2, // Both projects have todo traits in nested sections
		},
		{
			name:      "contains todo with value filter",
			query:     "object:project contains:{trait:todo value:todo}",
			wantCount: 1, // Only website has incomplete todo
		},
		{
			name:      "contains todo value done",
			query:     "object:project contains:{trait:todo value:done}",
			wantCount: 1, // Only mobile has completed todo
		},
		{
			name:      "contains priority high",
			query:     "object:project contains:{trait:priority value:high}",
			wantCount: 1, // Only website has high priority in subtree
		},
		{
			name:      "negated contains",
			query:     "object:project !contains:{trait:todo}",
			wantCount: 0, // Both projects have todos
		},
		{
			name:      "contains on date (direct trait on child)",
			query:     "object:date contains:{trait:due}",
			wantCount: 1, // daily note has due trait on its child meeting
		},
		{
			name:      "contains highlight",
			query:     "object:date contains:{trait:highlight}",
			wantCount: 1, // daily note has highlight on its meeting child
		},
		{
			name:      "project with direct has vs contains",
			query:     "object:project has:{trait:due}",
			wantCount: 1, // Only website has due directly on project (not section)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.ExecuteObjectQuery(q)
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

func TestExecuteTraitQuery(t *testing.T) {
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
			query:     "trait:due value:2025-06-30",
			wantCount: 1,
		},
		{
			name:      "highlight traits",
			query:     "trait:highlight",
			wantCount: 1,
		},
		{
			name:      "on object type",
			query:     "trait:due on:meeting",
			wantCount: 1,
		},
		{
			name:      "on project",
			query:     "trait:due on:project",
			wantCount: 1,
		},
		{
			name:      "refs to specific person",
			query:     "trait:due refs:[[people/freya]]",
			wantCount: 1, // trait2 on line 15 has a ref to freya on the same line
		},
		{
			name:      "refs with object subquery",
			query:     "trait:due refs:{object:person}",
			wantCount: 1, // trait2 refs a person on the same line
		},
		{
			name:      "negated refs",
			query:     "trait:due !refs:[[people/freya]]",
			wantCount: 2, // trait1 and trait4 don't have freya refs on same line
		},
		{
			name:      "refs to non-existent target",
			query:     "trait:due refs:[[people/thor]]",
			wantCount: 0, // No trait has refs to thor on same line
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.ExecuteTraitQuery(q)
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
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Object query tests with [[target]] predicates
	objectTests := []struct {
		name      string
		query     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "parent with direct target",
			query:     "object:section parent:[[projects/website]]",
			wantCount: 2, // website#tasks and website#design
		},
		{
			name:      "parent with short reference",
			query:     "object:section parent:[[website]]",
			wantCount: 2, // website#tasks and website#design
		},
		{
			name:      "ancestor with direct target",
			query:     "object:meeting ancestor:[[daily/2025-02-01]]",
			wantCount: 2, // standup and planning
		},
		{
			name:      "child with direct target",
			query:     "object:date child:[[daily/2025-02-01#standup]]",
			wantCount: 1, // daily/2025-02-01
		},
		{
			name:      "descendant with direct target",
			query:     "object:project descendant:[[projects/website#tasks]]",
			wantCount: 1, // projects/website
		},
		{
			name:      "negated parent target",
			query:     "object:section !parent:[[projects/website]]",
			wantCount: 1, // mobile#tasks
		},
		{
			name:      "non-existent target returns nothing",
			query:     "object:section parent:[[nonexistent]]",
			wantCount: 0,
		},
	}

	for _, tt := range objectTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.ExecuteObjectQuery(q)
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
			name:      "on with direct target",
			query:     "trait:todo on:[[projects/website#tasks]]",
			wantCount: 1, // trait5 on website#tasks
		},
		{
			name:      "within with direct target",
			query:     "trait:todo within:[[projects/website]]",
			wantCount: 1, // trait5 is within website (on website#tasks)
		},
		{
			name:      "within with short reference",
			query:     "trait:todo within:[[website]]",
			wantCount: 1, // trait5 is within website
		},
		{
			name:      "on non-existent target returns nothing",
			query:     "trait:todo on:[[nonexistent]]",
			wantCount: 0,
		},
		{
			name:      "negated on target",
			query:     "trait:todo !on:[[projects/website#tasks]]",
			wantCount: 1, // mobile#tasks has a todo
		},
	}

	for _, tt := range traitTests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			results, err := executor.ExecuteTraitQuery(q)
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
