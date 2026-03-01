package index

import (
	"testing"
)

func TestParseFilterExpression(t *testing.T) {
	tests := []struct {
		name           string
		filter         string
		fieldExpr      string
		wantCondition  string
		wantArgsCount  int
		wantArgsValues []interface{}
		wantErr        bool
	}{
		{
			name:           "simple value",
			filter:         "done",
			fieldExpr:      "value",
			wantCondition:  "value = ?",
			wantArgsCount:  1,
			wantArgsValues: []interface{}{"done"},
		},
		{
			name:           "NOT value",
			filter:         "!done",
			fieldExpr:      "value",
			wantCondition:  "value != ?",
			wantArgsCount:  1,
			wantArgsValues: []interface{}{"done"},
		},
		{
			name:          "OR two values",
			filter:        "todo|in-progress",
			fieldExpr:     "value",
			wantCondition: "(value = ? OR value = ?)",
			wantArgsCount: 2,
		},
		{
			name:          "NOT list uses AND",
			filter:        "!done|!cancelled",
			fieldExpr:     "value",
			wantCondition: "(value != ? AND value != ?)",
			wantArgsCount: 2,
		},
		{
			name:          "mixed OR and NOT",
			filter:        "active|!done",
			fieldExpr:     "value",
			wantCondition: "(value = ? OR value != ?)",
			wantArgsCount: 2,
		},
		{
			name:          "three values OR",
			filter:        "a|b|c",
			fieldExpr:     "value",
			wantCondition: "(value = ? OR value = ? OR value = ?)",
			wantArgsCount: 3,
		},
		{
			name:          "date filter today",
			filter:        "today",
			fieldExpr:     "value",
			wantCondition: "value = ?",
			wantArgsCount: 1,
		},
		{
			name:          "date filter tomorrow",
			filter:        "tomorrow",
			fieldExpr:     "value",
			wantCondition: "value = ?",
			wantArgsCount: 1,
		},
		{
			name:           "past treated as plain value",
			filter:         "past",
			fieldExpr:      "value",
			wantCondition:  "value = ?",
			wantArgsCount:  1,
			wantArgsValues: []interface{}{"past"},
		},
		{
			name:          "empty filter",
			filter:        "",
			fieldExpr:     "value",
			wantCondition: "1=1",
			wantArgsCount: 0,
		},
		{
			name:          "whitespace in OR",
			filter:        "done | cancelled",
			fieldExpr:     "value",
			wantCondition: "(value = ? OR value = ?)",
			wantArgsCount: 2,
		},
		{
			name:      "invalid date-like filter errors",
			filter:    "2025-13-45",
			fieldExpr: "value",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, err := parseFilterExpression(tt.filter, tt.fieldExpr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if condition != tt.wantCondition {
				t.Errorf("condition = %q, want %q", condition, tt.wantCondition)
			}

			if len(args) != tt.wantArgsCount {
				t.Errorf("args count = %d, want %d", len(args), tt.wantArgsCount)
			}

			// Check specific arg values if provided
			if tt.wantArgsValues != nil {
				for i, want := range tt.wantArgsValues {
					if i < len(args) && args[i] != want {
						t.Errorf("args[%d] = %v, want %v", i, args[i], want)
					}
				}
			}
		})
	}
}

func TestBuildFTSContentQuery_SanitizesHyphenatedTokens(t *testing.T) {
	q := BuildFTSContentQuery(`michael-truell OR "Michael Truell"`)
	if q != `content: ("michael-truell" OR "Michael Truell")` {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, `content: ("michael-truell" OR "Michael Truell")`)
	}
}

func TestBuildFTSSearchQuery_ScopesTitleAndContent(t *testing.T) {
	q := BuildFTSSearchQuery("hello world")
	want := `{title content}: (hello world)`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}

	q = BuildFTSSearchQuery(`michael-truell OR "Michael Truell"`)
	want = `{title content}: ("michael-truell" OR "Michael Truell")`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}

	q = BuildFTSSearchQuery("")
	want = `{title content}:""`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}
}

