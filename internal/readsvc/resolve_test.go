package readsvc

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

func TestResolveReferenceLiteralPathsAndErrors(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultPath, "people"), 0o755); err != nil {
		t.Fatalf("create people directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "people", "freya.md"), []byte("# Freya\n"), 0o644); err != nil {
		t.Fatalf("write freya.md: %v", err)
	}

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rt := &Runtime{
		VaultPath: vaultPath,
		VaultCfg:  &config.VaultConfig{},
		DB:        db,
	}

	t.Run("literal path resolution", func(t *testing.T) {
		result, err := ResolveReference("people/freya", rt, false)
		if err != nil {
			t.Fatalf("ResolveReference failed: %v", err)
		}
		if result.ObjectID != "people/freya" || result.IsSection {
			t.Fatalf("result = %#v, want people/freya non-section", result)
		}
	})

	t.Run("literal path with .md extension", func(t *testing.T) {
		result, err := ResolveReference("people/freya.md", rt, false)
		if err != nil {
			t.Fatalf("ResolveReference failed: %v", err)
		}
		if result.ObjectID != "people/freya" {
			t.Fatalf("ObjectID = %q, want people/freya", result.ObjectID)
		}
	})

	t.Run("not found error", func(t *testing.T) {
		_, err := ResolveReference("people/nonexistent", rt, false)
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
		if !IsRefNotFound(err) {
			t.Fatalf("expected RefNotFoundError, got %T", err)
		}
	})

	t.Run("absolute path outside vault is not resolved", func(t *testing.T) {
		outsidePath := filepath.Join(t.TempDir(), "outside.md")
		if err := os.WriteFile(outsidePath, []byte("# Outside\n"), 0o644); err != nil {
			t.Fatalf("write outside.md: %v", err)
		}

		_, err := ResolveReference(outsidePath, rt, false)
		if err == nil {
			t.Fatal("expected error for outside-vault path")
		}
		if !IsRefNotFound(err) {
			t.Fatalf("expected RefNotFoundError, got %T", err)
		}
	})
}

func TestResolveErrorHelpers(t *testing.T) {
	t.Parallel()

	ambiguous := &AmbiguousRefError{
		Reference: "freya",
		Matches:   []string{"people/freya", "clients/freya"},
	}
	if !IsAmbiguousRef(ambiguous) {
		t.Fatal("IsAmbiguousRef should return true")
	}
	if ambiguous.Error() == "" {
		t.Fatal("ambiguous error message should not be empty")
	}

	notFound := &RefNotFoundError{Reference: "freya", Detail: "file was deleted"}
	if !IsRefNotFound(notFound) {
		t.Fatal("IsRefNotFound should return true")
	}
	if got := notFound.Error(); got != "reference 'freya' not found: file was deleted" {
		t.Fatalf("RefNotFoundError.Error() = %q", got)
	}
}

func TestResolveReferenceWithDynamicDates(t *testing.T) {
	t.Parallel()

	t.Run("allows missing dynamic keyword", func(t *testing.T) {
		t.Parallel()

		vaultPath := t.TempDir()
		vaultCfg := &config.VaultConfig{DailyDirectory: "journal"}
		rt := &Runtime{VaultPath: vaultPath, VaultCfg: vaultCfg}

		result, err := ResolveReferenceWithDynamicDates("today", rt, true)
		if err != nil {
			t.Fatalf("ResolveReferenceWithDynamicDates failed: %v", err)
		}

		parsed, err := vault.ParseDateArg("today")
		if err != nil {
			t.Fatalf("ParseDateArg failed: %v", err)
		}
		dateStr := vault.FormatDateISO(parsed)
		expectedID := path.Join(vaultCfg.DailyDirectory, dateStr)
		expectedPath := filepath.Join(vaultPath, vaultCfg.DailyDirectory, dateStr+".md")

		if result.ObjectID != expectedID || result.FileObjectID != expectedID || result.FilePath != expectedPath || result.IsSection {
			t.Fatalf("result = %#v, want object/file %s at %s", result, expectedID, expectedPath)
		}
	})

	t.Run("dynamic keyword honors section fragment", func(t *testing.T) {
		t.Parallel()

		vaultPath := t.TempDir()
		vaultCfg := &config.VaultConfig{DailyDirectory: "daily"}
		rt := &Runtime{VaultPath: vaultPath, VaultCfg: vaultCfg}

		result, err := ResolveReferenceWithDynamicDates("tomorrow#notes", rt, true)
		if err != nil {
			t.Fatalf("ResolveReferenceWithDynamicDates failed: %v", err)
		}

		parsed, err := vault.ParseDateArg("tomorrow")
		if err != nil {
			t.Fatalf("ParseDateArg failed: %v", err)
		}
		expectedBaseID := path.Join(vaultCfg.DailyDirectory, vault.FormatDateISO(parsed))
		if !result.IsSection || result.ObjectID != expectedBaseID+"#notes" || result.FileObjectID != expectedBaseID {
			t.Fatalf("result = %#v, want section under %s", result, expectedBaseID)
		}
	})

	t.Run("rejects missing dynamic keyword when not allowed", func(t *testing.T) {
		t.Parallel()

		rt := &Runtime{VaultPath: t.TempDir(), VaultCfg: &config.VaultConfig{DailyDirectory: "daily"}}
		_, err := ResolveReferenceWithDynamicDates("yesterday", rt, false)
		if err == nil {
			t.Fatal("expected missing dynamic note error")
		}
		if !IsRefNotFound(err) {
			t.Fatalf("expected RefNotFoundError, got %T", err)
		}
	})
}

