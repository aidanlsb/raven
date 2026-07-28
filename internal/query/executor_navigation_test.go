package query

import (
	"reflect"
	"testing"
)

func TestTraitWithinIncludesAttachmentScope(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		DELETE FROM traits WHERE trait_type = 'todo';

		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('project/beta', 'project/beta.md', 'project', '{}', 1),
			('project/navigation', 'project/navigation.md', 'project', '{}', 1);

		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id)
		VALUES
			('project/navigation#tasks', 'project/navigation', 'project/navigation.md', 'tasks', 'Tasks', 2, 20, 29, 49, NULL),
			('project/navigation#subtasks', 'project/navigation', 'project/navigation.md', 'subtasks', 'Subtasks', 3, 30, 49, 49, 'project/navigation#tasks');

		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('trait-beta-preamble', 'project/beta.md', 'project/beta', 'todo', 'todo', 'Preamble task', 5),
			('trait-tasks', 'project/navigation.md', 'project/navigation#tasks', 'todo', 'todo', 'Direct task', 25),
			('trait-subtasks', 'project/navigation.md', 'project/navigation#subtasks', 'todo', 'todo', 'Nested task', 35);
	`)
	if err != nil {
		t.Fatalf("failed to insert within regression fixtures: %v", err)
	}

	executor := NewExecutor(db)
	tests := []struct {
		name       string
		scope      string
		wantIn     []string
		wantWithin []string
	}{
		{
			name:       "section includes direct and nested traits",
			scope:      "section .title==Tasks",
			wantIn:     []string{"trait-tasks"},
			wantWithin: []string{"trait-tasks", "trait-subtasks"},
		},
		{
			name:       "nested section includes directly attached trait",
			scope:      "section .title==Subtasks",
			wantIn:     []string{"trait-subtasks"},
			wantWithin: []string{"trait-subtasks"},
		},
		{
			name:       "object includes heading-free preamble trait",
			scope:      "[[project/beta]]",
			wantIn:     []string{"trait-beta-preamble"},
			wantWithin: []string{"trait-beta-preamble"},
		},
	}

	queryIDs := func(t *testing.T, predicate, scope string) []string {
		t.Helper()
		q, parseErr := Parse("trait:todo " + predicate + "(" + scope + ")")
		if parseErr != nil {
			t.Fatalf("parse error: %v", parseErr)
		}
		results, queryErr := executor.executeTraitQuery(q)
		if queryErr != nil {
			t.Fatalf("execute error: %v", queryErr)
		}
		ids := make([]string, 0, len(results))
		for _, result := range results {
			ids = append(ids, result.ID)
		}
		return ids
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inIDs := queryIDs(t, "in", tt.scope)
			if !reflect.DeepEqual(inIDs, tt.wantIn) {
				t.Errorf("in IDs = %#v, want %#v", inIDs, tt.wantIn)
			}

			withinIDs := queryIDs(t, "within", tt.scope)
			if !reflect.DeepEqual(withinIDs, tt.wantWithin) {
				t.Errorf("within IDs = %#v, want %#v", withinIDs, tt.wantWithin)
			}

			withinSet := make(map[string]struct{}, len(withinIDs))
			for _, id := range withinIDs {
				withinSet[id] = struct{}{}
			}
			for _, id := range inIDs {
				if _, ok := withinSet[id]; !ok {
					t.Errorf("within results do not include directly attached trait %q", id)
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
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentScopeID)
				}
			}
		})
	}
}

func TestSectionContainsNestedSection(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	q, err := Parse("section contains(section .title==Verification)")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	results, err := NewExecutor(db).executeSectionQuery(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{
		"projects/website#tasks":          true,
		"projects/website#implementation": true,
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(results), len(want), results)
	}
	for _, result := range results {
		if !want[result.ID] {
			t.Errorf("unexpected section result %q", result.ID)
		}
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
			name:      "refd by file source includes its sections",
			query:     "type:project refd([[projects/mobile]])",
			wantCount: 1, // website is referenced by the mobile project's tasks section
		},
		{
			name:      "refd by object subquery includes source sections",
			query:     "type:project refd(type:date)",
			wantCount: 2, // website and mobile are referenced by sections in the daily note
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
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentScopeID)
				}
			}
		})
	}
}
