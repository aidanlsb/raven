package readsvc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
)

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
