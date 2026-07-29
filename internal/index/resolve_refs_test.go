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
		INSERT INTO field_refs (
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
	if initial.Total != 1 || initial.Unresolved != 1 {
		t.Fatalf("body ref result = %+v, want one unresolved ref", initial)
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
	if healed.Total != 1 || healed.Resolved != 1 {
		t.Fatalf("body ref result = %+v, want one resolved ref", healed)
	}
	if healed.FieldTotal != 1 || healed.FieldResolved != 1 {
		t.Fatalf("field ref result = %+v, want one resolved field ref", healed)
	}

	var bodyTargetID, fieldTargetID, fieldStatus string
	if err := db.db.QueryRow(`
		SELECT r.target_id, fr.target_id, fr.resolution_status
		FROM refs r
		CROSS JOIN field_refs fr
		WHERE r.target_raw = ? AND fr.target_raw = ?
	`, targetRaw, targetRaw).Scan(&bodyTargetID, &fieldTargetID, &fieldStatus); err != nil {
		t.Fatalf("query healed references: %v", err)
	}
	if bodyTargetID != fieldTargetID || bodyTargetID != "companies/cursor" {
		t.Errorf("healed targets = (%q, %q), want both %q", bodyTargetID, fieldTargetID, "companies/cursor")
	}
	if fieldStatus != resolutionStatusResolved {
		t.Errorf("field ref status = %q, want %q", fieldStatus, resolutionStatusResolved)
	}
}
