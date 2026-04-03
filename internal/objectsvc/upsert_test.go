package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestUpsertCreateUpdateUnchanged(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  brief:
    default_path: brief/
    name_field: title
    fields:
      title:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	req := UpsertRequest{
		VaultPath:   vaultPath,
		TypeName:    "brief",
		Title:       "Daily Brief 2026-02-14",
		TargetPath:  "Daily Brief 2026-02-14",
		ReplaceBody: true,
		Content:     "# Brief V1",
		Schema:      sch,
	}

	created, err := Upsert(req)
	if err != nil {
		t.Fatalf("Upsert(create): %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("expected created status, got %q", created.Status)
	}

	unchanged, err := Upsert(req)
	if err != nil {
		t.Fatalf("Upsert(unchanged): %v", err)
	}
	if unchanged.Status != "unchanged" {
		t.Fatalf("expected unchanged status, got %q", unchanged.Status)
	}

	req.Content = "# Brief V2"
	updated, err := Upsert(req)
	if err != nil {
		t.Fatalf("Upsert(update): %v", err)
	}
	if updated.Status != "updated" {
		t.Fatalf("expected updated status, got %q", updated.Status)
	}

	b, err := os.ReadFile(updated.FilePath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "# Brief V2") {
		t.Fatalf("expected updated body content, got:\n%s", content)
	}
	if strings.Contains(content, "# Brief V1") {
		t.Fatalf("expected old body to be replaced, got:\n%s", content)
	}
}

func TestUpsertMissingRequiredField(t *testing.T) {
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

	_, err := Upsert(UpsertRequest{
		VaultPath:  vaultPath,
		TypeName:   "task",
		Title:      "Write tests",
		TargetPath: "Write tests",
		Schema:     sch,
	})
	if err == nil {
		t.Fatal("expected required-field error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorRequiredField {
		t.Fatalf("expected ErrorRequiredField, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "status") {
		t.Fatalf("expected missing status message, got %q", svcErr.Message)
	}
}

func TestUpsertTypeMismatchExistingObject(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  brief:
    default_path: brief/
    name_field: title
    fields:
      title:
        type: string
        required: true
  note:
    default_path: note/
    name_field: title
    fields:
      title:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	path := "shared/object"
	_, err := Upsert(UpsertRequest{
		VaultPath:  vaultPath,
		TypeName:   "brief",
		Title:      "Shared Object",
		TargetPath: path,
		Schema:     sch,
	})
	if err != nil {
		t.Fatalf("setup upsert: %v", err)
	}

	_, err = Upsert(UpsertRequest{
		VaultPath:  vaultPath,
		TypeName:   "note",
		Title:      "Shared Object",
		TargetPath: path,
		Schema:     sch,
	})
	if err == nil {
		t.Fatal("expected type mismatch error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorValidationFailed {
		t.Fatalf("expected ErrorValidationFailed, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "cannot upsert as") {
		t.Fatalf("unexpected error message: %q", svcErr.Message)
	}
}

func TestUpsertPreservesStringTypeFromTypedFieldValues(t *testing.T) {
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

	result, err := Upsert(UpsertRequest{
		VaultPath:  vaultPath,
		TypeName:   "person",
		Title:      "Typed Upsert Freya",
		TargetPath: "Typed Upsert Freya",
		FieldValues: map[string]schema.FieldValue{
			"email": schema.String("true"),
		},
		Schema: sch,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if result.Status != "created" {
		t.Fatalf("expected created status, got %q", result.Status)
	}

	created, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if !strings.Contains(string(created), `email: "true"`) {
		t.Fatalf("expected email to remain a string, got:\n%s", string(created))
	}
}

func writeTestSchema(t *testing.T, vaultPath, content string) {
	t.Helper()
	path := filepath.Join(vaultPath, "schema.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func loadTestSchema(t *testing.T, vaultPath string) *schema.Schema {
	t.Helper()
	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	return sch
}
