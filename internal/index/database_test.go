package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestDatabase(t *testing.T) {
	// Create a minimal schema for testing
	sch := schema.NewSchema()

	t.Run("initialization", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		stats, err := db.Stats()
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}

		if stats.ObjectCount != 0 {
			t.Errorf("expected 0 objects, got %d", stats.ObjectCount)
		}
	})

	t.Run("index document", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "test",
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
			Traits: []*parser.ParsedTrait{},
			Refs:   []*parser.ParsedRef{},
		}

		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		stats, err := db.Stats()
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}

		if stats.ObjectCount != 1 {
			t.Errorf("expected 1 object, got %d", stats.ObjectCount)
		}

		if stats.FileCount != 1 {
			t.Errorf("expected 1 file, got %d", stats.FileCount)
		}
	})

	t.Run("reindex replaces data", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "test",
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
		}

		// Index twice
		db.IndexDocument(doc, sch)
		db.IndexDocument(doc, sch)

		stats, _ := db.Stats()
		if stats.ObjectCount != 1 {
			t.Errorf("expected 1 object after reindex, got %d", stats.ObjectCount)
		}
	})

	t.Run("bare boolean trait gets default true value", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		// Create schema with a boolean trait that has default: true
		testSchema := schema.NewSchema()
		testSchema.Traits["highlight"] = &schema.TraitDefinition{
			Type:    schema.FieldTypeBool,
			Default: true,
		}

		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "test",
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "highlight",
					Value:          nil, // Bare trait, no value
					Content:        "This is important",
					Line:           5,
					ParentObjectID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Query for traits with value "true"
		trueFilter := "true"
		results, err := db.QueryTraits("highlight", &trueFilter)
		if err != nil {
			t.Fatalf("failed to query traits: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result for highlight=true, got %d", len(results))
		}

		if len(results) > 0 && (results[0].Value == nil || *results[0].Value != "true") {
			t.Errorf("expected value 'true', got %v", results[0].Value)
		}
	})

	t.Run("bare boolean trait without explicit default gets true", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		// Create schema with a boolean trait (no explicit default)
		testSchema := schema.NewSchema()
		testSchema.Traits["pinned"] = &schema.TraitDefinition{
			Type: schema.FieldTypeBool,
			// No explicit default - boolean traits should default to true when present
		}

		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "test",
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "pinned",
					Value:          nil, // Bare trait
					Content:        "Pinned item",
					Line:           3,
					ParentObjectID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Query for traits with value "true"
		trueFilter := "true"
		results, err := db.QueryTraits("pinned", &trueFilter)
		if err != nil {
			t.Fatalf("failed to query traits: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result for pinned=true, got %d", len(results))
		}
	})

	t.Run("enum trait with default value", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		// Create schema with an enum trait with default
		testSchema := schema.NewSchema()
		testSchema.Traits["priority"] = &schema.TraitDefinition{
			Type:    schema.FieldTypeEnum,
			Values:  []string{"low", "medium", "high"},
			Default: "medium",
		}

		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "test",
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "priority",
					Value:          nil, // Bare trait, should get default "medium"
					Content:        "A task",
					Line:           5,
					ParentObjectID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Query for traits with value "medium"
		mediumFilter := "medium"
		results, err := db.QueryTraits("priority", &mediumFilter)
		if err != nil {
			t.Fatalf("failed to query traits: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result for priority=medium, got %d", len(results))
		}
	})

	t.Run("undefined traits are not indexed", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		// Create schema WITHOUT the "undefined" trait
		testSchema := schema.NewSchema()
		testSchema.Traits["defined"] = &schema.TraitDefinition{
			Type: schema.FieldTypeBool,
		}

		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "test",
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "defined", // This one IS in schema
					Value:          nil,
					Content:        "Defined trait",
					Line:           3,
					ParentObjectID: "test",
				},
				{
					TraitType:      "undefined", // This one is NOT in schema
					Value:          nil,
					Content:        "Undefined trait",
					Line:           5,
					ParentObjectID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Query for defined trait - should find 1
		definedResults, err := db.QueryTraits("defined", nil)
		if err != nil {
			t.Fatalf("failed to query traits: %v", err)
		}
		if len(definedResults) != 1 {
			t.Errorf("expected 1 result for 'defined' trait, got %d", len(definedResults))
		}

		// Query for undefined trait - should find 0 (not indexed)
		undefinedResults, err := db.QueryTraits("undefined", nil)
		if err != nil {
			t.Fatalf("failed to query traits: %v", err)
		}
		if len(undefinedResults) != 0 {
			t.Errorf("expected 0 results for undefined trait, got %d (schema is source of truth)", len(undefinedResults))
		}
	})
}

func TestAllIndexedFilePaths(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.NewSchema()

	// Index a few documents
	files := []string{"people/alice.md", "projects/foo.md", "daily/2025-01-01.md"}
	for _, file := range files {
		doc := &parser.ParsedDocument{
			FilePath: file,
			Objects: []*parser.ParsedObject{
				{
					ID:         file[:len(file)-3], // strip .md
					ObjectType: "page",
					Fields:     make(map[string]schema.FieldValue),
					LineStart:  1,
				},
			},
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index %s: %v", file, err)
		}
	}

	// Get all indexed paths
	paths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("failed to get indexed paths: %v", err)
	}

	if len(paths) != 3 {
		t.Errorf("expected 3 indexed paths, got %d", len(paths))
	}

	// Verify all paths are present
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	for _, f := range files {
		if !pathSet[f] {
			t.Errorf("expected path %s to be in indexed paths", f)
		}
	}
}
