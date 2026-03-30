package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestReclassifyMoveWritesUpdatedContentAtDestination(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields:
      title:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	sourcePath := filepath.Join(vaultPath, "notes/my-note.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("---\ntype: note\ntitle: My Note\n---\n\nContent.\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := Reclassify(ReclassifyRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectID:    "notes/my-note",
		FilePath:    sourcePath,
		NewTypeName: "book",
		Force:       true,
	})
	if err != nil {
		t.Fatalf("Reclassify() error = %v", err)
	}
	if !result.Moved {
		t.Fatalf("expected moved result, got %#v", result)
	}

	destPath := filepath.Join(vaultPath, "books/my-note.md")
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if !strings.Contains(string(content), "type: book") {
		t.Fatalf("expected reclassified type in moved file, got:\n%s", string(content))
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("expected source file removed, err=%v", err)
	}
}

func TestReclassifyMoveFailureLeavesSourceUntouched(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields:
      title:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	sourcePath := filepath.Join(vaultPath, "notes/my-note.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("---\ntype: note\ntitle: My Note\n---\n\nContent.\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	destDir := filepath.Join(vaultPath, "books")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir destination dir: %v", err)
	}
	if err := os.Chmod(destDir, 0o555); err != nil {
		t.Fatalf("chmod destination dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(destDir, 0o755) })

	_, err := Reclassify(ReclassifyRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectID:    "notes/my-note",
		FilePath:    sourcePath,
		NewTypeName: "book",
		Force:       true,
	})
	if err == nil {
		t.Fatal("expected Reclassify() to fail")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorFileWrite {
		t.Fatalf("error code = %s, want %s", svcErr.Code, ErrorFileWrite)
	}

	content, readErr := os.ReadFile(sourcePath)
	if readErr != nil {
		t.Fatalf("read source after failure: %v", readErr)
	}
	if !strings.Contains(string(content), "type: note") {
		t.Fatalf("expected source file unchanged after failed move, got:\n%s", string(content))
	}
	if strings.Contains(string(content), "type: book") {
		t.Fatalf("source file was rewritten despite failed move:\n%s", string(content))
	}
}
