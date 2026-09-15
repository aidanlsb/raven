package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestReferenceTablesHealSameTarget(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	const targetRaw = "cursor"
	if _, err := db.db.Exec(`
		INSERT INTO refs (source_id, target_raw, file_path)
		VALUES ('notes/source', ?, 'notes/source.md');
		INSERT INTO refs (
			source_id, field_name, target_raw, target_id, resolution_status, file_path
		)
		VALUES ('notes/source', 'company', ?, NULL, 'missing', 'notes/source.md')
	`, targetRaw, targetRaw); err != nil {
		t.Fatalf("insert unresolved references: %v", err)
	}

	initial, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("resolve missing references: %v", err)
	}
	if initial.Total != 2 || initial.Unresolved != 2 {
		t.Fatalf("result = %+v, want two unresolved refs", initial)
	}
	if initial.FieldTotal != 1 || initial.FieldUnresolved != 1 {
		t.Fatalf("field ref result = %+v, want one unresolved field ref", initial)
	}

	targetDoc := &parser.ParsedDocument{
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
	if err := db.IndexDocument(targetDoc, schema.New()); err != nil {
		t.Fatalf("index target: %v", err)
	}

	healed, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("heal references: %v", err)
	}
	if healed.Total != 2 || healed.Resolved != 2 {
		t.Fatalf("result = %+v, want two resolved refs", healed)
	}
	if healed.FieldTotal != 1 || healed.FieldResolved != 1 {
		t.Fatalf("field ref result = %+v, want one resolved field ref", healed)
	}

	var bodyTargetID, fieldTargetID, bodyStatus, fieldStatus string
	if err := db.db.QueryRow(`
		SELECT r.target_id, r.resolution_status
		FROM refs r
		WHERE r.target_raw = ? AND r.field_name IS NULL
	`, targetRaw).Scan(&bodyTargetID, &bodyStatus); err != nil {
		t.Fatalf("query healed body reference: %v", err)
	}
	if err := db.db.QueryRow(`
		SELECT r.target_id, r.resolution_status
		FROM refs r
		WHERE r.target_raw = ? AND r.field_name = 'company'
	`, targetRaw).Scan(&fieldTargetID, &fieldStatus); err != nil {
		t.Fatalf("query healed field reference: %v", err)
	}
	if bodyTargetID != fieldTargetID || bodyTargetID != "companies/cursor" {
		t.Errorf("healed targets = (%q, %q), want both %q", bodyTargetID, fieldTargetID, "companies/cursor")
	}
	if bodyStatus != resolutionStatusResolved || fieldStatus != resolutionStatusResolved {
		t.Errorf("statuses = (%q, %q), want both %q", bodyStatus, fieldStatus, resolutionStatusResolved)
	}
}

func TestMarkdownRefsRecordAmbiguousStatus(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.New()
	for _, doc := range []*parser.ParsedDocument{
		{
			FilePath: "companies/cursor.md",
			Objects: []*model.Object{{
				ID: "companies/cursor", Type: "company",
				Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1,
			}},
		},
		{
			FilePath: "orgs/cursor.md",
			Objects: []*model.Object{{
				ID: "orgs/cursor", Type: "company",
				Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1,
			}},
		},
		{
			FilePath: "notes/source.md",
			Objects: []*model.Object{{
				ID: "notes/source", Type: "page",
				Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1,
			}},
			Refs: []*model.Reference{{
				SourceID: "notes/source", TargetRaw: "cursor", Line: model.IntPtr(3),
			}},
		},
	} {
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("index %s: %v", doc.FilePath, err)
		}
	}

	result, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("resolve references: %v", err)
	}
	if result.Total != 1 || result.Ambiguous != 1 || result.FieldTotal != 0 {
		t.Fatalf("result = %+v, want one ambiguous markdown ref", result)
	}

	var targetID *string
	var status string
	if err := db.db.QueryRow(`
		SELECT target_id, resolution_status FROM refs WHERE source_id = ? AND field_name IS NULL
	`, "notes/source").Scan(&targetID, &status); err != nil {
		t.Fatalf("query markdown ref: %v", err)
	}
	if targetID != nil {
		t.Errorf("target_id = %q, want NULL", *targetID)
	}
	if status != resolutionStatusAmbiguous {
		t.Errorf("status = %q, want %q", status, resolutionStatusAmbiguous)
	}
}
