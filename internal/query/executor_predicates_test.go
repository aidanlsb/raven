package query

import (
	"testing"
)

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
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentScopeID)
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
