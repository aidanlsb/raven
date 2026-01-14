package query

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupPipelineTestDB(t *testing.T) *sql.DB {
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
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		-- Projects with different numbers of todos
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('projects/alpha', 'projects/alpha.md', 'project', '{"status":"active","priority":"high"}', 1),
			('projects/beta', 'projects/beta.md', 'project', '{"status":"active","priority":"medium"}', 1),
			('projects/gamma', 'projects/gamma.md', 'project', '{"status":"paused","priority":"low"}', 1);

		-- Sections inside projects
		INSERT INTO objects (id, file_path, type, fields, line_start, parent_id) VALUES
			('projects/alpha#tasks', 'projects/alpha.md', 'section', '{"title":"Tasks"}', 10, 'projects/alpha'),
			('projects/beta#tasks', 'projects/beta.md', 'section', '{"title":"Tasks"}', 10, 'projects/beta');

		-- Todos on sections
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			-- Alpha has 3 todos
			('t1', 'projects/alpha.md', 'projects/alpha#tasks', 'todo', 'todo', 'Build feature A', 11),
			('t2', 'projects/alpha.md', 'projects/alpha#tasks', 'todo', 'todo', 'Build feature B', 12),
			('t3', 'projects/alpha.md', 'projects/alpha#tasks', 'todo', 'done', 'Setup project', 13),
			-- Beta has 1 todo
			('t4', 'projects/beta.md', 'projects/beta#tasks', 'todo', 'todo', 'Initial setup', 11),
			-- Gamma has 0 todos (no tasks section even)
			-- Due dates
			('d1', 'projects/alpha.md', 'projects/alpha', 'due', '2025-01-15', 'Deadline', 5),
			('d2', 'projects/beta.md', 'projects/beta', 'due', '2025-02-01', 'Deadline', 5);

		-- Refs for testing count(refs(_)) and count(refd(_))
		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('projects/alpha', 'projects/beta', 'projects/beta', 'projects/alpha.md', 3),
			('projects/alpha', 'projects/gamma', 'projects/gamma', 'projects/alpha.md', 4),
			('projects/beta', 'projects/alpha', 'projects/alpha', 'projects/beta.md', 3);
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	return db
}

func TestPipelineCount(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name       string
		query      string
		wantCounts map[string]int // object ID -> expected count
	}{
		{
			name:  "count todos in descendants",
			query: "object:project .status==active |> todos = count({trait:todo value==todo within:_})",
			wantCounts: map[string]int{
				"projects/alpha": 2, // 2 incomplete todos
				"projects/beta":  1, // 1 incomplete todo
			},
		},
		{
			name:  "count refs from object",
			query: "object:project |> outgoing = count(refs(_))",
			wantCounts: map[string]int{
				"projects/alpha": 2, // refs to beta and gamma
				"projects/beta":  1, // refs to alpha
				"projects/gamma": 0, // no outgoing refs
			},
		},
		{
			name:  "count refs to object",
			query: "object:project |> incoming = count(refd(_))",
			wantCounts: map[string]int{
				"projects/alpha": 1, // referenced by beta
				"projects/beta":  1, // referenced by alpha
				"projects/gamma": 1, // referenced by alpha
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			results, err := executor.ExecuteObjectQueryWithPipeline(q)
			if err != nil {
				t.Fatalf("failed to execute query: %v", err)
			}

			for _, r := range results {
				wantCount, exists := tt.wantCounts[r.ID]
				if !exists {
					continue
				}

				// Get the computed value name from the first assignment stage
				var valueName string
				for _, stage := range q.Pipeline.Stages {
					if a, ok := stage.(*AssignmentStage); ok {
						valueName = a.Name
						break
					}
				}

				gotCount, ok := r.Computed[valueName].(int)
				if !ok {
					t.Errorf("object %s: computed value %s is not an int: %v", r.ID, valueName, r.Computed[valueName])
					continue
				}

				if gotCount != wantCount {
					t.Errorf("object %s: got count %d, want %d", r.ID, gotCount, wantCount)
				}
			}
		})
	}
}

