package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestRevisionInitializedOnNewDatabase(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	rev, err := db.Revision()
	if err != nil {
		t.Fatalf("Revision: %v", err)
	}
	if rev.Generation == "" {
		t.Fatal("expected a generation on a fresh index")
	}
	if rev.Counter != 0 {
		t.Fatalf("counter = %d, want 0", rev.Counter)
	}
}

func TestRevisionBumpsOnIndexAndRemove(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	before, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	doc := parseTestDoc(t, "note.md", "---\ntype: note\n---\nHello.\n")
	if err := db.IndexDocumentWithMtime(doc, schema.New(), 100); err != nil {
		t.Fatalf("IndexDocumentWithMtime: %v", err)
	}
	afterIndex, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if afterIndex.Generation != before.Generation {
		t.Fatalf("generation changed on index: %q -> %q", before.Generation, afterIndex.Generation)
	}
	if afterIndex.Counter != before.Counter+1 {
		t.Fatalf("counter after index = %d, want %d", afterIndex.Counter, before.Counter+1)
	}

	if err := db.RemoveFile("note.md"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	afterRemove, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if afterRemove.Generation != before.Generation {
		t.Fatalf("generation changed on remove: %q -> %q", before.Generation, afterRemove.Generation)
	}
	if afterRemove.Counter != afterIndex.Counter+1 {
		t.Fatalf("counter after remove = %d, want %d", afterRemove.Counter, afterIndex.Counter+1)
	}
}

func TestClearAllDataMintsNewGeneration(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	if err := db.IndexDocumentWithMtime(parseTestDoc(t, "note.md", "body\n"), schema.New(), 1); err != nil {
		t.Fatal(err)
	}
	before, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if before.Counter == 0 {
		t.Fatal("expected a non-zero counter after indexing")
	}
	if err := db.ClearAllData(); err != nil {
		t.Fatalf("ClearAllData: %v", err)
	}
	after, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation == "" || after.Generation == before.Generation {
		t.Fatalf("generation after rebuild = %q, want a new value (was %q)", after.Generation, before.Generation)
	}
	if after.Counter != 0 {
		t.Fatalf("counter after rebuild = %d, want 0", after.Counter)
	}
}

func TestRevisionDoesNotSurviveIndexDeletion(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	db, err := Open(vaultPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	indexPath := filepath.Join(vaultPath, ".raven", "index.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(indexPath + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", indexPath+suffix, err)
		}
	}

	db, err = Open(vaultPath)
	if err != nil {
		t.Fatalf("reopen after delete: %v", err)
	}
	defer db.Close()
	second, err := db.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation == "" {
		t.Fatal("expected a generation after rebuild from scratch")
	}
	if second.Generation == first.Generation {
		t.Fatalf("generation survived index deletion: %q", second.Generation)
	}
	if second.Counter != 0 {
		t.Fatalf("counter after scratch rebuild = %d, want 0", second.Counter)
	}
}

func TestFileProjectionActionMtimeAndHash(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	db, err := Open(vaultPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	relPath := "note.md"
	fullPath := filepath.Join(vaultPath, relPath)
	content := []byte("---\ntype: note\n---\nHello.\n")
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), fullPath, vaultPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.IndexDocumentWithMtime(doc, schema.New(), stat.ModTime().Unix()); err != nil {
		t.Fatal(err)
	}

	action, err := db.FileProjectionAction(vaultPath, relPath)
	if err != nil {
		t.Fatal(err)
	}
	if action != FileProjectionSkip {
		t.Fatalf("action after index = %d, want skip", action)
	}

	// Touch keeps bytes the same; mtime moves so the hash check should skip.
	later := stat.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(fullPath, later, later); err != nil {
		t.Fatal(err)
	}
	action, err = db.FileProjectionAction(vaultPath, relPath)
	if err != nil {
		t.Fatal(err)
	}
	if action != FileProjectionSkip {
		t.Fatalf("action after touch = %d, want skip via hash", action)
	}

	if err := os.WriteFile(fullPath, []byte("---\ntype: note\n---\nChanged.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	action, err = db.FileProjectionAction(vaultPath, relPath)
	if err != nil {
		t.Fatal(err)
	}
	if action != FileProjectionIndex {
		t.Fatalf("action after edit = %d, want index", action)
	}

	if err := os.Remove(fullPath); err != nil {
		t.Fatal(err)
	}
	action, err = db.FileProjectionAction(vaultPath, relPath)
	if err != nil {
		t.Fatal(err)
	}
	if action != FileProjectionRemove {
		t.Fatalf("action after delete = %d, want remove", action)
	}
}

func TestRevisionMap(t *testing.T) {
	t.Parallel()
	rev := Revision{Generation: "abc", Counter: 7}
	got := rev.Map()
	if got["generation"] != "abc" || got["counter"] != int64(7) {
		t.Fatalf("Map() = %#v", got)
	}
}

func parseTestDoc(t *testing.T, relPath, content string) *parser.ParsedDocument {
	t.Helper()
	doc, err := parser.ParseDocument(content, relPath, ".")
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}
	return doc
}
