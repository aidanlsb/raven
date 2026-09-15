package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestFieldRefsResolveUnambiguous(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	companyDoc := &parser.ParsedDocument{
		FilePath: "companies/cursor.md",
		Objects: []*model.Object{
			{
				ID:        "companies/cursor",
				Type:      "company",
				Fields:    map[string]fieldvalue.FieldValue{},
				LineStart: 1,
			},
		},
	}
	if err := db.IndexDocument(companyDoc, sch); err != nil {
		t.Fatalf("failed to index company: %v", err)
	}

	personDoc := &parser.ParsedDocument{
		FilePath: "people/ada.md",
		Objects: []*model.Object{
			{
				ID:   "people/ada",
				Type: "person",
				Fields: map[string]fieldvalue.FieldValue{
					"company": fieldvalue.String("cursor"),
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
		FROM refs
		WHERE source_id = ? AND field_name = ?
	`, "people/ada", "company").Scan(&targetID, &status)
	if err != nil {
		t.Fatalf("failed to query field ref: %v", err)
	}
	if targetID != "companies/cursor" {
		t.Errorf("expected target_id 'companies/cursor', got '%s'", targetID)
	}
	if status != "resolved" {
		t.Errorf("expected status 'resolved', got '%s'", status)
	}

	general, err := db.Backlinks("companies/cursor")
	if err != nil {
		t.Fatalf("Backlinks() error = %v", err)
	}
	if len(general) != 1 || general[0].SourceID != "people/ada" {
		t.Fatalf("Backlinks() = %#v, want people/ada", general)
	}

	outlinks, err := db.Outlinks("people/ada")
	if err != nil {
		t.Fatalf("Outlinks() error = %v", err)
	}
	if len(outlinks) != 1 || outlinks[0].TargetRaw != "cursor" {
		t.Fatalf("Outlinks() = %#v, want cursor", outlinks)
	}

	var refCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM refs WHERE source_id = ?`, "people/ada").Scan(&refCount); err != nil {
		t.Fatalf("count refs: %v", err)
	}
	if refCount != 1 {
		t.Fatalf("indexed %d refs, want 1", refCount)
	}

	backlinks, err := db.FieldBacklinksWithRoots("companies/cursor", "", "")
	if err != nil {
		t.Fatalf("FieldBacklinksWithRoots() error = %v", err)
	}
	if len(backlinks) != 1 || backlinks[0].SourceID != "people/ada" || backlinks[0].FieldName != "company" {
		t.Fatalf("FieldBacklinksWithRoots() = %#v, want people/ada.company", backlinks)
	}
}

func TestFieldRefsResolveAmbiguous(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	companyDocs := []*parser.ParsedDocument{
		{
			FilePath: "companies/cursor.md",
			Objects: []*model.Object{
				{
					ID:        "companies/cursor",
					Type:      "company",
					Fields:    map[string]fieldvalue.FieldValue{},
					LineStart: 1,
				},
			},
		},
		{
			FilePath: "orgs/cursor.md",
			Objects: []*model.Object{
				{
					ID:        "orgs/cursor",
					Type:      "company",
					Fields:    map[string]fieldvalue.FieldValue{},
					LineStart: 1,
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
		Objects: []*model.Object{
			{
				ID:   "people/ada",
				Type: "person",
				Fields: map[string]fieldvalue.FieldValue{
					"company": fieldvalue.String("cursor"),
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
		FROM refs
		WHERE source_id = ? AND field_name = ?
	`, "people/ada", "company").Scan(&targetID, &status)
	if err != nil {
		t.Fatalf("failed to query field ref: %v", err)
	}
	if targetID != nil {
		t.Errorf("expected target_id to be NULL, got '%s'", *targetID)
	}
	if status != "ambiguous" {
		t.Errorf("expected status 'ambiguous', got '%s'", status)
	}
}

func TestIndexStoresEachSemanticReferenceOnce(t *testing.T) {
	t.Parallel()

	sch := personCompanySchema()
	tests := []struct {
		name      string
		content   string
		wantField string
		wantRaw   string
	}{
		{
			name: "wikilink in ref field",
			content: `---
type: person
company: "[[companies/cursor]]"
---
`,
			wantField: "company",
			wantRaw:   "companies/cursor",
		},
		{
			name: "bare ref field",
			content: `---
type: person
company: cursor
---
`,
			wantField: "company",
			wantRaw:   "cursor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, err := OpenInMemory()
			if err != nil {
				t.Fatalf("open database: %v", err)
			}
			defer db.Close()
			db.SetAutoResolveRefs(false)

			doc, err := parser.ParseDocument(tt.content, "people/ada.md", "/vault")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := db.IndexDocument(doc, sch); err != nil {
				t.Fatalf("index: %v", err)
			}

			rows, err := db.db.Query(`
				SELECT field_name, target_raw FROM refs WHERE source_id = ? ORDER BY id
			`, "people/ada")
			if err != nil {
				t.Fatalf("query refs: %v", err)
			}
			defer rows.Close()

			type row struct {
				fieldName *string
				targetRaw string
			}
			var got []row
			for rows.Next() {
				var r row
				if err := rows.Scan(&r.fieldName, &r.targetRaw); err != nil {
					t.Fatalf("scan: %v", err)
				}
				got = append(got, r)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("indexed %d refs %#v, want 1", len(got), got)
			}
			if got[0].fieldName == nil || *got[0].fieldName != tt.wantField {
				t.Fatalf("field_name = %v, want %q", got[0].fieldName, tt.wantField)
			}
			if got[0].targetRaw != tt.wantRaw {
				t.Fatalf("target_raw = %q, want %q", got[0].targetRaw, tt.wantRaw)
			}
		})
	}
}

func TestIndexKeepsBodyWikilinkAndFieldRefSeparate(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	content := `---
type: person
company: cursor
---

See [[cursor]] as well.
`
	doc, err := parser.ParseDocument(content, "people/ada.md", "/vault")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := db.IndexDocument(doc, personCompanySchema()); err != nil {
		t.Fatalf("index: %v", err)
	}

	var fieldCount, markdownCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM refs WHERE source_id = ? AND field_name = 'company'`, "people/ada").Scan(&fieldCount); err != nil {
		t.Fatalf("count field refs: %v", err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM refs WHERE source_id = ? AND field_name IS NULL`, "people/ada").Scan(&markdownCount); err != nil {
		t.Fatalf("count markdown refs: %v", err)
	}
	if fieldCount != 1 || markdownCount != 1 {
		t.Fatalf("field=%d markdown=%d, want 1 and 1", fieldCount, markdownCount)
	}
}

func personCompanySchema() *schema.Schema {
	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}
	return sch
}
