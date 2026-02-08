package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestCheckStaleness(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.NewSchema()

	vaultDir := t.TempDir()
	now := time.Now().Unix()

	stalePath := filepath.Join(vaultDir, "notes/stale.md")
	freshPath := filepath.Join(vaultDir, "notes/fresh.md")

	if err := os.MkdirAll(filepath.Dir(stalePath), 0755); err != nil {
		t.Fatalf("failed to create notes dir: %v", err)
	}

	// Create files on disk.
	if err := os.WriteFile(stalePath, []byte("stale"), 0644); err != nil {
		t.Fatalf("failed to write stale file: %v", err)
	}
	if err := os.WriteFile(freshPath, []byte("fresh"), 0644); err != nil {
		t.Fatalf("failed to write fresh file: %v", err)
	}

	// Index three docs: stale, fresh, and missing (not on disk).
	staleIndexedMtime := now - 100
	freshIndexedMtime := now + 100
	missingIndexedMtime := now

	staleDoc := &parser.ParsedDocument{
		FilePath: "notes/stale.md",
		Objects: []*parser.ParsedObject{
			{ID: "notes/stale", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
		},
	}
	freshDoc := &parser.ParsedDocument{
		FilePath: "notes/fresh.md",
		Objects: []*parser.ParsedObject{
			{ID: "notes/fresh", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
		},
	}
	missingDoc := &parser.ParsedDocument{
		FilePath: "notes/missing.md",
		Objects: []*parser.ParsedObject{
			{ID: "notes/missing", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
		},
	}

	if err := db.IndexDocumentWithMtime(staleDoc, sch, staleIndexedMtime); err != nil {
		t.Fatalf("failed to index stale doc: %v", err)
	}
	if err := db.IndexDocumentWithMtime(freshDoc, sch, freshIndexedMtime); err != nil {
		t.Fatalf("failed to index fresh doc: %v", err)
	}
	if err := db.IndexDocumentWithMtime(missingDoc, sch, missingIndexedMtime); err != nil {
		t.Fatalf("failed to index missing doc: %v", err)
	}

	// Make stale file newer than its indexed mtime.
	if err := os.Chtimes(stalePath, time.Unix(staleIndexedMtime+10, 0), time.Unix(staleIndexedMtime+10, 0)); err != nil {
		t.Fatalf("failed to chtimes stale file: %v", err)
	}
	// Make fresh file older than (or equal to) its indexed mtime.
	if err := os.Chtimes(freshPath, time.Unix(freshIndexedMtime-10, 0), time.Unix(freshIndexedMtime-10, 0)); err != nil {
		t.Fatalf("failed to chtimes fresh file: %v", err)
	}

	info, err := db.CheckStaleness(vaultDir)
	if err != nil {
		t.Fatalf("CheckStaleness error: %v", err)
	}

	if info.TotalFiles != 3 {
		t.Fatalf("TotalFiles = %d, want 3", info.TotalFiles)
	}
	if info.CheckedFiles != 2 {
		t.Fatalf("CheckedFiles = %d, want 2", info.CheckedFiles)
	}
	if !info.IsStale {
		t.Fatalf("IsStale = false, want true")
	}

	stales := make(map[string]bool, len(info.StaleFiles))
	for _, p := range info.StaleFiles {
		stales[p] = true
	}
	if !stales["notes/stale.md"] {
		t.Fatalf("expected notes/stale.md to be stale; got %v", info.StaleFiles)
	}
	if !stales["notes/missing.md"] {
		t.Fatalf("expected notes/missing.md to be stale (missing on disk); got %v", info.StaleFiles)
	}
	if stales["notes/fresh.md"] {
		t.Fatalf("did not expect notes/fresh.md to be stale; got %v", info.StaleFiles)
	}
}

func TestRemoveDeletedFiles(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.NewSchema()
	sch.Traits["flag"] = &schema.TraitDefinition{Type: schema.FieldTypeBool}

	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "notes"), 0755); err != nil {
		t.Fatalf("failed to create notes dir: %v", err)
	}

	// Create only the "exists" file on disk.
	if err := os.WriteFile(filepath.Join(vaultDir, "notes/exists.md"), []byte("ok"), 0644); err != nil {
		t.Fatalf("failed to write exists file: %v", err)
	}

	existsDoc := &parser.ParsedDocument{
		FilePath:   "notes/exists.md",
		RawContent: "ok",
		Objects: []*parser.ParsedObject{
			{ID: "notes/exists", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
		},
	}
	missingDoc := &parser.ParsedDocument{
		FilePath:   "notes/missing.md",
		RawContent: "missing",
		Objects: []*parser.ParsedObject{
			{ID: "notes/missing", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
		},
		Traits: []*parser.ParsedTrait{
			{TraitType: "flag", Value: nil, Content: "x", ParentObjectID: "notes/missing", Line: 2},
		},
		Refs: []*parser.ParsedRef{
			{SourceID: "notes/missing", TargetRaw: "people/freya", Line: 3},
		},
	}

	if err := db.IndexDocument(existsDoc, sch); err != nil {
		t.Fatalf("failed to index exists doc: %v", err)
	}
	if err := db.IndexDocument(missingDoc, sch); err != nil {
		t.Fatalf("failed to index missing doc: %v", err)
	}

	removed, err := db.RemoveDeletedFiles(vaultDir)
	if err != nil {
		t.Fatalf("RemoveDeletedFiles error: %v", err)
	}

	if len(removed) != 1 || removed[0] != "notes/missing.md" {
		t.Fatalf("removed = %v, want [notes/missing.md]", removed)
	}

	// Ensure the missing file is removed from the index.
	for _, table := range []string{"objects", "traits", "refs", "date_index", "fts_content"} {
		var n int
		if err := db.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE file_path = ?", "notes/missing.md").Scan(&n); err != nil {
			t.Fatalf("failed to query %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("expected %s rows for notes/missing.md to be 0, got %d", table, n)
		}
	}
	// And the existing file remains.
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM objects WHERE file_path = ?", "notes/exists.md").Scan(&n); err != nil {
		t.Fatalf("failed to query objects: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected notes/exists.md to remain indexed")
	}
}

func TestRemoveFilesWithPrefix(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.NewSchema()

	docs := []*parser.ParsedDocument{
		{
			FilePath: ".trash/a.md",
			Objects: []*parser.ParsedObject{
				{ID: ".trash/a", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
			},
		},
		{
			FilePath: ".trash/b.md",
			Objects: []*parser.ParsedObject{
				{ID: ".trash/b", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
			},
		},
		{
			FilePath: "keep/c.md",
			Objects: []*parser.ParsedObject{
				{ID: "keep/c", ObjectType: "page", Fields: map[string]schema.FieldValue{}, LineStart: 1},
			},
		},
	}

	for _, doc := range docs {
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("failed to index %s: %v", doc.FilePath, err)
		}
	}

	removedCount, err := db.RemoveFilesWithPrefix(".trash/")
	if err != nil {
		t.Fatalf("RemoveFilesWithPrefix error: %v", err)
	}
	if removedCount != 2 {
		t.Fatalf("removedCount = %d, want 2", removedCount)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM objects WHERE file_path LIKE '.trash/%'").Scan(&n); err != nil {
		t.Fatalf("failed to query objects: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 objects under .trash/, got %d", n)
	}
	if err := db.db.QueryRow("SELECT COUNT(*) FROM objects WHERE file_path = ?", "keep/c.md").Scan(&n); err != nil {
		t.Fatalf("failed to query keep/c.md objects: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected keep/c.md to remain indexed")
	}
}

func TestAllNameFieldValues(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.NewSchema()
	sch.Types["book"] = &schema.TypeDefinition{
		NameField: "title",
		Fields: map[string]*schema.FieldDefinition{
			"title": {Type: schema.FieldTypeString},
		},
	}

	doc := &parser.ParsedDocument{
		FilePath: "books/the-prose-edda.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "books/the-prose-edda",
				ObjectType: "book",
				Fields: map[string]schema.FieldValue{
					"title": schema.String("The Prose Edda"),
				},
				LineStart: 1,
			},
		},
	}

	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("failed to index doc: %v", err)
	}

	nameFieldMap, err := db.AllNameFieldValues(sch)
	if err != nil {
		t.Fatalf("AllNameFieldValues error: %v", err)
	}

	if got := nameFieldMap["The Prose Edda"]; got != "books/the-prose-edda" {
		t.Fatalf("nameFieldMap[%q] = %q, want %q", "The Prose Edda", got, "books/the-prose-edda")
	}
}
