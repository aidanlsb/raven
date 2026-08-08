package index

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// traitsByType reads all indexed traits of a given trait type straight from the
// traits table. It supports white-box indexing assertions in this package.
// The production list-by-type path is the RQL executor (internal/query), which
// imports internal/index; reading the table directly here keeps the index tests
// free of that import cycle while asserting the same indexed rows.
func traitsByType(t *testing.T, db *Database, traitType string) []model.Trait {
	t.Helper()
	rows, err := db.db.Query(
		"SELECT id, trait_type, value, content, file_path, line_number, parent_object_id FROM traits WHERE trait_type = ? ORDER BY line_number",
		traitType,
	)
	if err != nil {
		t.Fatalf("failed to query traits: %v", err)
	}
	defer rows.Close()

	var results []model.Trait
	for rows.Next() {
		var trait model.Trait
		var value sql.NullString
		if err := rows.Scan(&trait.ID, &trait.TraitType, &value, &trait.Content, &trait.FilePath, &trait.Line, &trait.ParentScopeID); err != nil {
			t.Fatalf("failed to scan trait: %v", err)
		}
		if value.Valid {
			s := value.String
			trait.SetIndexValueString(&s)
		}
		results = append(results, trait)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("trait rows error: %v", err)
	}
	return results
}

func TestIndexDocumentDistinguishesPostCommitResolutionFailure(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	defer db.Close()

	sourceDoc, err := parser.ParseDocument("[[target]]\n", filepath.Join(vaultPath, "source.md"), vaultPath)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	if err := db.IndexDocument(sourceDoc, schema.New()); err != nil {
		t.Fatalf("index source: %v", err)
	}
	if _, err := db.db.Exec(`
		CREATE TRIGGER fail_reference_resolution
		BEFORE UPDATE OF target_id ON refs
		BEGIN
			SELECT RAISE(ABORT, 'reference resolution failed');
		END;
	`); err != nil {
		t.Fatalf("create resolution trigger: %v", err)
	}

	targetDoc, err := parser.ParseDocument("# Target\n", filepath.Join(vaultPath, "target.md"), vaultPath)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	err = db.IndexDocument(targetDoc, schema.New())
	var postCommitErr *PostCommitReferenceResolutionError
	if !errors.As(err, &postCommitErr) {
		t.Fatalf("IndexDocument() error = %v, want PostCommitReferenceResolutionError", err)
	}
	if postCommitErr.FilePath != "target.md" || !postCommitErr.VaultWide {
		t.Fatalf("post-commit error = %#v, want target.md with vault-wide scope", postCommitErr)
	}
	if target, getErr := db.GetObject("target"); getErr != nil {
		t.Fatalf("get committed target: %v", getErr)
	} else if target == nil {
		t.Fatal("target is absent after post-commit resolution failure")
	}

	if _, err := db.db.Exec(`
		CREATE TRIGGER fail_object_insert
		BEFORE INSERT ON objects
		BEGIN
			SELECT RAISE(ABORT, 'index write failed');
		END;
	`); err != nil {
		t.Fatalf("create indexing trigger: %v", err)
	}
	preCommitDoc, err := parser.ParseDocument("# Pre-commit\n", filepath.Join(vaultPath, "pre-commit.md"), vaultPath)
	if err != nil {
		t.Fatalf("parse pre-commit document: %v", err)
	}
	err = db.IndexDocument(preCommitDoc, schema.New())
	if err == nil {
		t.Fatal("IndexDocument() error = nil, want pre-commit failure")
	}
	if errors.As(err, &postCommitErr) {
		t.Fatalf("pre-commit error was misclassified as post-commit: %v", err)
	}
	if object, getErr := db.GetObject("pre-commit"); getErr != nil {
		t.Fatalf("get rolled-back object: %v", getErr)
	} else if object != nil {
		t.Fatalf("pre-commit object was indexed: %#v", object)
	}
}

func TestStarterSchemaBareTodoIndexesAsOpen(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if _, err := schema.CreateDefault(vaultPath); err != nil {
		t.Fatalf("create starter schema: %v", err)
	}
	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load starter schema: %v", err)
	}
	doc, err := parser.ParseDocument(
		"# Tasks\n\n- @todo Review the PR\n",
		filepath.Join(vaultPath, "tasks.md"),
		vaultPath,
	)
	if err != nil {
		t.Fatalf("parse document: %v", err)
	}

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	defer db.Close()
	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("index document: %v", err)
	}

	results := traitsByType(t, db, "todo")
	if len(results) != 1 {
		t.Fatalf("todo result count = %d, want 1", len(results))
	}
	value := results[0].IndexValueString()
	if value == nil || *value != "todo" {
		t.Fatalf("bare todo value = %v, want %q", value, "todo")
	}
}

