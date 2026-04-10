package index

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aidanlsb/raven/internal/filelock"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestDatabase(t *testing.T) {
	t.Parallel()
	// Create a minimal schema for testing
	sch := schema.New()

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

	t.Run("clear all data rolls back on failure", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		if _, err := db.db.Exec(`
			INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
				('test', 'test.md', 'page', '{}', 1);
			INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number) VALUES
				('trait1', 'test.md', 'test', 'todo', 'todo', 'Line', 1);
			CREATE TRIGGER fail_trait_delete BEFORE DELETE ON traits
			BEGIN
				SELECT RAISE(FAIL, 'boom');
			END;
		`); err != nil {
			t.Fatalf("seed database: %v", err)
		}

		if err := db.ClearAllData(); err == nil {
			t.Fatal("expected ClearAllData to fail")
		}

		var objectCount, traitCount int
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM objects`).Scan(&objectCount); err != nil {
			t.Fatalf("count objects: %v", err)
		}
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM traits`).Scan(&traitCount); err != nil {
			t.Fatalf("count traits: %v", err)
		}

		if objectCount != 1 || traitCount != 1 {
			t.Fatalf("expected rollback to preserve rows, got objects=%d traits=%d", objectCount, traitCount)
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
		testSchema := schema.New()
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
		testSchema := schema.New()
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
		testSchema := schema.New()
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
		testSchema := schema.New()
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

	t.Run("remove document rolls back when cleanup fails", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		doc := &parser.ParsedDocument{
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

		testSchema := schema.New()
		testSchema.Traits["highlight"] = &schema.TraitDefinition{Type: schema.FieldTypeBool}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		if _, err := db.db.Exec(`
			CREATE TRIGGER fail_traits_delete
			BEFORE DELETE ON traits
			BEGIN
				SELECT RAISE(ABORT, 'trait delete failed');
			END;
		`); err != nil {
			t.Fatalf("create trigger: %v", err)
		}

		if err := db.RemoveDocument("people/freya"); err == nil {
			t.Fatal("expected remove document to fail")
		}

		for _, tc := range []struct {
			table string
			want  int
		}{
			{table: "objects", want: 2},
			{table: "traits", want: 1},
			{table: "refs", want: 1},
			{table: "fts_content", want: 2},
		} {
			var got int
			if err := db.db.QueryRow("SELECT COUNT(*) FROM "+tc.table+" WHERE file_path = ?", "objects/people/freya.md").Scan(&got); err != nil {
				t.Fatalf("count %s: %v", tc.table, err)
			}
			if got != tc.want {
				t.Fatalf("%s rows = %d, want %d", tc.table, got, tc.want)
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

	t.Run("resolver keeps duplicate name_field values ambiguous", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		testSchema := schema.New()
		testSchema.Types["book"] = &schema.TypeDefinition{
			NameField: "title",
			Fields: map[string]*schema.FieldDefinition{
				"title": {Type: schema.FieldTypeString},
			},
		}

		doc1 := &parser.ParsedDocument{
			FilePath: "books/first.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "books/first",
					ObjectType: "book",
					Fields: map[string]schema.FieldValue{
						"title": schema.String("Shared Display Name"),
					},
					LineStart: 1,
				},
			},
		}
		doc2 := &parser.ParsedDocument{
			FilePath: "books/second.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "books/second",
					ObjectType: "book",
					Fields: map[string]schema.FieldValue{
						"title": schema.String("Shared Display Name"),
					},
					LineStart: 1,
				},
			},
		}

		if err := db.IndexDocument(doc1, testSchema); err != nil {
			t.Fatalf("failed to index first doc: %v", err)
		}
		if err := db.IndexDocument(doc2, testSchema); err != nil {
			t.Fatalf("failed to index second doc: %v", err)
		}

		res, err := db.Resolver(ResolverOptions{Schema: testSchema})
		if err != nil {
			t.Fatalf("failed to build resolver: %v", err)
		}

		result := res.Resolve("Shared Display Name")
		if !result.Ambiguous {
			t.Fatalf("expected duplicate name_field resolution to be ambiguous, got %+v", result)
		}

		found := map[string]bool{}
		for _, match := range result.Matches {
			found[match] = true
		}
		if !found["books/first"] || !found["books/second"] {
			t.Fatalf("expected matches to include both books, got %v", result.Matches)
		}
	})

	t.Run("undefined traits are not indexed", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		// Create schema WITHOUT the "undefined" trait
		testSchema := schema.New()
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

func TestExtractDateString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value schema.FieldValue
		want  string
	}{
		{
			name:  "date value",
			value: schema.Date("2026-04-09"),
			want:  "2026-04-09",
		},
		{
			name:  "datetime value returns date prefix",
			value: schema.Datetime("2026-04-09T12:34:56Z"),
			want:  "2026-04-09",
		},
		{
			name:  "date-shaped junk string is rejected",
			value: schema.String("ABCD-EF-GH trailing text"),
			want:  "",
		},
		{
			name:  "short string is rejected",
			value: schema.String("2026-04"),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDateString(tt.value); got != tt.want {
				t.Fatalf("extractDateString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTraitIDConsistency is a regression test for the bug where indexDates used
// the raw loop index (idx) while indexInlineTraits used a counter that only
// incremented for defined traits. When undefined traits preceded defined ones,
// the two functions produced different IDs for the same physical trait, causing
// date queries to reference non-existent trait IDs.
func TestTraitIDConsistency(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Schema defines "due" but NOT "undefined" — so "undefined" must be skipped.
	testSchema := schema.New()
	testSchema.Traits["due"] = &schema.TraitDefinition{
		Type: schema.FieldTypeDate,
	}

	dueValue := schema.Date("2025-03-15")
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
				// raw index 0 — undefined, must be skipped
				TraitType:      "undefined",
				Value:          nil,
				Content:        "some note",
				Line:           3,
				ParentObjectID: "test",
			},
			{
				// raw index 1, but first defined trait → traitIdx=0 in both functions
				TraitType:      "due",
				Value:          &dueValue,
				Content:        "finish by",
				Line:           4,
				ParentObjectID: "test",
			},
		},
	}

	if err := db.IndexDocument(doc, testSchema); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	// The "due" trait should be stored with ID "test.md:trait:0" in the traits table.
	var traitID string
	if err := db.db.QueryRow(`SELECT id FROM traits WHERE trait_type = 'due'`).Scan(&traitID); err != nil {
		t.Fatalf("failed to query traits table: %v", err)
	}
	if traitID != "test.md:trait:0" {
		t.Errorf("traits table: got id %q, want %q", traitID, "test.md:trait:0")
	}

	// The date_index entry for the same trait must reference the same ID.
	var dateSourceID string
	if err := db.db.QueryRow(`SELECT source_id FROM date_index WHERE source_type = 'trait'`).Scan(&dateSourceID); err != nil {
		t.Fatalf("failed to query date_index table: %v", err)
	}
	if dateSourceID != traitID {
		t.Errorf("date_index source_id %q does not match traits.id %q — trait ID mismatch bug", dateSourceID, traitID)
	}
}

func TestDateIndexTraitIDsTrackIndexedTraitOrder(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Traits["due"] = &schema.TraitDefinition{Type: schema.FieldTypeDate}
	sch.Traits["review"] = &schema.TraitDefinition{Type: schema.FieldTypeDate}
	sch.Traits["status"] = &schema.TraitDefinition{Type: schema.FieldTypeString}

	dueValue := schema.Date("2025-03-15")
	reviewValue := schema.Date("2025-03-20")
	statusValue := schema.String("active")
	doc := &parser.ParsedDocument{
		FilePath: "notes/plan.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "notes/plan",
				ObjectType: "note",
				Fields:     map[string]schema.FieldValue{},
				LineStart:  1,
			},
		},
		Traits: []*parser.ParsedTrait{
			{
				TraitType:      "undefined",
				Content:        "skip me",
				Line:           2,
				ParentObjectID: "notes/plan",
			},
			{
				TraitType:      "due",
				Value:          &dueValue,
				Content:        "ship it",
				Line:           3,
				ParentObjectID: "notes/plan",
			},
			{
				TraitType:      "status",
				Value:          &statusValue,
				Content:        "state",
				Line:           4,
				ParentObjectID: "notes/plan",
			},
			{
				TraitType:      "review",
				Value:          &reviewValue,
				Content:        "check it",
				Line:           5,
				ParentObjectID: "notes/plan",
			},
		},
	}

	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	traitIDsByType := map[string]string{}
	rows, err := db.db.Query(`SELECT trait_type, id FROM traits ORDER BY line_number`)
	if err != nil {
		t.Fatalf("query traits: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var traitType, traitID string
		if err := rows.Scan(&traitType, &traitID); err != nil {
			t.Fatalf("scan trait row: %v", err)
		}
		traitIDsByType[traitType] = traitID
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate trait rows: %v", err)
	}

	wantTraitIDs := map[string]string{
		"due":    "notes/plan.md:trait:0",
		"status": "notes/plan.md:trait:1",
		"review": "notes/plan.md:trait:2",
	}
	for traitType, wantID := range wantTraitIDs {
		if got := traitIDsByType[traitType]; got != wantID {
			t.Fatalf("%s trait id = %q, want %q", traitType, got, wantID)
		}
	}

	dateIDsByType := map[string]string{}
	dateRows, err := db.db.Query(`
		SELECT field_name, source_id
		FROM date_index
		WHERE source_type = 'trait'
		ORDER BY date
	`)
	if err != nil {
		t.Fatalf("query date_index: %v", err)
	}
	defer dateRows.Close()
	for dateRows.Next() {
		var fieldName, sourceID string
		if err := dateRows.Scan(&fieldName, &sourceID); err != nil {
			t.Fatalf("scan date_index row: %v", err)
		}
		dateIDsByType[fieldName] = sourceID
	}
	if err := dateRows.Err(); err != nil {
		t.Fatalf("iterate date_index rows: %v", err)
	}

	if len(dateIDsByType) != 2 {
		t.Fatalf("got %d trait-backed date rows, want 2", len(dateIDsByType))
	}
	if got := dateIDsByType["due"]; got != traitIDsByType["due"] {
		t.Fatalf("due date_index source_id = %q, want %q", got, traitIDsByType["due"])
	}
	if got := dateIDsByType["review"]; got != traitIDsByType["review"] {
		t.Fatalf("review date_index source_id = %q, want %q", got, traitIDsByType["review"])
	}
}

