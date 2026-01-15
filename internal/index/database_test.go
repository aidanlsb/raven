package index

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"syscall"
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

	t.Run("remove document resolves file_path from DB (including embedded IDs)", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		doc := &parser.ParsedDocument{
			// Simulate vaults with directories config enabled: object ID does not include "objects/".
			FilePath: "objects/people/freya.md",
			RawContent: `---
---

# Freya

- @highlight Hello`,
			Objects: []*parser.ParsedObject{
				{
					ID:         "people/freya",
					ObjectType: "person",
					Fields:     map[string]schema.FieldValue{},
					LineStart:  1,
				},
				{
					ID:         "people/freya#notes",
					ObjectType: "section",
					Fields: map[string]schema.FieldValue{
						"title": schema.String("Notes"),
						"level": schema.Number(2),
					},
					LineStart: 5,
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "highlight",
					Value:          nil,
					Content:        "Hello",
					ParentObjectID: "people/freya",
					Line:           7,
				},
			},
			Refs: []*parser.ParsedRef{
				{
					SourceID:  "people/freya",
					TargetRaw: "projects/website",
					Line:      8,
					Start:     0,
					End:       0,
				},
			},
		}

		// Define trait so it gets indexed.
		testSchema := schema.NewSchema()
		testSchema.Traits["highlight"] = &schema.TraitDefinition{Type: schema.FieldTypeBool}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Remove by embedded ID (callers may pass file#section).
		if err := db.RemoveDocument("people/freya#notes"); err != nil {
			t.Fatalf("failed to remove document: %v", err)
		}

		// Verify all tables have been cleaned for that file.
		type tableCount struct {
			table string
		}
		for _, tc := range []tableCount{
			{table: "objects"},
			{table: "traits"},
			{table: "refs"},
			{table: "date_index"},
			{table: "fts_content"},
		} {
			var n int
			var q string
			if tc.table == "objects" {
				// objects are removed by id prefix; file_path should be irrelevant after RemoveDocument
				q = "SELECT COUNT(*) FROM objects WHERE id = ? OR id LIKE ?"
				err = db.db.QueryRow(q, "people/freya", "people/freya#%").Scan(&n)
			} else {
				q = "SELECT COUNT(*) FROM " + tc.table + " WHERE file_path = ?"
				err = db.db.QueryRow(q, "objects/people/freya.md").Scan(&n)
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("failed to query %s: %v", tc.table, err)
			}
			if n != 0 {
				t.Fatalf("expected %s rows to be 0, got %d", tc.table, n)
			}
		}
	})

	t.Run("remove document returns not found when missing", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		if err := db.RemoveDocument("people/missing"); !errors.Is(err, ErrObjectNotFound) {
			t.Fatalf("expected ErrObjectNotFound, got %v", err)
		}
	})

	t.Run("resolver includes aliases and daily directory", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		doc := &parser.ParsedDocument{
			FilePath:   "projects/website.md",
			RawContent: "# Website",
			Objects: []*parser.ParsedObject{
				{
					ID:         "projects/website",
					ObjectType: "project",
					Fields: map[string]schema.FieldValue{
						"alias": schema.String("WebSiteAlias"),
					},
					LineStart: 1,
				},
			},
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		res, err := db.Resolver(ResolverOptions{DailyDirectory: "journal"})
		if err != nil {
			t.Fatalf("failed to build resolver: %v", err)
		}

		aliasResolved := res.Resolve("websitealias") // different casing
		if aliasResolved.Ambiguous || aliasResolved.TargetID != "projects/website" {
			t.Fatalf("expected alias to resolve to projects/website, got %+v", aliasResolved)
		}

		dateResolved := res.Resolve("2025-02-01")
		if dateResolved.Ambiguous || dateResolved.TargetID != "journal/2025-02-01" {
			t.Fatalf("expected date shorthand to resolve to journal/2025-02-01, got %+v", dateResolved)
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

	t.Run("backlinks include frontmatter refs", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		freyaContent := `---
type: person
---

# Freya
`
		freyaDoc, err := parser.ParseDocument(freyaContent, "/vault/people/freya.md", "/vault")
		if err != nil {
			t.Fatalf("failed to parse freya doc: %v", err)
		}
		if err := db.IndexDocument(freyaDoc, sch); err != nil {
			t.Fatalf("failed to index freya doc: %v", err)
		}

		alphaContent := `---
type: project
owner: "[[people/freya]]"
---

# Alpha
`
		alphaDoc, err := parser.ParseDocument(alphaContent, "/vault/projects/alpha.md", "/vault")
		if err != nil {
			t.Fatalf("failed to parse alpha doc: %v", err)
		}
		if err := db.IndexDocument(alphaDoc, sch); err != nil {
			t.Fatalf("failed to index alpha doc: %v", err)
		}

		// Frontmatter refs should be indexed into refs table and show up in backlinks.
		bls, err := db.Backlinks("people/freya")
		if err != nil {
			t.Fatalf("failed to query backlinks: %v", err)
		}
		if len(bls) != 1 {
			t.Fatalf("expected 1 backlink, got %d", len(bls))
		}
		if bls[0].SourceID != "projects/alpha" {
			t.Fatalf("SourceID = %q, want %q", bls[0].SourceID, "projects/alpha")
		}
		if bls[0].FilePath != "projects/alpha.md" {
			t.Fatalf("FilePath = %q, want %q", bls[0].FilePath, "projects/alpha.md")
		}
		if bls[0].Line == nil || *bls[0].Line != 3 {
			t.Fatalf("Line = %v, want 3", bls[0].Line)
		}
	})
}

func TestAliasIndexing(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.NewSchema()

	t.Run("alias stored in objects table", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "people/freya",
					ObjectType: "person",
					Fields: map[string]schema.FieldValue{
						"name":  schema.String("Freya"),
						"alias": schema.String("goddess"),
					},
					LineStart: 1,
				},
			},
		}

		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Retrieve aliases
		aliases, err := db.AllAliases()
		if err != nil {
			t.Fatalf("failed to get aliases: %v", err)
		}

		if len(aliases) != 1 {
			t.Errorf("expected 1 alias, got %d", len(aliases))
		}

		if aliases["goddess"] != "people/freya" {
			t.Errorf("expected alias 'goddess' -> 'people/freya', got %v", aliases)
		}
	})

	t.Run("multiple objects with aliases", func(t *testing.T) {
		// Clear database first
		db.ClearAllData()

		docs := []*parser.ParsedDocument{
			{
				FilePath: "people/freya.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/freya",
						ObjectType: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/thor.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/thor",
						ObjectType: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("thunder"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/loki.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/loki",
						ObjectType: "person",
						Fields:     map[string]schema.FieldValue{}, // No alias
						LineStart:  1,
					},
				},
			},
		}

		for _, doc := range docs {
			if err := db.IndexDocument(doc, sch); err != nil {
				t.Fatalf("failed to index document: %v", err)
			}
		}

		aliases, err := db.AllAliases()
		if err != nil {
			t.Fatalf("failed to get aliases: %v", err)
		}

		if len(aliases) != 2 {
			t.Errorf("expected 2 aliases, got %d", len(aliases))
		}

		if aliases["goddess"] != "people/freya" {
			t.Errorf("expected alias 'goddess' -> 'people/freya'")
		}
		if aliases["thunder"] != "people/thor" {
			t.Errorf("expected alias 'thunder' -> 'people/thor'")
		}
	})

	t.Run("resolve references using aliases", func(t *testing.T) {
		// Clear database first
		db.ClearAllData()
		db.SetAutoResolveRefs(false)

		// Index a document with an alias
		doc1 := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "people/freya",
					ObjectType: "person",
					Fields: map[string]schema.FieldValue{
						"alias": schema.String("goddess"),
					},
					LineStart: 1,
				},
			},
		}
		if err := db.IndexDocument(doc1, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Index a document that references the alias
		doc2 := &parser.ParsedDocument{
			FilePath: "notes/meeting.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "notes/meeting",
					ObjectType: "page",
					Fields:     map[string]schema.FieldValue{},
					LineStart:  1,
				},
			},
			Refs: []*parser.ParsedRef{
				{
					SourceID:  "notes/meeting",
					TargetRaw: "goddess", // Reference by alias
					Line:      5,
					Start:     0,
					End:       10,
				},
			},
		}
		if err := db.IndexDocument(doc2, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Resolve references
		result, err := db.ResolveReferences("daily")
		if err != nil {
			t.Fatalf("failed to resolve references: %v", err)
		}

		if result.Resolved != 1 {
			t.Errorf("expected 1 resolved reference, got %d", result.Resolved)
		}
		if result.Unresolved != 0 {
			t.Errorf("expected 0 unresolved references, got %d", result.Unresolved)
		}
	})

	t.Run("detect duplicate aliases", func(t *testing.T) {
		db2, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db2.Close()

		// Index two documents with the SAME alias
		docs := []*parser.ParsedDocument{
			{
				FilePath: "people/freya.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/freya",
						ObjectType: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"), // Same alias
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/frigg.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/frigg",
						ObjectType: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"), // Same alias!
						},
						LineStart: 1,
					},
				},
			},
		}

		for _, doc := range docs {
			if err := db2.IndexDocument(doc, sch); err != nil {
				t.Fatalf("failed to index document: %v", err)
			}
		}

		// Find duplicate aliases
		duplicates, err := db2.FindDuplicateAliases()
		if err != nil {
			t.Fatalf("failed to find duplicate aliases: %v", err)
		}

		if len(duplicates) != 1 {
			t.Errorf("expected 1 duplicate alias, got %d", len(duplicates))
		}

		if len(duplicates) > 0 {
			if duplicates[0].Alias != "goddess" {
				t.Errorf("expected duplicate alias 'goddess', got %q", duplicates[0].Alias)
			}
			if len(duplicates[0].ObjectIDs) != 2 {
				t.Errorf("expected 2 object IDs in conflict, got %d", len(duplicates[0].ObjectIDs))
			}
		}
	})

	t.Run("first alias wins deterministically", func(t *testing.T) {
		db2, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db2.Close()

		// Index two documents with the SAME alias
		// people/freya comes before people/frigg alphabetically
		docs := []*parser.ParsedDocument{
			{
				FilePath: "people/frigg.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/frigg",
						ObjectType: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/freya.md",
				Objects: []*parser.ParsedObject{
					{
						ID:         "people/freya",
						ObjectType: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"),
						},
						LineStart: 1,
					},
				},
			},
		}

		for _, doc := range docs {
			if err := db2.IndexDocument(doc, sch); err != nil {
				t.Fatalf("failed to index document: %v", err)
			}
		}

		// Get aliases - should be deterministic (first alphabetically wins)
		aliases, err := db2.AllAliases()
		if err != nil {
			t.Fatalf("failed to get aliases: %v", err)
		}

		// "people/freya" comes before "people/frigg" alphabetically
		if aliases["goddess"] != "people/freya" {
			t.Errorf("expected 'goddess' -> 'people/freya' (first alphabetically), got %q", aliases["goddess"])
		}
	})

	t.Run("empty alias is not stored", func(t *testing.T) {
		db2, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db2.Close()

		doc := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "people/freya",
					ObjectType: "person",
					Fields: map[string]schema.FieldValue{
						"alias": schema.String(""), // Empty alias
					},
					LineStart: 1,
				},
			},
		}

		if err := db2.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		aliases, err := db2.AllAliases()
		if err != nil {
			t.Fatalf("failed to get aliases: %v", err)
		}

		if len(aliases) != 0 {
			t.Errorf("expected 0 aliases for empty alias field, got %d", len(aliases))
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

func TestOpenWithRebuildLock(t *testing.T) {
	vaultDir := t.TempDir()
	dbDir := filepath.Join(vaultDir, ".raven")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}

	lockPath := filepath.Join(dbDir, "index.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("failed to open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed to acquire test lock: %v", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	if _, _, err := OpenWithRebuild(vaultDir); !errors.Is(err, ErrIndexLocked) {
		t.Fatalf("expected ErrIndexLocked, got %v", err)
	}
}

