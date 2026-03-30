package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestSetByReferenceSuccess(t *testing.T) {
	t.Parallel()
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
      email:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: Freya\nemail: old@example.com\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := SetByReference(SetByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		Reference:   "people/freya",
		Updates:     map[string]string{"email": "new@example.com"},
	})
	if err != nil {
		t.Fatalf("SetByReference: %v", err)
	}
	if result.Embedded {
		t.Fatalf("expected non-embedded result")
	}
	if result.ObjectID != "people/freya" {
		t.Fatalf("expected object id people/freya, got %q", result.ObjectID)
	}
	if result.RelativePath != "people/freya.md" {
		t.Fatalf("expected relative path people/freya.md, got %q", result.RelativePath)
	}
	if got := result.ResolvedUpdates["email"]; got != "new@example.com" {
		t.Fatalf("expected resolved update email=new@example.com, got %q", got)
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if !strings.Contains(string(updated), "email: new@example.com") {
		t.Fatalf("expected updated email in file, got:\n%s", string(updated))
	}
}

func TestSetByReferenceMissingReference(t *testing.T) {
	t.Parallel()
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
	sch := loadTestSchema(t, vaultPath)

	_, err := SetByReference(SetByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		Reference:   "people/missing",
		Updates:     map[string]string{"alias": "ghost"},
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
