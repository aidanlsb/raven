package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestSetEmbeddedObjectSuccess(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  meeting:
    default_path: meetings/
    fields:
      status:
        type: string
      count:
        type: number
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "notes/day.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `---
type: page
---
# Standup
::meeting(status=planned, count=1)
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := SetEmbeddedObject(SetEmbeddedObjectRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		ObjectID:  "notes/day#standup",
		Updates: map[string]string{
			"status": "done",
		},
		TypedUpdates: map[string]schema.FieldValue{
			"count": schema.Number(2),
		},
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true, "id": true},
	})
	if err != nil {
		t.Fatalf("SetEmbeddedObject: %v", err)
	}

	if result.ObjectType != "meeting" {
		t.Fatalf("expected object type meeting, got %s", result.ObjectType)
	}
	if got := result.ResolvedUpdates["status"]; got != "done" {
		t.Fatalf("expected resolved status=done, got %q", got)
	}
	if got := result.ResolvedUpdates["count"]; got != "2" {
		t.Fatalf("expected resolved count=2, got %q", got)
	}
	if old, ok := result.PreviousFields["status"]; !ok {
		t.Fatalf("expected previous status in result")
	} else if oldStatus, ok := old.AsString(); !ok || oldStatus != "planned" {
		t.Fatalf("expected previous status planned, got %#v", old.Raw())
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	updatedContent := string(updated)
	if !strings.Contains(updatedContent, "::meeting(") {
		t.Fatalf("expected embedded type declaration to remain, got:\n%s", updatedContent)
	}
	if !strings.Contains(updatedContent, "status=done") {
		t.Fatalf("expected updated status in declaration, got:\n%s", updatedContent)
	}
	if !strings.Contains(updatedContent, "count=2") {
		t.Fatalf("expected updated count in declaration, got:\n%s", updatedContent)
	}
}

func TestSetEmbeddedObjectInvalidID(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  meeting:
    default_path: meetings/
    fields:
      status:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	_, err := SetEmbeddedObject(SetEmbeddedObjectRequest{
		VaultPath: vaultPath,
		FilePath:  filepath.Join(vaultPath, "notes/day.md"),
		ObjectID:  "notes/day",
		Updates: map[string]string{
			"status": "done",
		},
		Schema: sch,
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

func TestSetEmbeddedObjectUnknownField(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  meeting:
    default_path: meetings/
    fields:
      status:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "notes/day.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `---
type: page
---
# Standup
::meeting(status=planned)
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := SetEmbeddedObject(SetEmbeddedObjectRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		ObjectID:  "notes/day#standup",
		Updates: map[string]string{
			"unknown": "x",
		},
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true, "id": true},
	})
	if err == nil {
		t.Fatal("expected unknown field error")
	}

	var unknownErr *fieldmutation.UnknownFieldMutationError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownFieldMutationError, got %T", err)
	}
}
