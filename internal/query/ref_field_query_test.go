package query

import (
	"encoding/json"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestRefFieldQueryResolvesCanonicalTargets(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	personFields, _ := json.Marshal(map[string]interface{}{
		"company": "cursor",
	})

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('people/ada', 'people/ada.md', 'person', ?, 1)
	`, string(personFields))
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES ('people/ada', 'company', 'companies/cursor', 'cursor', 'resolved', 'people/ada.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	sch := schema.NewSchema()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	executor := NewExecutor(db)
	executor.SetSchema(sch)

	q, err := Parse("object:person .company==[[companies/cursor]]")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err := executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "people/ada" {
		t.Fatalf("expected people/ada, got %+v", results)
	}

	q, err = Parse("object:person .company==cursor")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err = executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "people/ada" {
		t.Fatalf("expected people/ada for shorthand query, got %+v", results)
	}
}

func TestRefFieldQueryErrorsOnAmbiguousStoredValue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	personFields, _ := json.Marshal(map[string]interface{}{
		"company": "cursor",
	})

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('orgs/cursor', 'orgs/cursor.md', 'company', '{}', 1),
			('people/ada', 'people/ada.md', 'person', ?, 1)
	`, string(personFields))
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES ('people/ada', 'company', NULL, 'cursor', 'ambiguous', 'people/ada.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	sch := schema.NewSchema()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	executor := NewExecutor(db)
	executor.SetSchema(sch)

	q, err := Parse("object:person .company==[[companies/cursor]]")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if _, err := executor.ExecuteObjectQuery(q); err == nil {
		t.Fatal("expected error for ambiguous stored ref, got nil")
	}
}