func TestResolveOperationCachesResolverWithinOperation(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	for _, relPath := range []string{"projects/raven.md", "projects/atlas.md"} {
		fullPath := filepath.Join(vaultPath, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("failed to create fixture directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("# fixture\n"), 0o644); err != nil {
			t.Fatalf("failed to write fixture file: %v", err)
		}
	}

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('projects/raven', 'projects/raven.md', 'project', 1, '{}'),
			('projects/atlas', 'projects/atlas.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to seed objects: %v", err)
	}

	rt := &Runtime{
		VaultPath: vaultPath,
		VaultCfg:  &config.VaultConfig{},
		DB:        db,
	}

	op, err := newResolveOperation(rt)
	if err != nil {
		t.Fatalf("newResolveOperation returned error: %v", err)
	}
	defer op.Close()

	first, err := op.resolveReference("raven", false)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if first.ObjectID != "projects/raven" {
		t.Fatalf("first resolved object ID = %q, want %q", first.ObjectID, "projects/raven")
	}
	if op.resolver == nil {
		t.Fatal("expected resolver to be cached after first resolve")
	}
	firstResolver := op.resolver

	second, err := op.resolveReference("atlas", false)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if second.ObjectID != "projects/atlas" {
		t.Fatalf("second resolved object ID = %q, want %q", second.ObjectID, "projects/atlas")
	}
	if op.resolver != firstResolver {
		t.Fatal("expected resolver instance to be reused within one resolve operation")
	}
}

func TestResolveReferenceWithDynamicDates_AmbiguousISODateLiteralPath(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultPath, "daily"), 0o755); err != nil {
		t.Fatalf("create daily directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "2025-02-01.md"), []byte("# Literal\n"), 0o644); err != nil {
		t.Fatalf("write literal date file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "daily", "2025-02-01.md"), []byte("# Daily\n"), 0o644); err != nil {
		t.Fatalf("write daily date file: %v", err)
	}

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('daily/2025-02-01', 'daily/2025-02-01.md', 'page', 1, '{}')
	`); err != nil {
		t.Fatalf("failed to seed daily object: %v", err)
	}

	rt := &Runtime{
		VaultPath: vaultPath,
		VaultCfg:  &config.VaultConfig{DailyDirectory: "daily"},
		DB:        db,
	}

	_, err = ResolveReferenceWithDynamicDates("2025-02-01", rt, false)
	if err == nil {
		t.Fatal("expected ambiguous ISO date collision")
	}

	var ambiguous *AmbiguousRefError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected AmbiguousRefError, got %T: %v", err, err)
	}
	if !resolveMatchesContain(ambiguous.Matches, "2025-02-01") {
		t.Fatalf("expected literal object match in %v", ambiguous.Matches)
	}
	if !resolveMatchesContain(ambiguous.Matches, "daily/2025-02-01") {
		t.Fatalf("expected daily match in %v", ambiguous.Matches)
	}
	if got := ambiguous.MatchSources["2025-02-01"]; got != "literal_path" {
		t.Fatalf("literal match source = %q, want %q", got, "literal_path")
	}
}

func resolveMatchesContain(matches []string, want string) bool {
	for _, match := range matches {
		if match == want {
			return true
		}
	}
	return false
}
