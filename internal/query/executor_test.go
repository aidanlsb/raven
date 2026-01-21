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
			target_id TEXT,
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
			('daily/2025-02-01#standup', 'daily/2025-02-01.md', 'meeting', '{"time":"09:00","attendees":["people/freya","people/loki"]}', 10),
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
			('trait7', 'projects/mobile.md', 'projects/mobile#tasks', 'todo', 'done', 'Setup CI/CD', 20),
			-- Test case for unresolved refs (target_id is NULL)
			('trait8', 'projects/mobile.md', 'projects/mobile#tasks', 'todo', 'todo', 'Cross-project task [[projects/website]]', 30);

		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('daily/2025-02-01#standup', 'projects/website', 'projects/website', 'daily/2025-02-01.md', 12),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 13),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 15),
			('daily/2025-02-01#planning', 'projects/mobile', 'projects/mobile', 'daily/2025-02-01.md', 32),
			('daily/2025-02-01#planning', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 33),
			('projects/website', 'people/freya', 'people/freya', 'projects/website.md', 5),
			-- Unresolved ref (target_id is NULL) - tests fallback to target_raw matching
			('projects/mobile#tasks', NULL, 'projects/website', 'projects/mobile.md', 30);

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
			query:     "object:project .status==active",
			wantCount: 1,
		},
		{
			name:      "array field membership (ref token)",
			query:     "object:meeting .attendees==[[people/freya]]",
			wantCount: 1, // standup has attendees including freya
		},
		{
			name:      "negated field filter",
			query:     "object:project !.status==active",
			wantCount: 1,
		},
		{
			name:      "field filter case insensitive",
			query:     "object:project .status==ACTIVE",
			wantCount: 1, // matches "active" case-insensitively
		},
		{
			name:      "field filter mixed case",
			query:     "object:project .status==Active",
			wantCount: 1, // matches "active" case-insensitively
		},
		{
			name:      "field exists with notnull",
			query:     "object:person notnull(.email)",
			wantCount: 1,
		},
		{
			name:      "has trait",
			query:     "object:project has:{trait:due}",
			wantCount: 1,
		},
		{
			name:      "meeting type",
			query:     "object:meeting",
			wantCount: 2, // standup and planning
		},
		{
			name:      "meeting has due",
			query:     "object:meeting has:{trait:due}",
			wantCount: 1,
		},
		{
			name:      "parent type",
			query:     "object:meeting parent:{object:date}",
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
			query:     "object:meeting refs:{object:project .status==active}",
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
			query:     `object:project .status==active content:"colleague"`,
			wantCount: 1, // Website is active and mentions colleague
		},
		// Descendant predicate tests
		{
			name:      "descendant section",
			query:     "object:project descendant:{object:section}",
			wantCount: 2, // Both website and mobile have section children
		},
		{
			name:      "descendant section with title",
			query:     `object:project descendant:{object:section}`,
			wantCount: 2,
		},
		{
			name:      "negated descendant",
			query:     "object:project !descendant:{object:section}",
			wantCount: 0, // Both projects have sections
		},
		{
			name:      "date has descendant meeting",
			query:     "object:date descendant:{object:meeting}",
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
			query:     "object:project contains:{trait:todo value==todo}",
			wantCount: 2, // Both projects have incomplete todos (trait5 on website, trait8 on mobile)
		},
		{
			name:      "contains todo value done",
			query:     "object:project contains:{trait:todo value==done}",
			wantCount: 1, // Only mobile has completed todo
		},
		{
			name:      "contains priority high",
			query:     "object:project contains:{trait:priority value==high}",
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
			query:     "trait:due value==2025-06-30",
			wantCount: 1,
		},
		{
			name:      "trait value case insensitive",
			query:     "trait:todo value==TODO",
			wantCount: 2, // matches "todo" case-insensitively (trait5 and trait8)
		},
		{
			name:      "trait value mixed case",
			query:     "trait:priority value==HIGH",
			wantCount: 1, // matches "high" case-insensitively
		},
		{
			name:      "highlight traits",
			query:     "trait:highlight",
			wantCount: 1,
		},
		{
			name:      "on object type",
			query:     "trait:due on:{object:meeting}",
			wantCount: 1,
		},
		{
			name:      "on project",
			query:     "trait:due on:{object:project}",
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
		// Tests for unresolved refs (target_id is NULL, fallback to target_raw)
		{
			name:      "refs with NULL target_id (unresolved) using direct ref",
			query:     "trait:todo refs:[[projects/website]]",
			wantCount: 1, // trait8 has unresolved ref to projects/website on line 30
		},
		{
			name:      "refs with NULL target_id (unresolved) using object subquery",
			query:     "trait:todo refs:{object:project}",
			wantCount: 1, // trait8 has unresolved ref to a project on line 30
		},
		// Content predicate tests
		{
			name:      "content search simple",
			query:     `trait:due content:"Follow up"`,
			wantCount: 1, // trait2 has "Follow up on timeline"
		},
		{
			name:      "content search case insensitive",
			query:     `trait:due content:"follow UP"`,
			wantCount: 1, // SQLite LIKE is case-insensitive by default
		},
		{
			name:      "content search no match",
			query:     `trait:due content:"nonexistent"`,
			wantCount: 0,
		},
		{
			name:      "content search negated",
			query:     `trait:due !content:"Follow up"`,
			wantCount: 2, // trait1 and trait4 don't have "Follow up"
		},
		{
			name:      "content combined with value",
			query:     `trait:todo content:"landing page" value==todo`,
			wantCount: 1, // trait5 has "Build landing page" with value==todo
		},
		{
			name:      "content combined with on",
			query:     `trait:highlight content:"Important" on:{object:meeting}`,
			wantCount: 1, // trait3 has "Important insight" on a meeting
		},
		{
			name:      "content search highlight",
			query:     `trait:highlight content:"insight"`,
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
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Object query tests with OR and groups
	objectTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "AND binds tighter than OR",
			query:     "object:project .status==active .priority==high | .status==paused",
			wantCount: 2, // (active AND high) OR paused
		},
		{
			name:      "OR field values",
			query:     "object:project (.status==active | .status==paused)",
			wantCount: 2, // website (active) and mobile (paused)
		},
		{
			name:      "OR with one match",
			query:     "object:project (.status==active | .status==nonexistent)",
			wantCount: 1, // website only
		},
		{
			name:      "grouped AND with field",
			query:     "object:project (.status==active) .priority==high",
			wantCount: 1, // website has both
		},
		{
			name:      "negated OR",
			query:     "object:project !(.status==active | .status==paused)",
			wantCount: 0, // both projects match the OR, so negation returns none
		},
		{
			name:      "OR priority values",
			query:     "object:project (.priority==high | .priority==medium)",
			wantCount: 2,
		},
		{
			name:      "complex: OR with has",
			query:     "object:project (has:{trait:due} | has:{trait:todo})",
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
			query:     "trait:due (on:{object:project} | on:{object:person})",
			wantCount: 2, // trait1 on project, trait4 on person
		},
		{
			name:      "OR value filter",
			query:     "trait:todo (value==todo | value==done)",
			wantCount: 3, // trait5 (todo), trait7 (done), trait8 (todo)
		},
		{
			name:      "grouped with value",
			query:     "trait:todo (value==todo) on:{object:section}",
			wantCount: 2, // trait5 and trait8 (both have value==todo and are on sections)
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

func TestAtPredicate(t *testing.T) {
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
			query:     "trait:due at:{trait:priority}",
			wantCount: 1, // colocated1 is on same line as priority
		},
		{
			name:      "at with co-located todo",
			query:     "trait:priority at:{trait:todo}",
			wantCount: 1, // trait6 is on same line as trait5 (todo)
		},
		{
			name:      "at no match",
			query:     "trait:remind at:{trait:priority}",
			wantCount: 0, // remind trait has no co-located priority
		},
		{
			name:      "negated at",
			query:     "trait:due !at:{trait:priority}",
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
			query:     "object:project refd:[[daily/2025-02-01#standup]]",
			wantCount: 1, // website is referenced by standup
		},
		{
			name:      "refd by meeting type",
			query:     "object:project refd:{object:meeting}",
			wantCount: 2, // website referenced by standup, mobile by planning
		},
		{
			name:      "person refd by meeting",
			query:     "object:person refd:{object:meeting}",
			wantCount: 1, // freya is referenced by both meetings
		},
		{
			name:      "person refd by project",
			query:     "object:person refd:{object:project}",
			wantCount: 1, // freya is referenced by website
		},
		{
			name:      "negated refd",
			query:     "object:person !refd:{object:meeting}",
			wantCount: 1, // loki is not referenced by any meeting
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
			query:     "trait:due value<2025-03-01",
			wantCount: 2, // trait2 (2025-02-03) and trait4 (2025-02-01)
		},
		{
			name:      "value greater than",
			query:     "trait:due value>2025-03-01",
			wantCount: 1, // trait1 (2025-06-30)
		},
		{
			name:      "value less than or equal",
			query:     "trait:due value<=2025-02-03",
			wantCount: 2, // trait2 and trait4
		},
		{
			name:      "value greater than or equal",
			query:     "trait:due value>=2025-02-03",
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
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Test refd shorthand (should expand to object subquery)
	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "refd shorthand",
			query:     "object:project refd:{object:meeting}",
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
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Object query hierarchy tests
	objectTests := []struct {
		name      string
		query     string
		wantCount int
	}{
		// Ancestor with subquery predicates
		{
			name:      "ancestor with field filter",
			query:     "object:meeting ancestor:{object:date}",
			wantCount: 2, // standup and planning are under date
		},
		{
			name:      "negated ancestor",
			query:     "object:section !ancestor:{object:project}",
			wantCount: 0, // all sections have project ancestors
		},
		// Child with subquery predicates
		{
			name:      "child with type",
			query:     "object:date child:{object:meeting}",
			wantCount: 1, // daily/2025-02-01 has meeting children
		},
		{
			name:      "child with has predicate",
			query:     "object:date child:{object:meeting has:{trait:due}}",
			wantCount: 1, // standup has due
		},
		{
			name:      "child no match",
			query:     "object:project child:{object:meeting}",
			wantCount: 0, // projects don't have meetings as children
		},
		// Descendant with subquery predicates
		{
			name:      "descendant with field filter",
			query:     "object:project descendant:{object:section .title==Tasks}",
			wantCount: 2, // both projects have Tasks sections
		},
		{
			name:      "descendant meeting in date",
			query:     "object:date descendant:{object:meeting has:{trait:highlight}}",
			wantCount: 1, // standup has highlight
		},
		// Parent with subquery predicates
		{
			name:      "parent with field filter",
			query:     "object:section parent:{object:project .status==active}",
			wantCount: 2, // website#tasks and website#design
		},
		{
			name:      "parent with has predicate",
			query:     "object:section parent:{object:project has:{trait:due}}",
			wantCount: 2, // website has due, so its sections match
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
			query:     "trait:todo within:{object:project .status==active}",
			wantCount: 1, // trait5 is within website (active)
		},
		{
			name:      "within with has predicate",
			query:     "trait:todo within:{object:project has:{trait:due}}",
			wantCount: 1, // website has due, trait5 is within it
		},
		{
			name:      "on with field filter",
			query:     "trait:todo on:{object:section .title==Tasks}",
			wantCount: 3, // trait5 on website#tasks, trait7 and trait8 on mobile#tasks
		},
		{
			name:      "within paused project",
			query:     "trait:todo within:{object:project .status==paused}",
			wantCount: 2, // trait7 and trait8 are within mobile (paused)
		},
		{
			name:      "highlight within date",
			query:     "trait:highlight within:{object:date}",
			wantCount: 1, // trait3 is within daily note
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
