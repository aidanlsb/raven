package objectsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestDeleteByReferenceSuccess(t *testing.T) {
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
traits: {}
`)

	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := DeleteByReference(DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Reference:   "people/freya",
		Behavior:    "trash",
		TrashDir:    ".trash",
	})
	if err != nil {
		t.Fatalf("DeleteByReference: %v", err)
	}
	if result.ObjectID != "people/freya" {
		t.Fatalf("expected deleted object people/freya, got %q", result.ObjectID)
	}
	if result.Behavior != "trash" {
		t.Fatalf("expected behavior trash, got %q", result.Behavior)
	}
	if result.TrashPath == "" {
		t.Fatal("expected trash path to be set")
	}
	if _, err := os.Stat(result.TrashPath); err != nil {
		t.Fatalf("expected trashed file to exist: %v", err)
	}
}
