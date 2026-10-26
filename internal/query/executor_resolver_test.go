package query

import (
	"testing"

	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestResolveTargetExactIDSkipsFullResolver(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	id, err := exec.resolveTarget("projects/website")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if id != "projects/website" {
		t.Fatalf("id = %q", id)
	}
	if exec.resolver != nil {
		t.Fatal("exact canonical ID should not build the full resolver")
	}

	sectionID, err := exec.resolveTarget("projects/website#tasks")
	if err != nil {
		t.Fatalf("section resolveTarget: %v", err)
	}
	if sectionID != "projects/website#tasks" {
		t.Fatalf("section id = %q", sectionID)
	}
	if exec.resolver != nil {
		t.Fatal("exact section ID should not build the full resolver")
	}
}

func TestResolveTargetShortNameStillUsesResolver(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	id, err := exec.resolveTarget("website")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if id != "projects/website" {
		t.Fatalf("id = %q, want projects/website", id)
	}
	if exec.resolver == nil {
		t.Fatal("short name should fall back to the full resolver")
	}
}

func TestResolveTargetExactIDMatchesResolverSemantics(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, alias, line_start) VALUES
			('2026-07-20', 'daily/2026-07-20.md', 'date', '{}', '', 1),
			('people/ada', 'people/ada.md', 'person', '{"name":"Ada Lovelace"}', 'Ada', 1),
			('notes/readme', 'notes/readme.md', 'note', '{}', '', 1),
			('archive/readme', 'archive/readme.md', 'note', '{}', '', 1);
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id)
		VALUES ('2026-07-20#standup', '2026-07-20', 'daily/2026-07-20.md', 'standup', 'Standup', 2, 10, 20, 20, NULL);
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {NameField: "name", Fields: map[string]*schema.FieldDefinition{"name": {Type: schema.FieldTypeString}}},
		},
	}
	exec := NewExecutor(db)
	exec.SetSchema(sch)

	full, err := indexschema.BuildResolver(db, indexschema.ResolverOptions{
		DailyDirectory: "daily",
		Schema:         sch,
	})
	if err != nil {
		t.Fatalf("BuildResolver: %v", err)
	}

	tests := []struct {
		name   string
		target string
	}{
		{name: "canonical object", target: "projects/website"},
		{name: "section fragment", target: "projects/website#tasks"},
		{name: "date id", target: "2026-07-20"},
		{name: "date section", target: "2026-07-20#standup"},
		{name: "alias", target: "Ada"},
		{name: "name field", target: "Ada Lovelace"},
		{name: "ambiguous short name", target: "readme"},
		{name: "unresolved", target: "does-not-exist"},
		{name: "legacy daily prefix", target: "daily/2026-07-20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exec.resolveTarget(tt.target)
			want := full.Resolve(tt.target)
			if want.Ambiguous {
				if err == nil {
					t.Fatalf("resolveTarget(%q) = %q, want ambiguous error", tt.target, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTarget(%q): %v", tt.target, err)
			}
			wantID := want.TargetID
			if wantID == "" {
				wantID = tt.target
			}
			if got != wantID {
				t.Fatalf("resolveTarget(%q) = %q, resolver = %q", tt.target, got, wantID)
			}
		})
	}
}

func TestDecimalFieldComparisonExecutes(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('beer/stout', 'beer/stout.md', 'beer', '{"rating": 4.8}', 1),
			('beer/lager', 'beer/lager.md', 'beer', '{"rating": 4.1}', 1);
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	exec := NewExecutor(db)
	q, err := Parse("type:beer .rating > 4.5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	run, err := exec.Run(q, RunRequest{IDsOnly: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(run.IDs) != 1 || run.IDs[0] != "beer/stout" {
		t.Fatalf("ids = %#v", run.IDs)
	}
}
