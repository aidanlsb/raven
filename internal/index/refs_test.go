package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestResolveReferences(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()

	// Index a document with objects
	doc1 := &parser.ParsedDocument{
		FilePath:   "people/freya.md",
		RawContent: "# Freya",
		Objects: []*parser.ParsedObject{
			{
				ID:         "people/freya",
				ObjectType: "person",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
	}
	if err := db.IndexDocument(doc1, sch); err != nil {
		t.Fatalf("failed to index doc1: %v", err)
	}

	// Index a document with references
	doc2 := &parser.ParsedDocument{
		FilePath:   "daily/2025-02-01.md",
		RawContent: "Met with [[people/freya]] and [[thor]]",
		Objects: []*parser.ParsedObject{
			{
				ID:         "daily/2025-02-01",
				ObjectType: "date",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
		Refs: []*parser.ParsedRef{
			{
				SourceID:  "daily/2025-02-01",
				TargetRaw: "people/freya",
				Line:      1,
			},
			{
				SourceID:  "daily/2025-02-01",
				TargetRaw: "thor", // Short reference - should not resolve (ambiguous or not found)
				Line:      1,
			},
		},
	}
	if err := db.IndexDocument(doc2, sch); err != nil {
		t.Fatalf("failed to index doc2: %v", err)
	}

	// Index another person for short name collision test
	doc3 := &parser.ParsedDocument{
		FilePath:   "people/thor.md",
		RawContent: "# Thor",
		Objects: []*parser.ParsedObject{
			{
				ID:         "people/thor",
				ObjectType: "person",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
	}
	if err := db.IndexDocument(doc3, sch); err != nil {
		t.Fatalf("failed to index doc3: %v", err)
	}

	// Resolve references
	result, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("failed to resolve references: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected 2 total refs, got %d", result.Total)
	}

	// Both should resolve now: people/freya (exact) and thor (unique short match)
	if result.Resolved != 2 {
		t.Errorf("expected 2 resolved refs, got %d", result.Resolved)
	}

	if result.Unresolved != 0 {
		t.Errorf("expected 0 unresolved refs, got %d", result.Unresolved)
	}

	// Verify target_id was set correctly
	var targetID string
	err = db.db.QueryRow(`SELECT target_id FROM refs WHERE target_raw = ?`, "people/freya").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs: %v", err)
	}
	if targetID != "people/freya" {
		t.Errorf("expected target_id 'people/freya', got '%s'", targetID)
	}

	// Verify short reference resolved
	err = db.db.QueryRow(`SELECT target_id FROM refs WHERE target_raw = ?`, "thor").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs for thor: %v", err)
	}
	if targetID != "people/thor" {
		t.Errorf("expected target_id 'people/thor', got '%s'", targetID)
	}
}

func TestResolveReferences_DateShorthand(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()

	// Index a daily note
	doc1 := &parser.ParsedDocument{
		FilePath:   "daily/2025-02-01.md",
		RawContent: "# Feb 1",
		Objects: []*parser.ParsedObject{
			{
				ID:         "daily/2025-02-01",
				ObjectType: "date",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
	}
	if err := db.IndexDocument(doc1, sch); err != nil {
		t.Fatalf("failed to index doc1: %v", err)
	}

	// Index a document that references the date using shorthand
	doc2 := &parser.ParsedDocument{
		FilePath:   "projects/test.md",
		RawContent: "See [[2025-02-01]]",
		Objects: []*parser.ParsedObject{
			{
				ID:         "projects/test",
				ObjectType: "project",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
		Refs: []*parser.ParsedRef{
			{
				SourceID:  "projects/test",
				TargetRaw: "2025-02-01", // Date shorthand
				Line:      1,
			},
		},
	}
	if err := db.IndexDocument(doc2, sch); err != nil {
		t.Fatalf("failed to index doc2: %v", err)
	}

	// Resolve references with "daily" as the daily directory
	result, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("failed to resolve references: %v", err)
	}

	if result.Resolved != 1 {
		t.Errorf("expected 1 resolved ref, got %d", result.Resolved)
	}

	// Verify target_id was set correctly
	var targetID string
	err = db.db.QueryRow(`SELECT target_id FROM refs WHERE target_raw = ?`, "2025-02-01").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs: %v", err)
	}
	if targetID != "daily/2025-02-01" {
		t.Errorf("expected target_id 'daily/2025-02-01', got '%s'", targetID)
	}
}

func TestResolveReferences_UnresolvedRef(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()

	// Index a document with a broken reference
	doc := &parser.ParsedDocument{
		FilePath:   "test.md",
		RawContent: "See [[nonexistent]]",
		Objects: []*parser.ParsedObject{
			{
				ID:         "test",
				ObjectType: "page",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
		Refs: []*parser.ParsedRef{
			{
				SourceID:  "test",
				TargetRaw: "nonexistent",
				Line:      1,
			},
		},
	}
	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("failed to index doc: %v", err)
	}

	// Resolve references
	result, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("failed to resolve references: %v", err)
	}

	if result.Resolved != 0 {
		t.Errorf("expected 0 resolved refs, got %d", result.Resolved)
	}

	if result.Unresolved != 1 {
		t.Errorf("expected 1 unresolved ref, got %d", result.Unresolved)
	}

	// Verify target_id is still NULL
	var targetID *string
	err = db.db.QueryRow(`SELECT target_id FROM refs WHERE target_raw = ?`, "nonexistent").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs: %v", err)
	}
	if targetID != nil {
		t.Errorf("expected target_id to be NULL, got '%s'", *targetID)
	}
}
