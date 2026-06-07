package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestUnsetObjectFileRemovesUnknownField(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  doc:
    default_path: docs/
    fields:
      title:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "docs/cleanup.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	originalBody := "# Cleanup\n\nBody stays put.\n"
	if err := os.WriteFile(filePath, []byte("---\ntype: doc\ndate: 2026-06-07\nlink: https://example.com\ntitle: Cleanup\n---\n"+originalBody), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := UnsetObjectFile(UnsetObjectFileRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		FilePath:    filePath,
		ObjectID:    "docs/cleanup",
		Fields:      []string{"date", "missing"},
		Schema:      sch,
	})
	if err != nil {
		t.Fatalf("UnsetObjectFile: %v", err)
	}
	if !result.Modified {
		t.Fatalf("expected modified result")
	}
	if _, ok := result.RemovedFields["date"]; !ok {
		t.Fatalf("expected date in removed fields: %#v", result.RemovedFields)
	}
	if len(result.MissingFields) != 1 || result.MissingFields[0] != "missing" {
		t.Fatalf("missing fields = %#v, want [missing]", result.MissingFields)
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	updatedText := string(updated)
	if strings.Contains(updatedText, "date:") {
		t.Fatalf("expected date to be removed, got:\n%s", updatedText)
	}
	if !strings.Contains(updatedText, "link: https://example.com") {
		t.Fatalf("expected unrelated frontmatter to remain, got:\n%s", updatedText)
	}
	if !strings.HasSuffix(updatedText, originalBody) {
		t.Fatalf("expected body to remain unchanged, got:\n%s", updatedText)
	}
}

func TestUnsetObjectFileRejectsReservedTypeField(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  doc:
    default_path: docs/
    fields:
      title:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	filePath := filepath.Join(vaultPath, "docs/cleanup.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: doc\ntitle: Cleanup\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := UnsetObjectFile(UnsetObjectFileRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		FilePath:    filePath,
		ObjectID:    "docs/cleanup",
		Fields:      []string{"type"},
		Schema:      sch,
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
