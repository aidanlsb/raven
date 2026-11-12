package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func relatedPersonSchema() *schema.Schema {
	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
		},
	}
	sch.Traits["related"] = &schema.TraitDefinition{Type: schema.FieldTypeRef}
	return sch
}

func TestBareTraitRefsLandInIndex(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := relatedPersonSchema()
	bob, err := parser.ParseDocument(`---
type: person
name: Bob
---
`, "person/bob.md", "/vault")
	if err != nil {
		t.Fatalf("parse bob: %v", err)
	}
	if err := db.IndexDocument(bob, sch); err != nil {
		t.Fatalf("index bob: %v", err)
	}

	ada, err := parser.ParseDocument(`---
type: person
name: Ada
---

@related(person/bob)
`, "person/ada.md", "/vault")
	if err != nil {
		t.Fatalf("parse ada: %v", err)
	}
	if err := db.IndexDocument(ada, sch); err != nil {
		t.Fatalf("index ada: %v", err)
	}

	if _, err := db.ResolveReferences("daily"); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var fieldName, targetRaw string
	var targetID *string
	err = db.db.QueryRow(`
		SELECT field_name, target_raw, target_id
		FROM refs
		WHERE source_id = ? AND field_name = ?
	`, "person/ada", "related").Scan(&fieldName, &targetRaw, &targetID)
	if err != nil {
		t.Fatalf("query refs: %v", err)
	}
	if fieldName != "related" || targetRaw != "person/bob" {
		t.Fatalf("got field=%q raw=%q, want related / person/bob", fieldName, targetRaw)
	}
	if targetID == nil || *targetID != "person/bob" {
		t.Fatalf("target_id = %v, want person/bob", targetID)
	}
}

func TestExtractSchemaTraitRefsBareAndWikilink(t *testing.T) {
	t.Parallel()

	sch := relatedPersonSchema()
	doc, err := parser.ParseDocument(`---
type: person
name: Ada
---

@related(person/bob)
`, "person/ada.md", "/vault")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	refs := parser.ExtractSchemaTraitRefs(doc.Traits, sch)
	if len(refs) != 1 {
		t.Fatalf("refs = %#v, want 1", refs)
	}
	if refs[0].FieldName != "related" || refs[0].TargetRaw != "person/bob" {
		t.Fatalf("ref = %#v", refs[0])
	}
}
