package index

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// TestRemoveFilesWithPrefix_ExcludedPathPruning tests that files under excluded
// directories (like .trash/) can be cleanly removed from the index.
func TestRemoveFilesWithPrefix_ExcludedPathPruning(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	sch := schema.New()

	// Index some files under .trash/ and keep/
	trashDoc1 := &parser.ParsedDocument{
		FilePath: ".trash/deleted1.md",
		Objects: []*model.Object{
			{ID: ".trash/deleted1", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}
	trashDoc2 := &parser.ParsedDocument{
		FilePath: ".trash/subfolder/deleted2.md",
		Objects: []*model.Object{
			{ID: ".trash/subfolder/deleted2", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}
	keepDoc := &parser.ParsedDocument{
		FilePath: "keep/important.md",
		Objects: []*model.Object{
			{ID: "keep/important", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}

	if err := db.IndexDocument(trashDoc1, sch); err != nil {
		t.Fatalf("IndexDocument(trashDoc1): %v", err)
	}
	if err := db.IndexDocument(trashDoc2, sch); err != nil {
		t.Fatalf("IndexDocument(trashDoc2): %v", err)
	}
	if err := db.IndexDocument(keepDoc, sch); err != nil {
		t.Fatalf("IndexDocument(keepDoc): %v", err)
	}

	// Verify all three are indexed
	allPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	if len(allPaths) != 3 {
		t.Fatalf("indexed %d files, want 3", len(allPaths))
	}

	// Remove files with .trash/ prefix
	removed, err := db.RemoveFilesWithPrefix(".trash/")
	if err != nil {
		t.Fatalf("RemoveFilesWithPrefix: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d files, want 2", removed)
	}

	// Verify only keep/ remains
	allPaths, err = db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths after removal: %v", err)
	}
	if len(allPaths) != 1 {
		t.Fatalf("indexed %d files after removal, want 1", len(allPaths))
	}
	if allPaths[0] != "keep/important.md" {
		t.Errorf("remaining file = %q, want %q", allPaths[0], "keep/important.md")
	}
}

// TestRemoveFilesWithPrefix_EmptyPrefix tests that an empty prefix is handled.
func TestRemoveFilesWithPrefix_EmptyPrefix(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	sch := schema.New()

	doc := &parser.ParsedDocument{
		FilePath: "test.md",
		Objects: []*model.Object{
			{ID: "test", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}
	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	// Empty prefix matches everything with SQL LIKE, so this actually removes the file
	// This is expected behavior - calling with "" is the same as matching all files
	removed, err := db.RemoveFilesWithPrefix("")
	if err != nil {
		t.Fatalf("RemoveFilesWithPrefix(''): %v", err)
	}
	if removed != 1 {
		t.Errorf("removed %d files, want 1 (empty prefix matches all)", removed)
	}

	// File should be removed
	allPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	if len(allPaths) != 0 {
		t.Errorf("indexed %d files, want 0 (empty prefix removes all)", len(allPaths))
	}
}

// TestRemoveDeletedFiles_RemovedFileCleanup tests that files deleted from disk
// are properly removed from the index.
func TestRemoveDeletedFiles_RemovedFileCleanup(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	vaultDir := t.TempDir()

	// Create two files on disk
	file1Path := filepath.Join(vaultDir, "exists.md")
	file2Path := filepath.Join(vaultDir, "will-delete.md")
	if err := os.WriteFile(file1Path, []byte("exists"), 0644); err != nil {
		t.Fatalf("WriteFile(exists): %v", err)
	}
	if err := os.WriteFile(file2Path, []byte("will delete"), 0644); err != nil {
		t.Fatalf("WriteFile(will-delete): %v", err)
	}

	// Index both files
	doc1 := &parser.ParsedDocument{
		FilePath: "exists.md",
		Objects: []*model.Object{
			{ID: "exists", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}
	doc2 := &parser.ParsedDocument{
		FilePath: "will-delete.md",
		Objects: []*model.Object{
			{ID: "will-delete", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}
	if err := db.IndexDocument(doc1, sch); err != nil {
		t.Fatalf("IndexDocument(doc1): %v", err)
	}
	if err := db.IndexDocument(doc2, sch); err != nil {
		t.Fatalf("IndexDocument(doc2): %v", err)
	}

	// Delete one file from disk
	if err := os.Remove(file2Path); err != nil {
		t.Fatalf("Remove(will-delete): %v", err)
	}

	// RemoveDeletedFiles should detect and remove it
	removed, err := db.RemoveDeletedFiles(vaultDir)
	if err != nil {
		t.Fatalf("RemoveDeletedFiles: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("removed %d files, want 1", len(removed))
	}
	if removed[0] != "will-delete.md" {
		t.Errorf("removed file = %q, want %q", removed[0], "will-delete.md")
	}

	// Verify only exists.md remains
	allPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	if len(allPaths) != 1 {
		t.Fatalf("indexed %d files, want 1", len(allPaths))
	}
	if allPaths[0] != "exists.md" {
		t.Errorf("remaining file = %q, want %q", allPaths[0], "exists.md")
	}
}

// TestRemoveDeletedFiles_AllExist tests that no files are removed when all exist.
func TestRemoveDeletedFiles_AllExist(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	vaultDir := t.TempDir()

	filePath := filepath.Join(vaultDir, "exists.md")
	if err := os.WriteFile(filePath, []byte("exists"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	doc := &parser.ParsedDocument{
		FilePath: "exists.md",
		Objects: []*model.Object{
			{ID: "exists", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}
	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	// All files exist, should remove nothing
	removed, err := db.RemoveDeletedFiles(vaultDir)
	if err != nil {
		t.Fatalf("RemoveDeletedFiles: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed %d files, want 0", len(removed))
	}
}

// TestRemoveFiles_MultipleFiles tests removing multiple files at once.
func TestRemoveFiles_MultipleFiles(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)

	sch := schema.New()

	// Index three files
	for _, name := range []string{"file1.md", "file2.md", "file3.md"} {
		doc := &parser.ParsedDocument{
			FilePath: name,
			Objects: []*model.Object{
				{ID: name[:len(name)-3], Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
			},
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("IndexDocument(%s): %v", name, err)
		}
	}

	// Remove two files
	if err := db.RemoveFiles([]string{"file1.md", "file2.md"}); err != nil {
		t.Fatalf("RemoveFiles: %v", err)
	}

	// Verify only file3 remains
	allPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	if len(allPaths) != 1 {
		t.Fatalf("indexed %d files, want 1", len(allPaths))
	}
	if allPaths[0] != "file3.md" {
		t.Errorf("remaining file = %q, want %q", allPaths[0], "file3.md")
	}
}

// TestRemoveFiles_EmptyList tests that removing zero files is a no-op.
func TestRemoveFiles_EmptyList(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	if err := db.RemoveFiles([]string{}); err != nil {
		t.Fatalf("RemoveFiles([]): %v", err)
	}
	if err := db.RemoveFiles(nil); err != nil {
		t.Fatalf("RemoveFiles(nil): %v", err)
	}
}

// TestRemoveDocument_RemovesEntireFile tests that RemoveDocument removes all data
// for the file, including sections.
func TestRemoveDocument_RemovesEntireFile(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	sch := schema.New()

	doc := &parser.ParsedDocument{
		FilePath: "guide.md",
		Objects: []*model.Object{
			{ID: "guide", Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
		Sections: []*model.Section{
			{ID: "guide#intro", FileObjectID: "guide", FilePath: "guide.md", Slug: "intro", Title: "Intro", Level: 2, LineStart: 5},
		},
	}
	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	// Remove by section ID should still remove the entire file
	if err := db.RemoveDocument("guide#intro"); err != nil {
		t.Fatalf("RemoveDocument: %v", err)
	}

	// Verify nothing remains
	allPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	if len(allPaths) != 0 {
		t.Errorf("indexed %d files, want 0 (entire file should be removed)", len(allPaths))
	}
}

// TestClearAllData_FullReset tests that ClearAllData removes everything.
func TestClearAllData_FullReset(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	sch := schema.New()

	// Index some files
	for i := 1; i <= 3; i++ {
		doc := &parser.ParsedDocument{
			FilePath: filepath.Join("notes", fmt.Sprintf("file%d.md", i)),
			Objects: []*model.Object{
				{ID: fmt.Sprintf("notes/file%d", i), Type: "page", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
			},
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("IndexDocument(%d): %v", i, err)
		}
	}

	// Verify files are indexed
	allPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	if len(allPaths) != 3 {
		t.Fatalf("indexed %d files, want 3", len(allPaths))
	}

	// Clear all
	if err := db.ClearAllData(); err != nil {
		t.Fatalf("ClearAllData: %v", err)
	}

	// Verify nothing remains
	allPaths, err = db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths after clear: %v", err)
	}
	if len(allPaths) != 0 {
		t.Errorf("indexed %d files after clear, want 0", len(allPaths))
	}
}
