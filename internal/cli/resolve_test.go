package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/vault"
)

func TestResolveReference(t *testing.T) {
	// Create a temp vault for testing
	tmpDir, err := os.MkdirTemp("", "resolve-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create schema.yaml
	schemaContent := `version: 2
types:
  person:
    name_field: name
    default_path: people/
    fields:
      name: { type: string, required: true }
      alias: { type: string }
`
	if err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Create test files
	peopleDir := filepath.Join(tmpDir, "people")
	if err := os.MkdirAll(peopleDir, 0755); err != nil {
		t.Fatalf("Failed to create people dir: %v", err)
	}

	freyaContent := `---
type: person
name: Freya
alias: The Queen
---

# Freya
`
	if err := os.WriteFile(filepath.Join(peopleDir, "freya.md"), []byte(freyaContent), 0644); err != nil {
		t.Fatalf("Failed to write freya.md: %v", err)
	}

	thorContent := `---
type: person
name: Thor
---

# Thor
`
	if err := os.WriteFile(filepath.Join(peopleDir, "thor.md"), []byte(thorContent), 0644); err != nil {
		t.Fatalf("Failed to write thor.md: %v", err)
	}

	// Initialize the database by running reindex
	// For testing, we'll use tryLiteralPath which doesn't need the database
	vaultCfg := &config.VaultConfig{}

	t.Run("literal path resolution", func(t *testing.T) {
		result, err := ResolveReference("people/freya", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			t.Fatalf("ResolveReference failed: %v", err)
		}
		if result.ObjectID != "people/freya" {
			t.Errorf("ObjectID = %q, want %q", result.ObjectID, "people/freya")
		}
		if result.IsSection {
			t.Error("Expected IsSection = false")
		}
	})

	t.Run("literal path with .md extension", func(t *testing.T) {
		result, err := ResolveReference("people/freya.md", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			t.Fatalf("ResolveReference failed: %v", err)
		}
		if result.ObjectID != "people/freya" {
			t.Errorf("ObjectID = %q, want %q", result.ObjectID, "people/freya")
		}
	})

	t.Run("not found error", func(t *testing.T) {
		_, err := ResolveReference("people/nonexistent", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		})
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
		if !IsRefNotFound(err) {
			t.Errorf("Expected RefNotFoundError, got %T", err)
		}
	})
}

func TestResolveReferenceErrors(t *testing.T) {
	t.Run("empty vault path", func(t *testing.T) {
		_, err := ResolveReference("test", ResolveOptions{
			VaultPath: "",
		})
		if err == nil {
			t.Error("Expected error for empty vault path")
		}
	})
}

func TestAmbiguousRefError(t *testing.T) {
	err := &AmbiguousRefError{
		Reference: "freya",
		Matches:   []string{"people/freya", "clients/freya"},
	}

	if !IsAmbiguousRef(err) {
		t.Error("IsAmbiguousRef should return true")
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
}

func TestRefNotFoundError(t *testing.T) {
	t.Run("without detail", func(t *testing.T) {
		err := &RefNotFoundError{Reference: "freya"}
		if !IsRefNotFound(err) {
			t.Error("IsRefNotFound should return true")
		}
		if err.Error() != "reference 'freya' not found" {
			t.Errorf("Unexpected error message: %s", err.Error())
		}
	})

	t.Run("with detail", func(t *testing.T) {
		err := &RefNotFoundError{Reference: "freya", Detail: "file was deleted"}
		msg := err.Error()
		if msg != "reference 'freya' not found: file was deleted" {
			t.Errorf("Unexpected error message: %s", msg)
		}
	})
}

func TestResolveReferenceWithDynamicDates(t *testing.T) {
	t.Run("falls back to dynamic keyword when not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultCfg := &config.VaultConfig{DailyDirectory: "journal"}

		result, err := resolveReferenceWithDynamicDates("today", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		}, true)
		if err != nil {
			t.Fatalf("resolveReferenceWithDynamicDates failed: %v", err)
		}

		parsed, err := vault.ParseDateArg("today")
		if err != nil {
			t.Fatalf("ParseDateArg failed: %v", err)
		}
		dateStr := vault.FormatDateISO(parsed)

		expectedID := filepath.Join(vaultCfg.DailyDirectory, dateStr)
		expectedPath := filepath.Join(tmpDir, vaultCfg.DailyDirectory, dateStr+".md")

		if result.ObjectID != expectedID {
			t.Errorf("ObjectID = %q, want %q", result.ObjectID, expectedID)
		}
		if result.FileObjectID != expectedID {
			t.Errorf("FileObjectID = %q, want %q", result.FileObjectID, expectedID)
		}
		if result.FilePath != expectedPath {
			t.Errorf("FilePath = %q, want %q", result.FilePath, expectedPath)
		}
		if result.IsSection {
			t.Error("Expected IsSection = false")
		}
	})

	t.Run("dynamic keyword honors section fragment", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultCfg := &config.VaultConfig{DailyDirectory: "daily"}

		result, err := resolveReferenceWithDynamicDates("tomorrow#notes", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		}, true)
		if err != nil {
			t.Fatalf("resolveReferenceWithDynamicDates failed: %v", err)
		}

		parsed, err := vault.ParseDateArg("tomorrow")
		if err != nil {
			t.Fatalf("ParseDateArg failed: %v", err)
		}
		dateStr := vault.FormatDateISO(parsed)
		expectedBaseID := filepath.Join(vaultCfg.DailyDirectory, dateStr)

		if !result.IsSection {
			t.Error("Expected IsSection = true")
		}
		if result.ObjectID != expectedBaseID+"#notes" {
			t.Errorf("ObjectID = %q, want %q", result.ObjectID, expectedBaseID+"#notes")
		}
		if result.FileObjectID != expectedBaseID {
			t.Errorf("FileObjectID = %q, want %q", result.FileObjectID, expectedBaseID)
		}
	})

	t.Run("normal resolution wins over dynamic keyword", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultCfg := &config.VaultConfig{}

		todayPath := filepath.Join(tmpDir, "today.md")
		if err := os.WriteFile(todayPath, []byte("# Today"), 0644); err != nil {
			t.Fatalf("Failed to write today.md: %v", err)
		}

		result, err := resolveReferenceWithDynamicDates("today", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		}, true)
		if err != nil {
			t.Fatalf("resolveReferenceWithDynamicDates failed: %v", err)
		}
		if result.ObjectID != "today" {
			t.Errorf("ObjectID = %q, want %q", result.ObjectID, "today")
		}
		if result.FilePath != todayPath {
			t.Errorf("FilePath = %q, want %q", result.FilePath, todayPath)
		}
	})

	t.Run("missing dynamic keyword respects allowMissing", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultCfg := &config.VaultConfig{}

		_, err := resolveReferenceWithDynamicDates("yesterday", ResolveOptions{
			VaultPath:   tmpDir,
			VaultConfig: vaultCfg,
		}, false)
		if err == nil {
			t.Fatal("Expected error for missing daily note")
		}
		if !IsRefNotFound(err) {
			t.Fatalf("Expected RefNotFoundError, got %T", err)
		}
	})
}
