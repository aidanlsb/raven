package query

import (
	"database/sql"
	"encoding/json"
	"sort"
	"testing"

	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/schema"
)

func setupRelationshipDB(t *testing.T) (*sql.DB, *schema.Schema) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(indexschema.SchemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}

	mustJSON := func(v map[string]interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json: %v", err)
		}
		return string(b)
	}

	_, err = db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('company/acme', 'company/acme.md', 'company', ?, 1),
			('company/beta', 'company/beta.md', 'company', ?, 1),
			('project/web', 'project/web.md', 'project', ?, 1),
			('project/old', 'project/old.md', 'project', ?, 1),
			('person/ada', 'person/ada.md', 'person', ?, 1),
			('person/bob', 'person/bob.md', 'person', ?, 1),
			('task/one', 'task/one.md', 'task', ?, 1),
			('task/two', 'task/two.md', 'task', ?, 1);

		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
			('related1', 'person/ada.md', 'person/ada', 'related', 'person/bob', 'Knows bob', 8);

		INSERT INTO refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number) VALUES
			('task/one', 'project', 'project/web', 'project/web', 'resolved', 'task/one.md', 1),
			('task/one', 'owner', 'person/ada', 'person/ada', 'resolved', 'task/one.md', 1),
			('task/one', 'collaborators', 'person/ada', 'person/ada', 'resolved', 'task/one.md', 1),
			('task/one', 'collaborators', 'person/bob', 'person/bob', 'resolved', 'task/one.md', 1),
			('task/two', 'project', 'project/old', 'project/old', 'resolved', 'task/two.md', 1),
			('task/two', 'owner', 'person/bob', 'person/bob', 'resolved', 'task/two.md', 1),
			('project/web', 'company', 'company/acme', 'company/acme', 'resolved', 'project/web.md', 1),
			('project/old', 'company', 'company/beta', 'company/beta', 'resolved', 'project/old.md', 1),
			('person/bob', 'lead', 'person/ada', 'person/ada', 'resolved', 'person/bob.md', 1),
			('person/ada', 'related', 'person/bob', 'person/bob', 'resolved', 'person/ada.md', 8);
	`,
		mustJSON(map[string]interface{}{"name": "Acme", "tier": 1}),
		mustJSON(map[string]interface{}{"name": "Beta", "tier": 2}),
		mustJSON(map[string]interface{}{"name": "Website", "status": "active"}),
		mustJSON(map[string]interface{}{"name": "Legacy", "status": "paused"}),
		mustJSON(map[string]interface{}{"name": "Ada", "team": "infra"}),
		mustJSON(map[string]interface{}{"name": "Bob", "team": "platform"}),
		mustJSON(map[string]interface{}{"status": "open", "due": "2026-02-10", "created": "2026-01-01"}),
		mustJSON(map[string]interface{}{"status": "done", "due": "2026-01-02", "created": "2026-01-10"}),
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	return db, relationshipSchema()
}

func queryIDs(t *testing.T, e *Executor, qstr string) []string {
	t.Helper()
	q, err := Parse(qstr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", qstr, err)
	}
	if err := NewValidator(e.schema).Validate(q); err != nil {
		t.Fatalf("Validate(%q): %v", qstr, err)
	}
	run, err := e.Run(q, RunRequest{})
	if err != nil {
		t.Fatalf("Run(%q): %v", qstr, err)
	}
	ids := make([]string, 0, len(run.Objects)+len(run.Traits))
	for _, obj := range run.Objects {
		ids = append(ids, obj.ID)
	}
	for _, tr := range run.Traits {
		ids = append(ids, tr.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestRelationshipPathQueries(t *testing.T) {
	t.Parallel()
	db, sch := setupRelationshipDB(t)
	defer db.Close()
	e := NewExecutor(db)
	e.SetSchema(sch)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "forward one hop",
			query: "type:task .project.status==active",
			want:  []string{"task/one"},
		},
		{
			name:  "forward two hops",
			query: "type:task .project.company.tier==1",
			want:  []string{"task/one"},
		},
		{
			name:  "existing id comparison",
			query: "type:task .project==[[project/web]]",
			want:  []string{"task/one"},
		},
		{
			name:  "includes on path",
			query: `type:task includes(.project.name, "web")`,
			want:  []string{"task/one"},
		},
		{
			name:  "exists on path",
			query: "type:task exists(.project.name)",
			want:  []string{"task/one", "task/two"},
		},
		{
			name:  "oneof on path",
			query: "type:task oneof(.project.status, [active, backlog])",
			want:  []string{"task/one"},
		},
		{
			name:  "array any ref element field",
			query: "type:task any(.collaborators, .team==infra)",
			want:  []string{"task/one"},
		},
		{
			name:  "reverse named edge",
			query: "type:person refd(type:task .owner==this .status==open)",
			want:  []string{"person/ada"},
		},
		{
			name:  "this.field",
			query: "type:person refd(type:task .owner==this.lead)",
			want:  []string{"person/bob"},
		},
		{
			name:  "multi-hop reverse flatten",
			query: "type:company refd(type:task .project.company==this)",
			want:  []string{"company/acme", "company/beta"},
		},
		{
			name:  "array element equals this",
			query: "type:person refd(type:task any(.collaborators, _==this))",
			want:  []string{"person/ada", "person/bob"},
		},
		{
			name:  "rhs path inequality",
			query: "type:task .due>.created",
			want:  []string{"task/one"},
		},
		{
			name:  "rhs path equality no match",
			query: "type:task .due==.created",
			want:  []string{},
		},
		{
			name:  "untyped refs unchanged",
			query: "type:task refs(type:project .status==active)",
			want:  []string{"task/one"},
		},
		{
			name:  "untyped refd unchanged",
			query: "type:person refd(type:task)",
			want:  []string{"person/ada", "person/bob"},
		},
		{
			name:  "nested refs returns rows",
			query: "type:task refs(type:project refs(type:company .tier==1))",
			want:  []string{"task/one"},
		},
		{
			name:  "nested refd returns rows",
			query: "type:company refd(type:project refd(type:task))",
			want:  []string{"company/acme", "company/beta"},
		},
		{
			name:  "bare trait ref via refd",
			query: "type:person refd(trait:related)",
			want:  []string{"person/bob"},
		},
		{
			name:  "bare trait ref value equals this",
			query: "type:person refd(trait:related .value==this)",
			want:  []string{"person/bob"},
		},
		{
			name:  "absence of reverse edge",
			query: "type:person !refd(type:task .owner==this)",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryIDs(t, e, tt.query)
			want := append([]string{}, tt.want...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("ids = %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("ids = %v, want %v", got, want)
				}
			}
		})
	}
}

func TestNestedRefsDoNotReturnEmpty(t *testing.T) {
	t.Parallel()
	db, sch := setupRelationshipDB(t)
	defer db.Close()
	e := NewExecutor(db)
	e.SetSchema(sch)

	ids := queryIDs(t, e, "type:task refs(type:project refs(type:company .tier==1))")
	if len(ids) != 1 || ids[0] != "task/one" {
		t.Fatalf("nested refs = %v, want [task/one]", ids)
	}
}