func TestResolveReferencesBatched(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()

	targetDoc := &parser.ParsedDocument{
		FilePath: "people/freya.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "people/freya",
				ObjectType: "person",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
	}
	if err := db.IndexDocument(targetDoc, sch); err != nil {
		t.Fatalf("failed to index target doc: %v", err)
	}

	refCount := 800
	refs := make([]*parser.ParsedRef, 0, refCount)
	for i := 0; i < refCount; i++ {
		refs = append(refs, &parser.ParsedRef{
			SourceID:  "notes/meeting",
			TargetRaw: "people/freya",
			Line:      i + 1,
		})
	}

	refDoc := &parser.ParsedDocument{
		FilePath: "notes/meeting.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "notes/meeting",
				ObjectType: "page",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
		Refs: refs,
	}
	if err := db.IndexDocument(refDoc, sch); err != nil {
		t.Fatalf("failed to index ref doc: %v", err)
	}

	result, err := db.ResolveReferences("daily")
	if err != nil {
		t.Fatalf("failed to resolve references: %v", err)
	}
	if result.Total != refCount {
		t.Fatalf("expected %d total refs, got %d", refCount, result.Total)
	}
	if result.Resolved != refCount {
		t.Fatalf("expected %d resolved refs, got %d", refCount, result.Resolved)
	}
	if result.Unresolved != 0 {
		t.Fatalf("expected 0 unresolved refs, got %d", result.Unresolved)
	}
}
