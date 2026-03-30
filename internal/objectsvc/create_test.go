package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestCreateObjectSuccess(t *testing.T) {
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

	result, err := Create(CreateRequest{
		VaultPath:  vaultPath,
		TypeName:   "person",
		Title:      "Freya",
		TargetPath: "Freya",
		Schema:     sch,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.RelativePath != "people/freya.md" {
		t.Fatalf("expected people/freya.md, got %s", result.RelativePath)
	}
	if _, err := os.Stat(result.FilePath); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestCreateMissingRequiredField(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  task:
    default_path: task/
    name_field: name
    fields:
      name:
        type: string
        required: true
      status:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	_, err := Create(CreateRequest{
		VaultPath:  vaultPath,
		TypeName:   "task",
		Title:      "Write tests",
		TargetPath: "Write tests",
		Schema:     sch,
	})
	if err == nil {
		t.Fatal("expected required field error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorRequiredField {
		t.Fatalf("expected ErrorRequiredField, got %s", svcErr.Code)
	}
}

func TestCreateRejectsExistingFile(t *testing.T) {
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

	if err := os.MkdirAll(filepath.Join(vaultPath, "people"), 0o755); err != nil {
		t.Fatalf("mkdir people: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "people/freya.md"), []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := Create(CreateRequest{
		VaultPath:  vaultPath,
		TypeName:   "person",
		Title:      "Freya",
		TargetPath: "Freya",
		Schema:     sch,
	})
	if err == nil {
		t.Fatal("expected file exists error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorFileExists {
		t.Fatalf("expected ErrorFileExists, got %s", svcErr.Code)
	}
}

func TestCreateRejectsWrongRefTargetType(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields:
      title:
        type: string
  issue:
    default_path: issues/
    name_field: title
    fields:
      title:
        type: string
        required: true
      parent:
        type: ref
        target: page
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	notePath := filepath.Join(vaultPath, "notes/overview.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("---\ntype: note\ntitle: Overview\n---\n"), 0o644); err != nil {
		t.Fatalf("seed note: %v", err)
	}

	_, err := Create(CreateRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		TypeName:    "issue",
		Title:       "Broken parent",
		TargetPath:  "Broken parent",
		FieldValues: map[string]string{
			"parent": "[[notes/overview]]",
		},
		Schema: sch,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorValidationFailed {
		t.Fatalf("expected ErrorValidationFailed, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "expected 'page'") {
		t.Fatalf("expected page target mismatch, got %q", svcErr.Message)
	}
}