func TestPipelineFilter(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{
			name:    "filter by computed count > 0",
			query:   "object:project |> todos = count({trait:todo value==todo within:_}) filter(todos > 0)",
			wantIDs: []string{"projects/alpha", "projects/beta"},
		},
		{
			name:    "filter by computed count >= 2",
			query:   "object:project |> todos = count({trait:todo value==todo within:_}) filter(todos >= 2)",
			wantIDs: []string{"projects/alpha"},
		},
		{
			name:    "filter by field",
			query:   "object:project |> filter(.priority == high)",
			wantIDs: []string{"projects/alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			results, err := executor.ExecuteObjectQueryWithPipeline(q)
			if err != nil {
				t.Fatalf("failed to execute query: %v", err)
			}

			gotIDs := make([]string, len(results))
			for i, r := range results {
				gotIDs[i] = r.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("got %d results, want %d\ngot: %v\nwant: %v", len(gotIDs), len(tt.wantIDs), gotIDs, tt.wantIDs)
				return
			}

			for i, id := range tt.wantIDs {
				if gotIDs[i] != id {
					t.Errorf("result[%d]: got %s, want %s", i, gotIDs[i], id)
				}
			}
		})
	}
}

func TestPipelineSort(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{
			name:    "sort by computed count descending",
			query:   "object:project .status==active |> todos = count({trait:todo value==todo within:_}) sort(todos, desc)",
			wantIDs: []string{"projects/alpha", "projects/beta"}, // alpha=2, beta=1
		},
		{
			name:    "sort by field ascending",
			query:   "object:project |> sort(.priority, asc)",
			wantIDs: []string{"projects/alpha", "projects/gamma", "projects/beta"}, // high, low, medium (alphabetical)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			results, err := executor.ExecuteObjectQueryWithPipeline(q)
			if err != nil {
				t.Fatalf("failed to execute query: %v", err)
			}

			gotIDs := make([]string, len(results))
			for i, r := range results {
				gotIDs[i] = r.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("got %d results, want %d\ngot: %v\nwant: %v", len(gotIDs), len(tt.wantIDs), gotIDs, tt.wantIDs)
				return
			}

			for i, id := range tt.wantIDs {
				if gotIDs[i] != id {
					t.Errorf("result[%d]: got %s, want %s", i, gotIDs[i], id)
				}
			}
		})
	}
}

func TestPipelineLimit(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	q, err := Parse("object:project |> limit(2)")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	results, err := executor.ExecuteObjectQueryWithPipeline(q)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}

func TestPipelineMultiSort(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Sort by status (asc), then priority (desc)
	// status: active < paused (alphabetically)
	// priority desc: medium > low > high (alphabetically descending, m > l > h)
	q, err := Parse("object:project |> sort(.status, asc) sort(.priority, desc)")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	results, err := executor.ExecuteObjectQueryWithPipeline(q)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}

	// Verify we got all 3 projects and they're sorted
	if len(results) != 3 {
		t.Errorf("got %d results, want 3", len(results))
	}

	// active first (alpha,beta), then paused (gamma)
	// Within active: priority desc means medium > high (m > h alphabetically)
	// So: beta(active,medium) -> alpha(active,high) -> gamma(paused,low)
	expectedOrder := []string{"projects/beta", "projects/alpha", "projects/gamma"}
	for i, r := range results {
		if r.ID != expectedOrder[i] {
			t.Errorf("result[%d]: got %s, want %s", i, r.ID, expectedOrder[i])
		}
	}
}

