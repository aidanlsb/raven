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
			name:          "OR with NOT",
			filter:        "!done|!cancelled",
			fieldExpr:     "value",
			wantCondition: "(value != ? OR value != ?)",
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
			name:          "date filter past",
			filter:        "past",
			fieldExpr:     "value",
			wantCondition: "value < ? AND value IS NOT NULL",
			wantArgsCount: 1,
		},
		{
			name:          "NOT date filter",
			filter:        "!past",
			fieldExpr:     "value",
			wantCondition: "NOT (value < ? AND value IS NOT NULL)",
			wantArgsCount: 1,
		},
		{
			name:          "OR date filters",
			filter:        "this-week|past",
			fieldExpr:     "value",
			wantCondition: "(value >= ? AND value <= ? OR value < ? AND value IS NOT NULL)",
			wantArgsCount: 3,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args := parseFilterExpression(tt.filter, tt.fieldExpr)

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
		// This means: value != 'done' OR value != 'cancelled'
		// Which matches everything (since every value is either not-done or not-cancelled)
		if len(results) != 4 {
			t.Errorf("expected 4 results, got %d", len(results))
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
}