func TestSearch_AllowsHyphenatedTokenWithOR(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert a minimal object row for SearchWithType joins (and good hygiene).
	_, err = db.db.Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES ('people/michael-truell', 'people/michael-truell.md', 'person', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	// Index a search row. FTS tokenization will split on '-', which is fine.
	_, err = db.db.Exec(`INSERT INTO fts_content (object_id, title, content, file_path) VALUES (?, ?, ?, ?)`,
		"people/michael-truell",
		"Michael Truell",
		`::meeting(with=[[michael-truell]])`,
		"daily/2026-01-29.md",
	)
	if err != nil {
		t.Fatalf("failed to insert fts row: %v", err)
	}

	// This used to fail with "no such column: truell" due to FTS parsing.
	results, err := db.Search(`michael-truell OR "Michael Truell"`, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}
}

func TestSearch_MatchesTitle(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES ('project/raven', 'project/raven.md', 'project', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	// Index with a distinctive title but no matching content
	_, err = db.db.Exec(`INSERT INTO fts_content (object_id, title, content, file_path) VALUES (?, ?, ?, ?)`,
		"project/raven",
		"Raven Knowledge Base",
		"This is a project about structured notes.",
		"project/raven.md",
	)
	if err != nil {
		t.Fatalf("failed to insert fts row: %v", err)
	}

	// Search for a term that only appears in the title
	results, err := db.Search("Knowledge", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search to match title, got no results")
	}
	if results[0].ObjectID != "project/raven" {
		t.Fatalf("expected object_id 'project/raven', got %q", results[0].ObjectID)
	}
}

func TestQueryTraitsWithFilterExpressions(t *testing.T) {
	// Integration tests with actual database
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test traits directly
	_, err = db.db.Exec(`
		INSERT INTO traits (id, trait_type, value, content, file_path, line_number, parent_object_id)
		VALUES 
			('t1', 'status', 'done', 'Task 1', 'test.md', 1, 'obj1'),
			('t2', 'status', 'todo', 'Task 2', 'test.md', 2, 'obj2'),
			('t3', 'status', 'in-progress', 'Task 3', 'test.md', 3, 'obj3'),
			('t4', 'status', 'cancelled', 'Task 4', 'test.md', 4, 'obj4'),
			('t5', 'priority', 'high', 'Priority 1', 'test.md', 5, 'obj5'),
			('t6', 'priority', 'low', 'Priority 2', 'test.md', 6, 'obj6')
	`)
	if err != nil {
		t.Fatalf("failed to insert test traits: %v", err)
	}

	t.Run("simple filter", func(t *testing.T) {
		filter := "done"
		results, err := db.QueryTraits("status", &filter)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if len(results) > 0 && *results[0].Value != "done" {
			t.Errorf("expected value 'done', got '%s'", *results[0].Value)
		}
	})

	t.Run("NOT filter", func(t *testing.T) {
		filter := "!done"
		results, err := db.QueryTraits("status", &filter)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 results (todo, in-progress, cancelled), got %d", len(results))
		}
		// Verify none are "done"
		for _, r := range results {
			if r.Value != nil && *r.Value == "done" {
				t.Errorf("found 'done' in NOT done results")
			}
		}
	})

	t.Run("OR filter", func(t *testing.T) {
		filter := "todo|in-progress"
		results, err := db.QueryTraits("status", &filter)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("NOT with OR filter", func(t *testing.T) {
		filter := "!done|!cancelled"
		results, err := db.QueryTraits("status", &filter)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		// This means: value != 'done' AND value != 'cancelled'
		if len(results) != 2 {
			t.Errorf("expected 2 results (todo, in-progress), got %d", len(results))
		}
	})

	t.Run("no filter returns all", func(t *testing.T) {
		results, err := db.QueryTraits("status", nil)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 4 {
			t.Errorf("expected 4 results, got %d", len(results))
		}
	})

	t.Run("invalid date-like filter errors", func(t *testing.T) {
		filter := "2025-13-45"
		_, err := db.QueryTraits("status", &filter)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}

func TestBacklinks(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES 
			('people/freya', 'people/freya.md', 'person', 1, '{}'),
			('daily/2025-02-01', 'daily/2025-02-01.md', 'date', 1, '{}'),
			('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	// Insert test refs
	_, err = db.db.Exec(`
		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number)
		VALUES 
			('daily/2025-02-01', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 5),
			('projects/bifrost', 'people/freya', 'freya', 'projects/bifrost.md', 10),
			('projects/bifrost', 'people/freya#notes', 'freya#notes', 'projects/bifrost.md', 11)
	`)
	if err != nil {
		t.Fatalf("failed to insert test refs: %v", err)
	}

	t.Run("find backlinks to person", func(t *testing.T) {
		results, err := db.Backlinks("people/freya")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		// Includes a section backlink via target_id LIKE 'people/freya#%'.
		if len(results) != 3 {
			t.Errorf("expected 3 backlinks, got %d", len(results))
		}
	})

	t.Run("no backlinks", func(t *testing.T) {
		results, err := db.Backlinks("projects/bifrost")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 backlinks, got %d", len(results))
		}
	})
}

func TestOutlinks(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES
			('people/freya', 'people/freya.md', 'person', 1, '{}'),
			('daily/2025-02-01', 'daily/2025-02-01.md', 'date', 1, '{}'),
			('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	// Insert test refs (including a section outlink via source_id LIKE 'projects/bifrost#%').
	_, err = db.db.Exec(`
		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number, position_start)
		VALUES
			('daily/2025-02-01', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 5, 1),
			('projects/bifrost', 'people/freya', 'freya', 'projects/bifrost.md', 10, 1),
			('projects/bifrost#notes', 'people/freya', 'freya', 'projects/bifrost.md', 11, 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert test refs: %v", err)
	}

	t.Run("find outlinks from object (includes section outlinks)", func(t *testing.T) {
		results, err := db.Outlinks("projects/bifrost")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 outlinks, got %d", len(results))
		}
	})

	t.Run("find outlinks from daily note", func(t *testing.T) {
		results, err := db.Outlinks("daily/2025-02-01")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 outlink, got %d", len(results))
		}
	})
}

func TestGetObject(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test object
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES ('people/freya', 'people/freya.md', 'person', 1, '{"name": "Freya"}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test object: %v", err)
	}

	t.Run("find existing object", func(t *testing.T) {
		obj, err := db.GetObject("people/freya")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if obj == nil {
			t.Fatal("expected object, got nil")
		}
		if obj.Type != "person" {
			t.Errorf("expected type 'person', got '%s'", obj.Type)
		}
	})

	t.Run("object not found", func(t *testing.T) {
		obj, err := db.GetObject("people/nonexistent")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if obj != nil {
			t.Errorf("expected nil for nonexistent object, got %v", obj)
		}
	})
}

func TestUntypedPages(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES 
			('notes/random', 'notes/random.md', 'page', 1, '{}'),
			('people/freya', 'people/freya.md', 'person', 1, '{}'),
			('notes/another', 'notes/another.md', 'page', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	t.Run("find untyped pages", func(t *testing.T) {
		results, err := db.UntypedPages()
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 untyped pages, got %d", len(results))
		}
	})
}

func TestQueryObjects(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES 
			('people/freya', 'people/freya.md', 'person', 1, '{"name": "Freya"}'),
			('people/thor', 'people/thor.md', 'person', 1, '{"name": "Thor"}'),
			('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	t.Run("query by type", func(t *testing.T) {
		results, err := db.QueryObjects("person")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 person objects, got %d", len(results))
		}
	})

	t.Run("query non-existent type", func(t *testing.T) {
		results, err := db.QueryObjects("company")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})
}
