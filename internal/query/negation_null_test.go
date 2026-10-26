package query

import (
	"slices"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestBooleanNegationIncludesUnsetScalarFields(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('issue/error', 'issue/error.md', 'issue', '{"category":"error"}', 1),
			('issue/other', 'issue/other.md', 'issue', '{"category":"docs"}', 1),
			('issue/unset', 'issue/unset.md', 'issue', '{}', 1);
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	exec := NewExecutor(db)
	exec.SetSchema(&schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"issue": {
				Fields: map[string]*schema.FieldDefinition{
					"category": {Type: schema.FieldTypeString},
				},
			},
		},
	})

	ids := func(t *testing.T, q string) []string {
		t.Helper()
		parsed, parseErr := Parse(q)
		if parseErr != nil {
			t.Fatalf("parse %q: %v", q, parseErr)
		}
		run, runErr := exec.Run(parsed, RunRequest{IDsOnly: true})
		if runErr != nil {
			t.Fatalf("run %q: %v", q, runErr)
		}
		out := append([]string(nil), run.IDs...)
		slices.Sort(out)
		return out
	}

	all := ids(t, "type:issue")
	matching := ids(t, "type:issue .category == error")
	negated := ids(t, "type:issue !(.category == error)")
	inequality := ids(t, "type:issue .category != error")

	if !slices.Equal(matching, []string{"issue/error"}) {
		t.Fatalf("P = %#v, want [issue/error]", matching)
	}
	if !slices.Equal(negated, []string{"issue/other", "issue/unset"}) {
		t.Fatalf("!P = %#v, want unset rows included", negated)
	}
	if got, want := len(matching)+len(negated), len(all); got != want {
		t.Fatalf("P + !P = %d, all = %d", got, want)
	}
	if slices.Contains(inequality, "issue/unset") {
		t.Fatalf(".category != error should exclude unset rows, got %#v", inequality)
	}
	if !slices.Equal(inequality, []string{"issue/other"}) {
		t.Fatalf(".category != error = %#v, want [issue/other]", inequality)
	}
}

func TestBooleanNegationIncludesUnsetTraitValues(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('t-error', 'issue/error.md', 'issue/error', 'category', 'error', 'error', 2),
			('t-other', 'issue/other.md', 'issue/other', 'category', 'docs', 'docs', 2),
			('t-unset', 'issue/unset.md', 'issue/unset', 'category', NULL, 'unset', 2);
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	exec := NewExecutor(db)
	ids := func(t *testing.T, q string) []string {
		t.Helper()
		parsed, parseErr := Parse(q)
		if parseErr != nil {
			t.Fatalf("parse %q: %v", q, parseErr)
		}
		run, runErr := exec.Run(parsed, RunRequest{IDsOnly: true})
		if runErr != nil {
			t.Fatalf("run %q: %v", q, runErr)
		}
		out := append([]string(nil), run.IDs...)
		slices.Sort(out)
		return out
	}

	all := ids(t, "trait:category")
	matching := ids(t, "trait:category .value == error")
	negated := ids(t, "trait:category !(.value == error)")
	if got, want := len(matching)+len(negated), len(all); got != want {
		t.Fatalf("P + !P = %d, all = %d (P=%#v !P=%#v all=%#v)", got, want, matching, negated, all)
	}
	if !slices.Contains(negated, "t-unset") {
		t.Fatalf("!P = %#v, want unset trait included", negated)
	}
}
