package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestWriteCreateUpdateUnchanged(t *testing.T) {
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

	req := WriteRequest{
		VaultPath:   vaultPath,
		TypeName:    "brief",
		Title:       "Daily Brief 2026-02-14",
		TargetPath:  "Daily Brief 2026-02-14",
		ReplaceBody: true,
		Content:     "# Brief V1",
		Schema:      sch,
	}

	created, err := Write(req)
	if err != nil {
		t.Fatalf("Write(create): %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("expected created status, got %q", created.Status)
	}

	unchanged, err := Write(req)
	if err != nil {
		t.Fatalf("Write(unchanged): %v", err)
	}
	if unchanged.Status != "unchanged" {
		t.Fatalf("expected unchanged status, got %q", unchanged.Status)
	}

	req.Content = "# Brief V2"
	updated, err := Write(req)
	if err != nil {
		t.Fatalf("Write(update): %v", err)
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

func TestWriteMissingRequiredField(t *testing.T) {
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

	_, err := Write(WriteRequest{
		VaultPath:  vaultPath,
		TypeName:   "task",
		Title:      "Write tests",
		TargetPath: "Write tests",
		Schema:     sch,
	})
	if err == nil {
		t.Fatal("expected required-field error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrRequiredFieldMissing {
		t.Fatalf("expected ErrorRequiredField, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "status") {
		t.Fatalf("expected missing status message, got %q", svcErr.Message)
	}
}

func TestWriteTypeMismatchExistingObject(t *testing.T) {
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
	_, err := Write(WriteRequest{
		VaultPath:  vaultPath,
		TypeName:   "brief",
		Title:      "Shared Object",
		TargetPath: path,
		Schema:     sch,
	})
	if err != nil {
		t.Fatalf("setup write: %v", err)
	}

	_, err = Write(WriteRequest{
		VaultPath:  vaultPath,
		TypeName:   "note",
		Title:      "Shared Object",
		TargetPath: path,
		Schema:     sch,
	})
	if err == nil {
		t.Fatal("expected type mismatch error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrValidationFailed {
		t.Fatalf("expected ErrorValidationFailed, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "cannot write as") {
		t.Fatalf("unexpected error message: %q", svcErr.Message)
	}
}

func TestWritePreservesStringTypeFromTypedFieldValues(t *testing.T) {
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

	result, err := Write(WriteRequest{
		VaultPath:  vaultPath,
		TypeName:   "person",
		Title:      "Typed Write Freya",
		TargetPath: "Typed Write Freya",
		FieldValues: map[string]fieldvalue.FieldValue{
			"email": fieldvalue.String("true"),
		},
		Schema: sch,
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
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

func TestWriteCreateAppliesDefaultTemplate(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
version: 2
templates:
  interview_default:
    file: templates/interview/default.md
types:
  interview:
    default_path: interviews/
    name_field: title
    templates: [interview_default]
    default_template: interview_default
    fields:
      title:
        type: string
        required: true
traits: {}
`)
	if err := os.MkdirAll(filepath.Join(vaultPath, "templates", "interview"), 0o755); err != nil {
		t.Fatalf("mkdir templates/interview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "templates", "interview", "default.md"), []byte("## Interview Template\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	sch := loadTestSchema(t, vaultPath)

	result, err := Write(WriteRequest{
		VaultPath:  vaultPath,
		TypeName:   "interview",
		Title:      "Jane Doe",
		TargetPath: "Jane Doe",
		Schema:     sch,
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if result.Status != "created" {
		t.Fatalf("expected created status, got %q", result.Status)
	}

	created, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if !strings.Contains(string(created), "## Interview Template") {
		t.Fatalf("expected default template content, got:\n%s", string(created))
	}
}

func TestWriteCreateOnlyDoesNotReplaceExisting(t *testing.T) {
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

	created, err := Write(WriteRequest{
		VaultPath:  vaultPath,
		TypeName:   "person",
		Title:      "Freya",
		TargetPath: "Freya",
		Schema:     sch,
	})
	if err != nil {
		t.Fatalf("Write(create): %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("expected created status, got %q", created.Status)
	}
	original := "---\ntype: person\nname: Freya\n---\n\nKeep me.\n"
	if err := os.WriteFile(created.FilePath, []byte(original), 0o644); err != nil {
		t.Fatalf("seed existing body: %v", err)
	}

	_, err = Write(WriteRequest{
		VaultPath:   vaultPath,
		TypeName:    "person",
		Title:       "Freya",
		TargetPath:  "Freya",
		ReplaceBody: true,
		Content:     "# replaced",
		CreateOnly:  true,
		Schema:      sch,
	})
	if err == nil {
		t.Fatal("expected create-only write to fail when the object exists")
	}
	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrFileExists {
		t.Fatalf("expected FILE_EXISTS, got %s", svcErr.Code)
	}

	got, err := os.ReadFile(created.FilePath)
	if err != nil {
		t.Fatalf("read existing file: %v", err)
	}
	if string(got) != original {
		t.Fatalf("create-only write clobbered existing object, got:\n%s", got)
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
