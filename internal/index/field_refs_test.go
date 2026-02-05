package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestFieldRefsResolveUnambiguous(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	companyDoc := &parser.ParsedDocument{
		FilePath: "companies/cursor.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "companies/cursor",
				ObjectType: "company",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
	}
	if err := db.IndexDocument(companyDoc, sch); err != nil {
		t.Fatalf("failed to index company: %v", err)
	}

	personDoc := &parser.ParsedDocument{
		FilePath: "people/ada.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "people/ada",
				ObjectType: "person",
				Fields: map[string]schema.FieldValue{
					"company": schema.String("cursor"),
				},
				LineStart: 1,
			},
		},
	}
	if err := db.IndexDocument(personDoc, sch); err != nil {
		t.Fatalf("failed to index person: %v", err)
	}

	if _, err := db.ResolveReferences("daily"); err != nil {
		t.Fatalf("failed to resolve references: %v", err)
	}

	var targetID, status string
	err = db.db.QueryRow(`
		SELECT target_id, resolution_status
		FROM field_refs
		WHERE source_id = ? AND field_name = ?
	`, "people/ada", "company").Scan(&targetID, &status)
	if err != nil {
		t.Fatalf("failed to query field_refs: %v", err)
	}
	if targetID != "companies/cursor" {
		t.Errorf("expected target_id 'companies/cursor', got '%s'", targetID)
	}
	if status != "resolved" {
		t.Errorf("expected status 'resolved', got '%s'", status)
	}
}

func TestFieldRefsResolveAmbiguous(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	companyDocs := []*parser.ParsedDocument{
		{
			FilePath: "companies/cursor.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "companies/cursor",
					ObjectType: "company",
					Fields:     map[string]schema.FieldValue{},
					LineStart:  1,
				},
			},
		},
		{
			FilePath: "orgs/cursor.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "orgs/cursor",
					ObjectType: "company",
					Fields:     map[string]schema.FieldValue{},
					LineStart:  1,
				},
			},
		},
	}
	for _, doc := range companyDocs {
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index company: %v", err)
		}
	}

	personDoc := &parser.ParsedDocument{
		FilePath: "people/ada.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "people/ada",
				ObjectType: "person",
				Fields: map[string]schema.FieldValue{
					"company": schema.String("cursor"),
				},
				LineStart: 1,
			},
		},
	}
	if err := db.IndexDocument(personDoc, sch); err != nil {
		t.Fatalf("failed to index person: %v", err)
	}

	if _, err := db.ResolveReferences("daily"); err != nil {
		t.Fatalf("failed to resolve references: %v", err)
	}

	var targetID *string
	var status string
	err = db.db.QueryRow(`
		SELECT target_id, resolution_status
		FROM field_refs
		WHERE source_id = ? AND field_name = ?
	`, "people/ada", "company").Scan(&targetID, &status)
	if err != nil {
		t.Fatalf("failed to query field_refs: %v", err)
	}
	if targetID != nil {
		t.Errorf("expected target_id to be NULL, got '%s'", *targetID)
	}
	if status != "ambiguous" {
		t.Errorf("expected status 'ambiguous', got '%s'", status)
	}
}
