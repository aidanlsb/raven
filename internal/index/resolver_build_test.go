package index

import (
	"database/sql"
	"testing"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestAliasIndexing(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()

	t.Run("alias stored in objects table", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*model.Object{
				{
					ID:   "people/freya",
					Type: "person",
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
				Objects: []*model.Object{
					{
						ID:   "people/freya",
						Type: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/thor.md",
				Objects: []*model.Object{
					{
						ID:   "people/thor",
						Type: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("thunder"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/loki.md",
				Objects: []*model.Object{
					{
						ID:        "people/loki",
						Type:      "person",
						Fields:    map[string]schema.FieldValue{}, // No alias
						LineStart: 1,
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
			Objects: []*model.Object{
				{
					ID:   "people/freya",
					Type: "person",
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
			Objects: []*model.Object{
				{
					ID:        "notes/meeting",
					Type:      "page",
					Fields:    map[string]schema.FieldValue{},
					LineStart: 1,
				},
			},
			Refs: []*model.Reference{
				{
					SourceID:      "notes/meeting",
					TargetRaw:     "goddess", // Reference by alias
					Line:          model.IntPtr(5),
					PositionStart: model.IntPtr(0),
					PositionEnd:   model.IntPtr(10),
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
				Objects: []*model.Object{
					{
						ID:   "people/freya",
						Type: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"), // Same alias
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/frigg.md",
				Objects: []*model.Object{
					{
						ID:   "people/frigg",
						Type: "person",
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
				Objects: []*model.Object{
					{
						ID:   "people/frigg",
						Type: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/freya.md",
				Objects: []*model.Object{
					{
						ID:   "people/freya",
						Type: "person",
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

	t.Run("resolver treats duplicate aliases as ambiguous", func(t *testing.T) {
		db2, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db2.Close()

		docs := []*parser.ParsedDocument{
			{
				FilePath: "people/freya.md",
				Objects: []*model.Object{
					{
						ID:   "people/freya",
						Type: "person",
						Fields: map[string]schema.FieldValue{
							"alias": schema.String("goddess"),
						},
						LineStart: 1,
					},
				},
			},
			{
				FilePath: "people/frigg.md",
				Objects: []*model.Object{
					{
						ID:   "people/frigg",
						Type: "person",
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

		res, err := db2.Resolver(ResolverOptions{})
		if err != nil {
			t.Fatalf("failed to build resolver: %v", err)
		}

		result := res.Resolve("goddess")
		if !result.Ambiguous {
			t.Fatalf("expected duplicate alias to be ambiguous, got %+v", result)
		}
		if len(result.Matches) != 2 {
			t.Fatalf("expected 2 matches, got %d", len(result.Matches))
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
			Objects: []*model.Object{
				{
					ID:   "people/freya",
					Type: "person",
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
func TestAllAliasesFromDB_LegacySchemaWithoutAliasColumn(t *testing.T) {
	t.Parallel()

	rawDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer rawDB.Close()

	_, err = rawDB.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		file_path TEXT NOT NULL,
		type TEXT NOT NULL,
		fields TEXT NOT NULL DEFAULT '{}',
		line_start INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create legacy objects table: %v", err)
	}

	aliases, err := allAliasesFromDB(rawDB)
	if err != nil {
		t.Fatalf("allAliasesFromDB returned error: %v", err)
	}
	if len(aliases) != 0 {
		t.Fatalf("expected no aliases from legacy schema, got %v", aliases)
	}

	aliasMatches, err := allAliasMatchesFromDB(rawDB)
	if err != nil {
		t.Fatalf("allAliasMatchesFromDB returned error: %v", err)
	}
	if len(aliasMatches) != 0 {
		t.Fatalf("expected no alias matches from legacy schema, got %v", aliasMatches)
	}
}
func TestResolveReferencesBatched(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.New()

	targetDoc := &parser.ParsedDocument{
		FilePath: "people/freya.md",
		Objects: []*model.Object{
			{
				ID:        "people/freya",
				Type:      "person",
				Fields:    map[string]schema.FieldValue{},
				LineStart: 1,
			},
		},
	}
	if err := db.IndexDocument(targetDoc, sch); err != nil {
		t.Fatalf("failed to index target doc: %v", err)
	}

	refCount := 800
	refs := make([]*model.Reference, 0, refCount)
	for i := 0; i < refCount; i++ {
		refs = append(refs, &model.Reference{
			SourceID:  "notes/meeting",
			TargetRaw: "people/freya",
			Line:      model.IntPtr(i + 1),
		})
	}

	refDoc := &parser.ParsedDocument{
		FilePath: "notes/meeting.md",
		Objects: []*model.Object{
			{
				ID:        "notes/meeting",
				Type:      "page",
				Fields:    map[string]schema.FieldValue{},
				LineStart: 1,
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
