package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
)

func TestSetObjectFileSuccess(t *testing.T) {
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

	result, err := SetObjectFile(SetObjectFileRequest{
		FilePath:      filePath,
		ObjectID:      "people/freya",
		Updates:       map[string]string{"email": "new@example.com"},
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err != nil {
		t.Fatalf("SetObjectFile: %v", err)
	}
	if result.ObjectType != "person" {
		t.Fatalf("expected object type person, got %s", result.ObjectType)
	}
	if result.ResolvedUpdates["email"] != "new@example.com" {
		t.Fatalf("expected resolved email update")
	}
	if old, ok := result.PreviousFields["email"]; !ok {
		t.Fatalf("expected previous email in result")
	} else if oldEmail, ok := old.AsString(); !ok || oldEmail != "old@example.com" {
		t.Fatalf("expected previous email old@example.com, got %#v", old.Raw())
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if !strings.Contains(string(updated), "email: new@example.com") {
		t.Fatalf("expected updated email in file, got:\n%s", string(updated))
	}
}

func TestSetObjectFileNoFrontmatter(t *testing.T) {
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

	filePath := filepath.Join(vaultPath, "note.md")
	if err := os.WriteFile(filePath, []byte("# no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := SetObjectFile(SetObjectFileRequest{
		FilePath:      filePath,
		ObjectID:      "note",
		Updates:       map[string]string{"status": "done"},
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorInvalidInput {
		t.Fatalf("expected ErrorInvalidInput, got %s", svcErr.Code)
	}
}

func TestSetObjectFileUnknownField(t *testing.T) {
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

	_, err := SetObjectFile(SetObjectFileRequest{
		FilePath:      filePath,
		ObjectID:      "people/freya",
		Updates:       map[string]string{"unknown": "x"},
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err == nil {
		t.Fatal("expected unknown field error")
	}

	var unknownErr *fieldmutation.UnknownFieldMutationError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownFieldMutationError, got %T", err)
	}
}

func TestSetObjectFileRejectsWrongRefTargetType(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  company:
    default_path: companies/
    fields:
      name:
        type: string
        required: true
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
      employer:
        type: ref
        target: person
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	companyPath := filepath.Join(vaultPath, "companies/acme.md")
	if err := os.MkdirAll(filepath.Dir(companyPath), 0o755); err != nil {
		t.Fatalf("mkdir companies: %v", err)
	}
	if err := os.WriteFile(companyPath, []byte("---\ntype: company\nname: Acme\n---\n"), 0o644); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	personPath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(personPath), 0o755); err != nil {
		t.Fatalf("mkdir people: %v", err)
	}
	if err := os.WriteFile(personPath, []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed person: %v", err)
	}

	_, err := SetObjectFile(SetObjectFileRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		FilePath:    personPath,
		ObjectID:    "people/freya",
		Updates: map[string]string{
			"employer": "[[companies/acme]]",
		},
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var validationErr *fieldmutation.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if len(validationErr.Issues) != 1 {
		t.Fatalf("expected 1 validation issue, got %d", len(validationErr.Issues))
	}
	if !strings.Contains(validationErr.Issues[0].Message, "expected 'person'") {
		t.Fatalf("unexpected validation message: %q", validationErr.Issues[0].Message)
	}
}
