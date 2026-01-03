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

		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('trait1', 'projects/website.md', 'projects/website', 'due', '2025-06-30', 'projects/website', 1),
			('trait2', 'daily/2025-02-01.md', 'daily/2025-02-01#standup', 'due', '2025-02-03', 'Follow up on timeline', 15),
			('trait3', 'daily/2025-02-01.md', 'daily/2025-02-01#standup', 'highlight', NULL, 'Important insight', 18),
			('trait4', 'people/freya.md', 'people/freya', 'due', '2025-02-01', 'Send docs', 12);

		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('daily/2025-02-01#standup', 'projects/website', 'projects/website', 'daily/2025-02-01.md', 12),
			('daily/2025-02-01#standup', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 13),
			('daily/2025-02-01#planning', 'projects/mobile', 'projects/mobile', 'daily/2025-02-01.md', 32),
			('daily/2025-02-01#planning', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 33),
			('projects/website', 'people/freya', 'people/freya', 'projects/website.md', 5);
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