func TestPipelineMinMaxOnTraits(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// min/max should work on trait queries - get earliest due date
	q, err := Parse("object:project |> earliest = min({trait:due within:_})")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	results, err := executor.ExecuteObjectQueryWithPipeline(q)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}

	// Alpha has due date 2025-01-15, Beta has 2025-02-01, Gamma has none
	for _, r := range results {
		earliest := r.Computed["earliest"]
		switch r.ID {
		case "projects/alpha":
			if earliest == nil {
				t.Errorf("alpha should have earliest due date, got nil")
			} else if v, ok := earliest.(*string); !ok || v == nil || *v != "2025-01-15" {
				t.Errorf("alpha earliest: got %v, want 2025-01-15", earliest)
			}
		case "projects/beta":
			if earliest == nil {
				t.Errorf("beta should have earliest due date, got nil")
			} else if v, ok := earliest.(*string); !ok || v == nil || *v != "2025-02-01" {
				t.Errorf("beta earliest: got %v, want 2025-02-01", earliest)
			}
		case "projects/gamma":
			if earliest != nil {
				t.Errorf("gamma should have no due date, got %v", earliest)
			}
		}
	}
}

func TestPipelineAggregationValidation(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	t.Run("max on object without field should error", func(t *testing.T) {
		q, err := Parse("object:project |> val = max({object:section parent:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		_, err = executor.ExecuteObjectQueryWithPipeline(q)
		if err == nil {
			t.Fatal("expected error for max() on object query without field")
		}
		if !strings.Contains(err.Error(), "requires a field") {
			t.Errorf("error should mention 'requires a field', got: %v", err)
		}
	})

	t.Run("min on object without field should error", func(t *testing.T) {
		q, err := Parse("object:project |> val = min({object:section parent:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		_, err = executor.ExecuteObjectQueryWithPipeline(q)
		if err == nil {
			t.Fatal("expected error for min() on object query without field")
		}
		if !strings.Contains(err.Error(), "requires a field") {
			t.Errorf("error should mention 'requires a field', got: %v", err)
		}
	})

	t.Run("max with field on object query should work", func(t *testing.T) {
		q, err := Parse("object:project |> maxPriority = max(.priority, {object:project refs:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Just verify it runs without error
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("count on objects should work without field", func(t *testing.T) {
		q, err := Parse("object:project |> sectionCount = count({object:section parent:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Alpha has 1 section, Beta has 1 section, Gamma has 0 sections
		for _, r := range results {
			val := r.Computed["sectionCount"]
			switch r.ID {
			case "projects/alpha", "projects/beta":
				if val != 1 {
					t.Errorf("%s: got %v want 1", r.ID, val)
				}
			case "projects/gamma":
				if val != 0 {
					t.Errorf("%s: got %v want 0", r.ID, val)
				}
			}
		}
	})
}

func TestPipelineFullExample(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Complex pipeline: count todos, filter, sort, limit
	q, err := Parse("object:project |> todos = count({trait:todo value==todo within:_}) filter(todos > 0) sort(todos, desc) limit(5)")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	results, err := executor.ExecuteObjectQueryWithPipeline(q)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}

	// Should get alpha (2 todos) then beta (1 todo)
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
		return
	}

	if results[0].ID != "projects/alpha" {
		t.Errorf("first result: got %s, want projects/alpha", results[0].ID)
	}
	if results[1].ID != "projects/beta" {
		t.Errorf("second result: got %s, want projects/beta", results[1].ID)
	}

	// Check computed values
	if todos, ok := results[0].Computed["todos"].(int); !ok || todos != 2 {
		t.Errorf("alpha todos: got %v, want 2", results[0].Computed["todos"])
	}
	if todos, ok := results[1].Computed["todos"].(int); !ok || todos != 1 {
		t.Errorf("beta todos: got %v, want 1", results[1].Computed["todos"])
	}
}

// ============================================================================
// TRAIT QUERY PIPELINE TESTS
// ============================================================================

func TestTraitPipelineBasic(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	t.Run("simple sort by value", func(t *testing.T) {
		q, err := Parse("trait:due |> sort(.value, asc)")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Should have 2 due traits, sorted by date value
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}

		// 2025-01-15 < 2025-02-01
		if results[0].Value == nil || *results[0].Value != "2025-01-15" {
			t.Errorf("first result: got %v, want 2025-01-15", results[0].Value)
		}
		if results[1].Value == nil || *results[1].Value != "2025-02-01" {
			t.Errorf("second result: got %v, want 2025-02-01", results[1].Value)
		}
	})

	t.Run("limit", func(t *testing.T) {
		q, err := Parse("trait:todo |> limit(2)")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("filter by value", func(t *testing.T) {
		q, err := Parse("trait:todo |> filter(.value == todo)")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Should only get incomplete todos (value == "todo", not "done")
		if len(results) != 3 {
			t.Errorf("expected 3 incomplete todos, got %d", len(results))
		}
	})
}

func TestTraitPipelineAggregation(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	t.Run("count refs on trait line", func(t *testing.T) {
		q, err := Parse("trait:todo |> refCount = count(refs(_))")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// All traits should have refCount computed (may be 0)
		for _, r := range results {
			if _, ok := r.Computed["refCount"]; !ok {
				t.Errorf("trait %s missing refCount computed value", r.ID)
			}
		}
	})

	t.Run("on:_ in trait context should error", func(t *testing.T) {
		// on:_ is invalid in trait context because on: expects an object, not a trait
		q, err := Parse("trait:todo |> siblings = count({trait:due on:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		_, err = executor.ExecuteTraitQueryWithPipeline(q)
		if err == nil {
			t.Fatal("expected error for on:_ in trait context")
		}
		if !strings.Contains(err.Error(), "on:_ is invalid in trait context") {
			t.Errorf("expected error about on:_ being invalid, got: %v", err)
		}
	})

	t.Run("count co-located traits via at:_", func(t *testing.T) {
		// at:_ binds to the trait's file+line (co-location)
		q, err := Parse("trait:todo |> colocated = count({trait:priority at:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Verify we have computed values (may be 0 for todos without co-located priority)
		for _, r := range results {
			if _, ok := r.Computed["colocated"]; !ok {
				t.Errorf("trait %s missing colocated computed value", r.ID)
			}
		}
	})

	t.Run("refd:_ works for traits (refs on same line)", func(t *testing.T) {
		// refd:_ in trait context means "is referenced by (the line containing) this trait"
		q, err := Parse("trait:due |> referenced = count({object:project refd:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Verify we have computed values
		for _, r := range results {
			if _, ok := r.Computed["referenced"]; !ok {
				t.Errorf("trait %s missing referenced computed value", r.ID)
			}
		}
	})

	t.Run("refs:_ should error for traits", func(t *testing.T) {
		// refs:_ is invalid because traits can't be the target of [[...]] references
		q, err := Parse("trait:todo |> x = count({object:project refs:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		_, err = executor.ExecuteTraitQueryWithPipeline(q)
		if err == nil {
			t.Fatal("expected error for refs:_ in trait context")
		}
		if !strings.Contains(err.Error(), "refs:_ is invalid in trait context") {
			t.Errorf("expected error about refs:_ being invalid, got: %v", err)
		}
	})

	t.Run("has:_ works for traits", func(t *testing.T) {
		// has:_ means "objects that directly have this trait"
		q, err := Parse("trait:todo |> parents = count({object:project has:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Each todo should have a parents count (the objects that have this trait)
		for _, r := range results {
			if _, ok := r.Computed["parents"]; !ok {
				t.Errorf("trait %s missing parents computed value", r.ID)
			}
		}
	})

	t.Run("contains:_ works for traits", func(t *testing.T) {
		// contains:_ means "objects that contain this trait in their subtree"
		q, err := Parse("trait:todo |> containers = count({object:project contains:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Each todo should have a containers count
		for _, r := range results {
			if _, ok := r.Computed["containers"]; !ok {
				t.Errorf("trait %s missing containers computed value", r.ID)
			}
		}
	})

	// Test all predicates that should ERROR in trait context
	errorCases := []struct {
		name    string
		query   string
		errText string
	}{
		{"within:_", "trait:todo |> x = count({object:project within:_})", "within:_ is invalid in trait context"},
		{"ancestor:_", "trait:todo |> x = count({object:project ancestor:_})", "ancestor:_ is invalid in trait context"},
		{"descendant:_", "trait:todo |> x = count({object:project descendant:_})", "descendant:_ is invalid in trait context"},
		{"parent:_", "trait:todo |> x = count({object:project parent:_})", "parent:_ is invalid in trait context"},
		{"child:_", "trait:todo |> x = count({object:project child:_})", "child:_ is invalid in trait context"},
	}

	for _, tc := range errorCases {
		t.Run(tc.name+" should error for traits", func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			_, err = executor.ExecuteTraitQueryWithPipeline(q)
			if err == nil {
				t.Fatalf("expected error for %s in trait context", tc.name)
			}
			if !strings.Contains(err.Error(), tc.errText) {
				t.Errorf("expected error containing '%s', got: %v", tc.errText, err)
			}
		})
	}
}

func TestObjectPipelinePredicates(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// Test all predicates that should WORK in object context
	workingCases := []struct {
		name  string
		query string
	}{
		{"ancestor:_", "object:project |> x = count({object:section ancestor:_})"},
		{"descendant:_", "object:project |> x = count({object:section descendant:_})"},
		{"parent:_", "object:project |> x = count({object:section parent:_})"},
		{"child:_", "object:project |> x = count({object:section child:_})"},
		{"refs:_", "object:project |> x = count({object:project refs:_})"},
		{"refd:_", "object:project |> x = count({object:project refd:_})"},
		{"within:_", "object:project |> x = count({trait:todo within:_})"},
		{"on:_", "object:project |> x = count({trait:todo on:_})"},
	}

	for _, tc := range workingCases {
		t.Run(tc.name+" works for objects", func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			results, err := executor.ExecuteObjectQueryWithPipeline(q)
			if err != nil {
				t.Fatalf("failed to execute query with %s: %v", tc.name, err)
			}

			// Verify we got results with computed values
			for _, r := range results {
				if _, ok := r.Computed["x"]; !ok {
					t.Errorf("object %s missing computed value for %s", r.ID, tc.name)
				}
			}
		})
	}
}

func TestTraitPipelineComplex(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	t.Run("full pipeline: aggregate, filter, sort, limit", func(t *testing.T) {
		// Count refs, filter to those with any, sort by count desc, limit to 5
		q, err := Parse("trait:todo |> refs = count(refs(_)) sort(.value, asc) limit(3)")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		if len(results) > 3 {
			t.Errorf("expected at most 3 results, got %d", len(results))
		}

		// Verify computed values exist
		for _, r := range results {
			if _, ok := r.Computed["refs"]; !ok {
				t.Errorf("trait %s missing refs computed value", r.ID)
			}
		}
	})

	t.Run("multi-sort on traits", func(t *testing.T) {
		// Sort by value (todo status), then by content
		q, err := Parse("trait:todo |> sort(.value, asc)")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteTraitQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Should have 4 todos total
		if len(results) != 4 {
			t.Errorf("expected 4 todos, got %d", len(results))
		}
	})
}

func TestBatchedAggregation(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	// These queries should use batched execution (no extra predicates beyond the type and binding)
	t.Run("batched count refs from", func(t *testing.T) {
		q, err := Parse("object:project |> outgoing = count(refs(_))")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Verify counts
		for _, r := range results {
			switch r.ID {
			case "projects/alpha":
				if r.Computed["outgoing"] != 2 {
					t.Errorf("alpha outgoing: got %v, want 2", r.Computed["outgoing"])
				}
			case "projects/beta":
				if r.Computed["outgoing"] != 1 {
					t.Errorf("beta outgoing: got %v, want 1", r.Computed["outgoing"])
				}
			case "projects/gamma":
				if r.Computed["outgoing"] != 0 {
					t.Errorf("gamma outgoing: got %v, want 0", r.Computed["outgoing"])
				}
			}
		}
	})

	t.Run("batched count children", func(t *testing.T) {
		q, err := Parse("object:project |> sections = count({object:section parent:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Alpha and Beta have 1 section each, Gamma has 0
		for _, r := range results {
			switch r.ID {
			case "projects/alpha":
				if r.Computed["sections"] != 1 {
					t.Errorf("alpha sections: got %v, want 1", r.Computed["sections"])
				}
			case "projects/beta":
				if r.Computed["sections"] != 1 {
					t.Errorf("beta sections: got %v, want 1", r.Computed["sections"])
				}
			case "projects/gamma":
				if r.Computed["sections"] != 0 {
					t.Errorf("gamma sections: got %v, want 0", r.Computed["sections"])
				}
			}
		}
	})

	t.Run("batched count descendants", func(t *testing.T) {
		q, err := Parse("object:project |> desc = count(descendants(_))")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Alpha and Beta have 1 descendant (their section), Gamma has 0
		for _, r := range results {
			switch r.ID {
			case "projects/alpha":
				if r.Computed["desc"] != 1 {
					t.Errorf("alpha descendants: got %v, want 1", r.Computed["desc"])
				}
			case "projects/beta":
				if r.Computed["desc"] != 1 {
					t.Errorf("beta descendants: got %v, want 1", r.Computed["desc"])
				}
			case "projects/gamma":
				if r.Computed["desc"] != 0 {
					t.Errorf("gamma descendants: got %v, want 0", r.Computed["desc"])
				}
			}
		}
	})

	t.Run("batched count traits with within binding", func(t *testing.T) {
		// Simple trait query without extra predicates - should batch
		q, err := Parse("object:project |> traits = count({trait:todo within:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Alpha has 3 todos, Beta has 1, Gamma has 0
		for _, r := range results {
			switch r.ID {
			case "projects/alpha":
				if r.Computed["traits"] != 3 {
					t.Errorf("alpha todos: got %v, want 3", r.Computed["traits"])
				}
			case "projects/beta":
				if r.Computed["traits"] != 1 {
					t.Errorf("beta todos: got %v, want 1", r.Computed["traits"])
				}
			case "projects/gamma":
				if r.Computed["traits"] != 0 {
					t.Errorf("gamma todos: got %v, want 0", r.Computed["traits"])
				}
			}
		}
	})
}

func TestTraitPipelineMinMax(t *testing.T) {
	db := setupPipelineTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)

	t.Run("min on trait subquery works without field", func(t *testing.T) {
		// This uses trait subquery - should work without field specifier
		q, err := Parse("object:project |> earliest = min({trait:due within:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		// Alpha should have 2025-01-15, Beta should have 2025-02-01
		for _, r := range results {
			switch r.ID {
			case "projects/alpha":
				if earliest, ok := r.Computed["earliest"].(*string); !ok || earliest == nil || *earliest != "2025-01-15" {
					t.Errorf("alpha earliest: got %v, want 2025-01-15", r.Computed["earliest"])
				}
			case "projects/beta":
				if earliest, ok := r.Computed["earliest"].(*string); !ok || earliest == nil || *earliest != "2025-02-01" {
					t.Errorf("beta earliest: got %v, want 2025-02-01", r.Computed["earliest"])
				}
			case "projects/gamma":
				if r.Computed["earliest"] != nil {
					t.Errorf("gamma should have nil earliest, got %v", r.Computed["earliest"])
				}
			}
		}
	})

	t.Run("max on trait subquery", func(t *testing.T) {
		q, err := Parse("object:project |> latest = max({trait:due within:_})")
		if err != nil {
			t.Fatalf("failed to parse query: %v", err)
		}

		results, err := executor.ExecuteObjectQueryWithPipeline(q)
		if err != nil {
			t.Fatalf("failed to execute query: %v", err)
		}

		for _, r := range results {
			switch r.ID {
			case "projects/alpha":
				if latest, ok := r.Computed["latest"].(*string); !ok || latest == nil || *latest != "2025-01-15" {
					t.Errorf("alpha latest: got %v, want 2025-01-15", r.Computed["latest"])
				}
			case "projects/beta":
				if latest, ok := r.Computed["latest"].(*string); !ok || latest == nil || *latest != "2025-02-01" {
					t.Errorf("beta latest: got %v, want 2025-02-01", r.Computed["latest"])
				}
			}
		}
	})
}
