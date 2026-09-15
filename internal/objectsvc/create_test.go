package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/svcerr"
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
	rt := testRuntime(t, vaultPath)
	result, err := Create(rt, CreateRequest{
		TypeName:   "person",
		Title:      "Freya",
		TargetPath: "Freya",
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

func TestCreateSlugifiesTitleWhenNoExplicitPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		title      string
		wantRel    string
		wantFMLine string
	}{
		{
			name:       "path separator in title",
			title:      "config.VaultConfig duplicates internal/paths",
			wantRel:    "notes/config-vaultconfig-duplicates-internal-paths.md",
			wantFMLine: "name: config.VaultConfig duplicates internal/paths",
		},
		{
			name:       "spaces in title",
			title:      "Raven Move Friction",
			wantRel:    "notes/raven-move-friction.md",
			wantFMLine: "name: Raven Move Friction",
		},
		{
			name:       "unicode in title",
			title:      "Über Café",
			wantRel:    "notes/uber-cafe.md",
			wantFMLine: "name: Über Café",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    name_field: name
    fields:
      name:
        type: string
        required: true
traits: {}
`)
			rt := testRuntime(t, vaultPath)
			// No TargetPath: the path/slug is derived from the title.
			result, err := Create(rt, CreateRequest{
				TypeName: "note",
				Title:    tc.title,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if result.RelativePath != tc.wantRel {
				t.Fatalf("expected relative path %q, got %q", tc.wantRel, result.RelativePath)
			}

			created, err := os.ReadFile(result.FilePath)
			if err != nil {
				t.Fatalf("read created file: %v", err)
			}
			if !strings.Contains(string(created), tc.wantFMLine) {
				t.Fatalf("expected verbatim title line %q in frontmatter, got:\n%s", tc.wantFMLine, string(created))
			}
		})
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
	rt := testRuntime(t, vaultPath)
	_, err := Create(rt, CreateRequest{
		TypeName:   "task",
		Title:      "Write tests",
		TargetPath: "Write tests",
	})
	if err == nil {
		t.Fatal("expected required field error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrRequiredFieldMissing {
		t.Fatalf("expected ErrorRequiredField, got %s", svcErr.Code)
	}
}

func TestCreateUnknownFieldReturnsValidationFailed(t *testing.T) {
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
	rt := testRuntime(t, vaultPath)
	_, err := Create(rt, CreateRequest{
		TypeName:   "person",
		Title:      "Freya",
		TargetPath: "Freya",
		FieldValues: map[string]fieldvalue.FieldValue{
			"favorite_color": fieldvalue.String("blue"),
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrValidationFailed {
		t.Fatalf("expected ErrorValidationFailed, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "unknown field 'favorite_color'") {
		t.Fatalf("unexpected error message: %q", svcErr.Message)
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
	rt := testRuntime(t, vaultPath)
	if err := os.MkdirAll(filepath.Join(vaultPath, "people"), 0o755); err != nil {
		t.Fatalf("mkdir people: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "people/freya.md"), []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := Create(rt, CreateRequest{
		TypeName:   "person",
		Title:      "Freya",
		TargetPath: "Freya",
	})
	if err == nil {
		t.Fatal("expected file exists error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrFileExists {
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
	rt := testRuntime(t, vaultPath)
	notePath := filepath.Join(vaultPath, "notes/overview.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("---\ntype: note\ntitle: Overview\n---\n"), 0o644); err != nil {
		t.Fatalf("seed note: %v", err)
	}

	_, err := Create(rt, CreateRequest{
		TypeName:   "issue",
		Title:      "Broken parent",
		TargetPath: "Broken parent",
		FieldValues: map[string]fieldvalue.FieldValue{
			"parent": fieldvalue.Ref("notes/overview"),
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrValidationFailed {
		t.Fatalf("expected ErrorValidationFailed, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "expected 'page'") {
		t.Fatalf("expected page target mismatch, got %q", svcErr.Message)
	}
}

func TestCreateRejectsUnsupportedFieldTypeInSchema(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  task:
    default_path: task/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: enum-ish
traits: {}
`)
	rt := testRuntime(t, vaultPath)
	_, err := Create(rt, CreateRequest{
		TypeName:   "task",
		Title:      "Broken schema",
		TargetPath: "Broken schema",
		FieldValues: map[string]fieldvalue.FieldValue{
			"status": fieldvalue.String("open"),
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrValidationFailed {
		t.Fatalf("expected ErrorValidationFailed, got %s", svcErr.Code)
	}
	if !strings.Contains(svcErr.Message, "unsupported field type 'enum-ish'") {
		t.Fatalf("unexpected error message: %q", svcErr.Message)
	}
}

func TestCreatePreservesStringTypeFromTypedFieldValues(t *testing.T) {
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
	rt := testRuntime(t, vaultPath)
	result, err := Create(rt, CreateRequest{
		TypeName:   "person",
		Title:      "Typed Freya",
		TargetPath: "Typed Freya",
		FieldValues: map[string]fieldvalue.FieldValue{
			"email": fieldvalue.String("true"),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	created, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if !strings.Contains(string(created), `email: "true"`) {
		t.Fatalf("expected email to remain a string, got:\n%s", string(created))
	}
}