func TestIndexDocumentStoresAndReplacesLinkEdges(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	doc, err := parser.ParseDocument(`---
type: page
---

[report](../assets/Report.PDF)

## Media

![diagram](../assets/diagram.png)
[site](https://EXAMPLE.COM:443/Case?Q=Value#Frag)
[note](../other.md#section)
[[other]]
`, filepath.Join(vaultPath, "notes", "source.md"), vaultPath)
	if err != nil {
		t.Fatalf("parse document: %v", err)
	}

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	defer db.Close()
	if err := db.IndexDocument(doc, schema.New()); err != nil {
		t.Fatalf("index document: %v", err)
	}

	rows, err := db.db.Query(`
		SELECT source_id, source_type, file_path, line_number, position_start, position_end,
		       raw_target, display, is_image, scheme, ext, normalized_key
		FROM links
		ORDER BY line_number
	`)
	if err != nil {
		t.Fatalf("query links: %v", err)
	}
	defer rows.Close()

	var got []*model.Link
	for rows.Next() {
		link := &model.Link{}
		if err := rows.Scan(
			&link.SourceID,
			&link.SourceType,
			&link.FilePath,
			&link.Line,
			&link.PositionStart,
			&link.PositionEnd,
			&link.RawTarget,
			&link.Display,
			&link.IsImage,
			&link.Scheme,
			&link.Ext,
			&link.NormalizedKey,
		); err != nil {
			t.Fatalf("scan link: %v", err)
		}
		got = append(got, link)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate links: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("indexed links = %#v, want 3", got)
	}

	if got[0].SourceID != "notes/source" || got[0].SourceType != "page" ||
		got[0].FilePath != "notes/source.md" || got[0].RawTarget != "../assets/Report.PDF" ||
		got[0].Display != "report" || got[0].IsImage || got[0].Scheme != "file" ||
		got[0].Ext != "pdf" || got[0].NormalizedKey != "assets/Report.PDF" {
		t.Errorf("report row = %#v", got[0])
	}
	if got[0].Line != 5 || got[0].PositionStart != 0 || got[0].PositionEnd <= got[0].PositionStart {
		t.Errorf("report location = line %d [%d,%d)", got[0].Line, got[0].PositionStart, got[0].PositionEnd)
	}

	if got[1].SourceID != "notes/source" || got[1].SourceType != "page" ||
		!got[1].IsImage || got[1].Display != "diagram" || got[1].Ext != "png" {
		t.Errorf("image row = %#v", got[1])
	}
	if got[2].Scheme != "url" || got[2].Ext != "" ||
		got[2].NormalizedKey != "https://example.com/Case?Q=Value#Frag" {
		t.Errorf("URL row = %#v", got[2])
	}

	replacement, err := parser.ParseDocument("[only](../assets/only.txt)\n", filepath.Join(vaultPath, "notes", "source.md"), vaultPath)
	if err != nil {
		t.Fatalf("parse replacement: %v", err)
	}
	if err := db.IndexDocument(replacement, schema.New()); err != nil {
		t.Fatalf("index replacement: %v", err)
	}
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM links WHERE file_path = ?`, "notes/source.md").Scan(&count); err != nil {
		t.Fatalf("count replacement links: %v", err)
	}
	if count != 1 {
		t.Fatalf("replacement link count = %d, want 1", count)
	}
}

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
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
			Traits: []*model.Trait{},
			Refs:   []*model.Reference{},
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

	t.Run("index document drops unknown frontmatter keys", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		typedSchema := schema.New()
		typedSchema.Types["person"] = &schema.TypeDefinition{
			Fields: map[string]*schema.FieldDefinition{
				"name": {Type: schema.FieldTypeString},
			},
		}

		doc := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*model.Object{
				{
					ID:   "people/freya",
					Type: "person",
					Fields: map[string]fieldvalue.FieldValue{
						"name":    fieldvalue.String("Freya"),
						"alias":   fieldvalue.String("queen"),
						"unknown": fieldvalue.String("should-not-index"),
					},
					LineStart: 1,
				},
			},
		}

		if err := db.IndexDocument(doc, typedSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		warnings := UnknownFrontmatterWarnings(doc, typedSchema)
		if len(warnings) != 1 {
			t.Fatalf("expected 1 warning, got %v", warnings)
		}

		obj, err := db.GetObject("people/freya")
		if err != nil {
			t.Fatalf("GetObject: %v", err)
		}
		if _, ok := obj.Fields["unknown"]; ok {
			t.Fatalf("expected unknown field omitted from index, got %#v", obj.Fields)
		}
		if got, ok := obj.Fields["name"]; !ok {
			t.Fatalf("expected name Freya, got %#v", obj.Fields)
		} else if s, ok := got.AsString(); !ok || s != "Freya" {
			t.Fatalf("expected name Freya, got %#v", got)
		}
		if _, ok := obj.Fields["alias"]; !ok {
			t.Fatalf("expected reserved alias retained, got %#v", obj.Fields)
		}
	})

	t.Run("index array trait value", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		value := fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("raven"), fieldvalue.String("skills")})
		doc := &parser.ParsedDocument{
			FilePath: "test.md",
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "tags",
					Value:         &value,
					Content:       "Review built-in skills",
					ParentScopeID: "test",
					Line:          1,
				},
			},
			Refs: []*model.Reference{},
		}
		sch := schema.New()
		sch.Traits["tags"] = &schema.TraitDefinition{Type: schema.FieldTypeStringArray}

		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		var got string
		if err := db.db.QueryRow(`SELECT value FROM traits WHERE trait_type = 'tags'`).Scan(&got); err != nil {
			t.Fatalf("query trait value: %v", err)
		}
		if got != `["raven","skills"]` {
			t.Fatalf("indexed trait value = %q, want %q", got, `["raven","skills"]`)
		}
	})

	t.Run("indexes generated date field for date objects", func(t *testing.T) {
		db, err := OpenInMemory()
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		doc := &parser.ParsedDocument{
			FilePath: "daily/2026-05-23.md",
			Objects: []*model.Object{
				{
					ID:        "daily/2026-05-23",
					Type:      "date",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
		}

		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		var gotDate, gotSourceID, gotField string
		err = db.db.QueryRow(`
			SELECT date, source_id, field_name
			FROM date_index
			WHERE source_type = 'object'
		`).Scan(&gotDate, &gotSourceID, &gotField)
		if err != nil {
			t.Fatalf("failed to query generated date index row: %v", err)
		}
		if gotDate != "2026-05-23" || gotSourceID != "daily/2026-05-23" || gotField != "date" {
			t.Fatalf("generated date index row = date %q source %q field %q", gotDate, gotSourceID, gotField)
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
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
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
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "highlight",
					Value:         nil, // Bare trait, no value
					Content:       "This is important",
					Line:          5,
					ParentScopeID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// The bare highlight trait should be indexed with its default value "true".
		results := traitsByType(t, db, "highlight")

		if len(results) != 1 {
			t.Errorf("expected 1 result for highlight, got %d", len(results))
		}

		if len(results) > 0 && (results[0].Value == nil || results[0].IndexValueString() == nil || *results[0].IndexValueString() != "true") {
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
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "pinned",
					Value:         nil, // Bare trait
					Content:       "Pinned item",
					Line:          3,
					ParentScopeID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// A bare boolean trait without an explicit default should index as "true".
		results := traitsByType(t, db, "pinned")

		if len(results) != 1 {
			t.Errorf("expected 1 result for pinned, got %d", len(results))
		}
		if len(results) > 0 && (results[0].IndexValueString() == nil || *results[0].IndexValueString() != "true") {
			t.Errorf("expected value 'true', got %v", results[0].Value)
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
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "priority",
					Value:         nil, // Bare trait, should get default "medium"
					Content:       "A task",
					Line:          5,
					ParentScopeID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// The bare enum priority trait should index with its default "medium".
		results := traitsByType(t, db, "priority")

		if len(results) != 1 {
			t.Errorf("expected 1 result for priority, got %d", len(results))
		}
		if len(results) > 0 && (results[0].IndexValueString() == nil || *results[0].IndexValueString() != "medium") {
			t.Errorf("expected value 'medium', got %v", results[0].Value)
		}
	})

	t.Run("remove document resolves file_path from DB (including section IDs)", func(t *testing.T) {
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
			Objects: []*model.Object{
				{
					ID:        "people/freya",
					Type:      "person",
					Fields:    map[string]fieldvalue.FieldValue{},
					LineStart: 1,
				},
				{
					ID:   "people/freya#notes",
					Type: "section",
					Fields: map[string]fieldvalue.FieldValue{
						"title": fieldvalue.String("Notes"),
						"level": fieldvalue.Number(2),
					},
					LineStart: 5,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "highlight",
					Value:         nil,
					Content:       "Hello",
					ParentScopeID: "people/freya",
					Line:          7,
				},
			},
			Refs: []*model.Reference{
				{
					SourceID:      "people/freya",
					TargetRaw:     "projects/website",
					Line:          model.IntPtr(8),
					PositionStart: model.IntPtr(0),
					PositionEnd:   model.IntPtr(0),
				},
			},
		}

		// Define trait so it gets indexed.
		testSchema := schema.New()
		testSchema.Traits["highlight"] = &schema.TraitDefinition{Type: schema.FieldTypeBool}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Remove by section ID (callers may pass file#section).
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
			Objects: []*model.Object{
				{
					ID:        "people/freya",
					Type:      "person",
					Fields:    map[string]fieldvalue.FieldValue{},
					LineStart: 1,
				},
				{
					ID:   "people/freya#notes",
					Type: "section",
					Fields: map[string]fieldvalue.FieldValue{
						"title": fieldvalue.String("Notes"),
						"level": fieldvalue.Number(2),
					},
					LineStart: 5,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "highlight",
					Value:         nil,
					Content:       "Hello",
					ParentScopeID: "people/freya",
					Line:          7,
				},
			},
			Refs: []*model.Reference{
				{
					SourceID:      "people/freya",
					TargetRaw:     "projects/website",
					Line:          model.IntPtr(8),
					PositionStart: model.IntPtr(0),
					PositionEnd:   model.IntPtr(0),
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
			Objects: []*model.Object{
				{
					ID:   "projects/website",
					Type: "project",
					Fields: map[string]fieldvalue.FieldValue{
						"alias": fieldvalue.String("WebSiteAlias"),
					},
					LineStart: 1,
				},
			},
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		res, err := db.Resolver(indexschema.ResolverOptions{DailyDirectory: "journal"})
		if err != nil {
			t.Fatalf("failed to build resolver: %v", err)
		}

		aliasResolved := res.Resolve("websitealias") // different casing
		if aliasResolved.Ambiguous || aliasResolved.TargetID != "projects/website" {
			t.Fatalf("expected alias to resolve to projects/website, got %+v", aliasResolved)
		}

		dateResolved := res.Resolve("2025-02-01")
		if dateResolved.Ambiguous || dateResolved.TargetID != "2025-02-01" {
			t.Fatalf("expected date shorthand to resolve to bare date 2025-02-01, got %+v", dateResolved)
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
			Objects: []*model.Object{
				{
					ID:   "books/first",
					Type: "book",
					Fields: map[string]fieldvalue.FieldValue{
						"title": fieldvalue.String("Shared Display Name"),
					},
					LineStart: 1,
				},
			},
		}
		doc2 := &parser.ParsedDocument{
			FilePath: "books/second.md",
			Objects: []*model.Object{
				{
					ID:   "books/second",
					Type: "book",
					Fields: map[string]fieldvalue.FieldValue{
						"title": fieldvalue.String("Shared Display Name"),
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

		res, err := db.Resolver(indexschema.ResolverOptions{Schema: testSchema})
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
			Objects: []*model.Object{
				{
					ID:        "test",
					Type:      "page",
					Fields:    make(map[string]fieldvalue.FieldValue),
					LineStart: 1,
				},
			},
			Traits: []*model.Trait{
				{
					TraitType:     "defined", // This one IS in schema
					Value:         nil,
					Content:       "Defined trait",
					Line:          3,
					ParentScopeID: "test",
				},
				{
					TraitType:     "undefined", // This one is NOT in schema
					Value:         nil,
					Content:       "Undefined trait",
					Line:          5,
					ParentScopeID: "test",
				},
			},
		}

		if err := db.IndexDocument(doc, testSchema); err != nil {
			t.Fatalf("failed to index document: %v", err)
		}

		// Defined trait is indexed; undefined trait is not (schema is source of truth).
		definedResults := traitsByType(t, db, "defined")
		if len(definedResults) != 1 {
			t.Errorf("expected 1 result for 'defined' trait, got %d", len(definedResults))
		}

		undefinedResults := traitsByType(t, db, "undefined")
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
