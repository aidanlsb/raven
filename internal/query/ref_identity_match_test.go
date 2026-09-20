package query

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/schema"
)

func setupRefIdentityMatchDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec(indexschema.SchemaSQL); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('companies/acme', 'companies/acme.md', 'company', '{}', 1),
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('notes/resolved-acme', 'notes/resolved-acme.md', 'note', '{}', 1),
			('notes/resolved-stale-raw', 'notes/resolved-stale-raw.md', 'note', '{}', 1),
			('notes/unresolved-shorthand', 'notes/unresolved-shorthand.md', 'note', '{}', 1),
			('notes/unresolved-canonical', 'notes/unresolved-canonical.md', 'note', '{}', 1),
			('staff/resolved-acme', 'staff/resolved-acme.md', 'employee', '{"company":"acme"}', 1),
			('staff/resolved-stale-raw', 'staff/resolved-stale-raw.md', 'employee', '{"company":"cursor"}', 1),
			('staff/unresolved-shorthand', 'staff/unresolved-shorthand.md', 'employee', '{"company":"ghost"}', 1),
			('staff/unresolved-canonical', 'staff/unresolved-canonical.md', 'employee', '{"company":"companies/acme"}', 1),
			('tasks/resolved-stale-raw', 'tasks/resolved-stale-raw.md', 'task', '{"owners":["cursor"]}', 1);

		INSERT INTO refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number) VALUES
			('notes/resolved-acme', NULL, 'companies/acme', 'acme', 'resolved', 'notes/resolved-acme.md', 5),
			('notes/resolved-stale-raw', NULL, 'companies/acme', 'cursor', 'resolved', 'notes/resolved-stale-raw.md', 5),
			('notes/unresolved-shorthand', NULL, NULL, 'ghost', 'missing', 'notes/unresolved-shorthand.md', 5),
			('notes/unresolved-canonical', NULL, NULL, 'companies/acme', 'missing', 'notes/unresolved-canonical.md', 5),
			('staff/resolved-acme', 'company', 'companies/acme', 'acme', 'resolved', 'staff/resolved-acme.md', 1),
			('staff/resolved-stale-raw', 'company', 'companies/acme', 'cursor', 'resolved', 'staff/resolved-stale-raw.md', 1),
			('staff/unresolved-shorthand', 'company', NULL, 'ghost', 'missing', 'staff/unresolved-shorthand.md', 1),
			('staff/unresolved-canonical', 'company', NULL, 'companies/acme', 'missing', 'staff/unresolved-canonical.md', 1),
			('tasks/resolved-stale-raw', 'owners', 'companies/acme', 'cursor', 'resolved', 'tasks/resolved-stale-raw.md', 1);
	`)
	if err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	return db
}

func refIdentityMatchSchema() *schema.Schema {
	sch := schema.New()
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}
	sch.Types["note"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}
	sch.Types["employee"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["task"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"owners": {Type: schema.FieldTypeRefArray, Target: "company"},
		},
	}
	return sch
}

func TestRefIdentityMatch_FieldAndBodyShareResolvedIDPreferredSemantics(t *testing.T) {
	t.Parallel()
	db := setupRefIdentityMatchDB(t)
	defer db.Close()

	executor := NewExecutor(db)
	executor.SetSchema(refIdentityMatchSchema())

	tests := []struct {
		name    string
		query   string
		wantIDs map[string]bool
	}{
		{
			name:    "field shorthand matches resolved rows on target_id",
			query:   "type:employee .company==acme",
			wantIDs: map[string]bool{"staff/resolved-acme": true, "staff/resolved-stale-raw": true},
		},
		{
			name:    "field canonical id matches resolved rows and unresolved canonical raw",
			query:   "type:employee .company==[[companies/acme]]",
			wantIDs: map[string]bool{"staff/resolved-acme": true, "staff/resolved-stale-raw": true, "staff/unresolved-canonical": true},
		},
		{
			name:    "field ignores target_raw on a resolved row",
			query:   "type:employee .company==cursor",
			wantIDs: map[string]bool{},
		},
		{
			name:    "field unresolved row still matches via target_raw",
			query:   "type:employee .company==ghost",
			wantIDs: map[string]bool{"staff/unresolved-shorthand": true},
		},
		{
			name:    "body refs shorthand matches resolved rows on target_id",
			query:   "type:note refs([[acme]])",
			wantIDs: map[string]bool{"notes/resolved-acme": true, "notes/resolved-stale-raw": true},
		},
		{
			name:    "body refs canonical id matches resolved rows and unresolved canonical raw",
			query:   "type:note refs([[companies/acme]])",
			wantIDs: map[string]bool{"notes/resolved-acme": true, "notes/resolved-stale-raw": true, "notes/unresolved-canonical": true},
		},
		{
			name:    "body refs ignores target_raw on a resolved row",
			query:   "type:note refs([[cursor]])",
			wantIDs: map[string]bool{},
		},
		{
			name:    "body refs unresolved row still matches via target_raw",
			query:   "type:note refs([[ghost]])",
			wantIDs: map[string]bool{"notes/unresolved-shorthand": true},
		},
		{
			name:    "body refs subquery uses the same identity-first join",
			query:   "type:note refs(type:company)",
			wantIDs: map[string]bool{"notes/resolved-acme": true, "notes/resolved-stale-raw": true, "notes/unresolved-canonical": true},
		},
		{
			name:    "refd matches the resolved target, not stale target_raw",
			query:   "type:company refd([[notes/resolved-stale-raw]])",
			wantIDs: map[string]bool{"companies/acme": true},
		},
		{
			name:    "refd unresolved row still matches via target_raw",
			query:   "type:company refd([[notes/unresolved-canonical]])",
			wantIDs: map[string]bool{"companies/acme": true},
		},
		{
			name:    "ref array element ignores target_raw on a resolved row",
			query:   `type:task any(.owners, _ == cursor)`,
			wantIDs: map[string]bool{},
		},
		{
			name:    "ref array element matches resolved rows on target_id",
			query:   `type:task any(.owners, _ == acme)`,
			wantIDs: map[string]bool{"tasks/resolved-stale-raw": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			run, err := executor.Run(q, RunRequest{})
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			got := make(map[string]bool, len(run.Objects))
			for _, r := range run.Objects {
				got[r.ID] = true
			}
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got ids %#v, want %#v", got, tt.wantIDs)
			}
			for id := range tt.wantIDs {
				if !got[id] {
					t.Fatalf("missing %s in %#v", id, got)
				}
			}
		})
	}
}
