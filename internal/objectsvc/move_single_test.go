package objectsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestMoveByReferenceSuccess(t *testing.T) {
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

	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := MoveByReference(MoveByReferenceRequest{
		VaultPath:      vaultPath,
		VaultConfig:    &config.VaultConfig{},
		Schema:         sch,
		Reference:      "people/freya",
		Destination:    "archive/freya-archived",
		UpdateRefs:     true,
		SkipTypeCheck:  true,
		FailOnIndexErr: false,
	})
	if err != nil {
		t.Fatalf("MoveByReference: %v", err)
	}
	if result.NeedsConfirm {
		t.Fatal("expected move to proceed without confirmation")
	}
	if result.SourceID != "people/freya" {
		t.Fatalf("expected source id people/freya, got %q", result.SourceID)
	}
	if result.DestinationID != "archive/freya-archived" {
		t.Fatalf("expected destination id archive/freya-archived, got %q", result.DestinationID)
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "archive/freya-archived.md")); err != nil {
		t.Fatalf("expected destination file to exist: %v", err)
	}
}

func TestMoveByReferenceTypeMismatchNeedsConfirm(t *testing.T) {
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
  project:
    default_path: projects/
    name_field: name
    fields:
      name:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := MoveByReference(MoveByReferenceRequest{
		VaultPath:      vaultPath,
		VaultConfig:    &config.VaultConfig{},
		Schema:         sch,
		Reference:      "people/freya",
		Destination:    "projects/freya",
		UpdateRefs:     true,
		SkipTypeCheck:  false,
		FailOnIndexErr: false,
	})
	if err != nil {
		t.Fatalf("MoveByReference: %v", err)
	}
	if !result.NeedsConfirm {
		t.Fatal("expected type mismatch to require confirmation")
	}
	if result.TypeMismatch == nil {
		t.Fatal("expected mismatch details")
	}
	if result.TypeMismatch.ExpectedType != "project" {
		t.Fatalf("expected expected type project, got %q", result.TypeMismatch.ExpectedType)
	}
	if result.TypeMismatch.ActualType != "person" {
		t.Fatalf("expected actual type person, got %q", result.TypeMismatch.ActualType)
	}
}
