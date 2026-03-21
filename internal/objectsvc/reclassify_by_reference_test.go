package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestReclassifyByReferenceSuccess(t *testing.T) {
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields:
      author:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "notes/my-note.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: note\n---\n# My Note\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := ReclassifyByReference(ReclassifyByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		Reference:   "notes/my-note",
		NewTypeName: "book",
		FieldValues: map[string]string{"author": "Tolkien"},
		NoMove:      true,
		Force:       true,
	})
	if err != nil {
		t.Fatalf("ReclassifyByReference: %v", err)
	}
	if result.ObjectID != "notes/my-note" {
		t.Fatalf("expected object id notes/my-note, got %q", result.ObjectID)
	}
	if result.NewType != "book" {
		t.Fatalf("expected new type book, got %q", result.NewType)
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	content := string(updated)
	if !strings.Contains(content, "type: book") || !strings.Contains(content, "author: Tolkien") {
		t.Fatalf("expected updated frontmatter, got:\n%s", content)
	}
}

func TestReclassifyByReferenceMissingReference(t *testing.T) {
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields: {}
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	_, err := ReclassifyByReference(ReclassifyByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		Reference:   "notes/missing",
		NewTypeName: "book",
	})
	if err == nil {
		t.Fatal("expected missing reference error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorRefNotFound {
		t.Fatalf("expected ErrorRefNotFound, got %s", svcErr.Code)
	}
}
