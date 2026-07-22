package query

import (
	"strings"
	"testing"
)

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
					t.Logf("  - %s: %s (parent: %s)", r.TraitType, r.Content, r.ParentScopeID)
				}
			}
		})
	}
}