func TestTraitIDsStableAcrossReindexForMultilineParagraph(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Traits["todo"] = &schema.TraitDefinition{Type: schema.FieldTypeString}

	content := "First task @todo one\nSecond task @todo two\nThird task @todo three\n"
	expectedIDs := []string{
		"notes/unstable.md:trait:0",
		"notes/unstable.md:trait:1",
		"notes/unstable.md:trait:2",
	}

	for i := 0; i < 20; i++ {
		if err := db.ClearAllData(); err != nil {
			t.Fatalf("iteration %d: failed to clear database: %v", i, err)
		}

		doc, err := parser.ParseDocument(content, "/vault/notes/unstable.md", "/vault")
		if err != nil {
			t.Fatalf("iteration %d: failed to parse document: %v", i, err)
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("iteration %d: failed to index document: %v", i, err)
		}

		rows, err := db.db.Query(`SELECT id FROM traits ORDER BY line_number`)
		if err != nil {
			t.Fatalf("iteration %d: failed to query traits: %v", i, err)
		}

		var gotIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatalf("iteration %d: failed to scan trait id: %v", i, err)
			}
			gotIDs = append(gotIDs, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iteration %d: row iteration error: %v", i, err)
		}
		rows.Close()

		if len(gotIDs) != len(expectedIDs) {
			t.Fatalf("iteration %d: got %d traits, want %d", i, len(gotIDs), len(expectedIDs))
		}
		for j, gotID := range gotIDs {
			if gotID != expectedIDs[j] {
				t.Fatalf(
					"iteration %d: trait id at line-order index %d = %q, want %q (unstable trait ordering)",
					i, j, gotID, expectedIDs[j],
				)
			}
		}

		var dateSourceID string
		if err := db.db.QueryRow(`
			SELECT source_id FROM date_index
			WHERE source_type='trait'
			ORDER BY date
			LIMIT 1
		`).Scan(&dateSourceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("iteration %d: failed to query date index source id: %v", i, err)
		}
		if dateSourceID != "" {
			wantPrefix := "notes/unstable.md:trait:"
			if len(dateSourceID) < len(wantPrefix) || dateSourceID[:len(wantPrefix)] != wantPrefix {
				t.Fatalf("iteration %d: unexpected date_index trait source id %q", i, dateSourceID)
			}
		}
	}
}

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

	t.Run("resolver treats duplicate aliases as ambiguous", func(t *testing.T) {
		db2, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db2.Close()

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

func TestAllIndexedFilePaths(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()

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
	t.Parallel()
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

	if err := filelock.TryLockExclusive(lockFile); err != nil {
		t.Fatalf("failed to acquire test lock: %v", err)
	}
	defer filelock.Unlock(lockFile)

	if _, _, err := OpenWithRebuild(vaultDir); !errors.Is(err, ErrIndexLocked) {
		t.Fatalf("expected ErrIndexLocked, got %v", err)
	}
}

func TestIsSchemaCompatibleUsesMetaVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version *string
		want    bool
	}{
		{
			name: "current version is compatible",
			version: func() *string {
				v := strconv.Itoa(CurrentDBVersion)
				return &v
			}(),
			want: true,
		},
		{
			name: "stale version is incompatible",
			version: func() *string {
				v := strconv.Itoa(CurrentDBVersion - 1)
				return &v
			}(),
			want: false,
		},
		{
			name: "missing version is incompatible",
			want: false,
		},
		{
			name: "invalid version is incompatible",
			version: func() *string {
				v := "banana"
				return &v
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rawDB, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			defer rawDB.Close()

			if _, err := rawDB.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
				t.Fatalf("create meta table: %v", err)
			}
			if tt.version != nil {
				if _, err := rawDB.Exec(`INSERT INTO meta (key, value) VALUES ('version', ?)`, *tt.version); err != nil {
					t.Fatalf("insert version: %v", err)
				}
			}

			if got := isSchemaCompatible(rawDB); got != tt.want {
				t.Fatalf("isSchemaCompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenWithRebuildRebuildsStaleVersionAfterPlainOpen(t *testing.T) {
	t.Parallel()

	vaultDir := t.TempDir()
	legacyVersion := strconv.Itoa(CurrentDBVersion - 1)
	seedLegacyIndexVersion(t, vaultDir, legacyVersion)

	db, err := Open(vaultDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	version, ok, err := storedDatabaseVersion(db.db)
	if err != nil {
		t.Fatalf("storedDatabaseVersion after Open: %v", err)
	}
	if !ok || version != CurrentDBVersion-1 {
		t.Fatalf("expected Open to preserve legacy version %d, got ok=%v version=%d", CurrentDBVersion-1, ok, version)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	rebuiltDB, rebuilt, err := OpenWithRebuild(vaultDir)
	if err != nil {
		t.Fatalf("OpenWithRebuild: %v", err)
	}
	defer rebuiltDB.Close()
	if !rebuilt {
		t.Fatal("expected stale version to trigger rebuild")
	}

	currentVersion, ok, err := storedDatabaseVersion(rebuiltDB.db)
	if err != nil {
		t.Fatalf("storedDatabaseVersion after rebuild: %v", err)
	}
	if !ok || currentVersion != CurrentDBVersion {
		t.Fatalf("expected rebuilt DB version %d, got ok=%v version=%d", CurrentDBVersion, ok, currentVersion)
	}
}

func TestOpenWithRebuildRebuildsWhenVersionMissing(t *testing.T) {
	t.Parallel()

	vaultDir := t.TempDir()
	seedLegacyIndexVersion(t, vaultDir, "")

	db, rebuilt, err := OpenWithRebuild(vaultDir)
	if err != nil {
		t.Fatalf("OpenWithRebuild: %v", err)
	}
	defer db.Close()
	if !rebuilt {
		t.Fatal("expected missing version to trigger rebuild")
	}

	currentVersion, ok, err := storedDatabaseVersion(db.db)
	if err != nil {
		t.Fatalf("storedDatabaseVersion after rebuild: %v", err)
	}
	if !ok || currentVersion != CurrentDBVersion {
		t.Fatalf("expected rebuilt DB version %d, got ok=%v version=%d", CurrentDBVersion, ok, currentVersion)
	}
}

func seedLegacyIndexVersion(t *testing.T, vaultDir string, version string) {
	t.Helper()

	dbDir := filepath.Join(vaultDir, ".raven")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}

	rawDB, err := sql.Open("sqlite", filepath.Join(dbDir, "index.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer rawDB.Close()

	if _, err := rawDB.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create meta table: %v", err)
	}
	if version != "" {
		if _, err := rawDB.Exec(`INSERT INTO meta (key, value) VALUES ('version', ?)`, version); err != nil {
			t.Fatalf("insert version: %v", err)
		}
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
