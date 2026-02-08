package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestResolveObjectToFileWithRoots_DirectCandidates(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "people"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(vaultDir, "people", "freya.md")
	if err := os.WriteFile(target, []byte("# Freya\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ResolveObjectToFileWithRoots(vaultDir, "people/freya", "", "")
	if err != nil {
		t.Fatalf("ResolveObjectToFileWithRoots error: %v", err)
	}
	if got != target {
		t.Fatalf("got %q, want %q", got, target)
	}
}

func TestResolveObjectToFileWithRoots_WithDirectoryRoots(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "objects", "people"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(vaultDir, "pages"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	objPath := filepath.Join(vaultDir, "objects", "people", "freya.md")
	if err := os.WriteFile(objPath, []byte("# Freya\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	pagePath := filepath.Join(vaultDir, "pages", "my-note.md")
	if err := os.WriteFile(pagePath, []byte("# Note\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	gotObj, err := ResolveObjectToFileWithRoots(vaultDir, "people/freya", "objects/", "pages/")
	if err != nil {
		t.Fatalf("ResolveObjectToFileWithRoots object error: %v", err)
	}
	if gotObj != objPath {
		t.Fatalf("got %q, want %q", gotObj, objPath)
	}

	gotPage, err := ResolveObjectToFileWithRoots(vaultDir, "my-note", "objects/", "pages/")
	if err != nil {
		t.Fatalf("ResolveObjectToFileWithRoots page error: %v", err)
	}
	if gotPage != pagePath {
		t.Fatalf("got %q, want %q", gotPage, pagePath)
	}
}

func TestResolveObjectToFileWithRoots_SlugifiedFallback(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "objects", "people"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Only create lowercased file on disk.
	objPath := filepath.Join(vaultDir, "objects", "people", "sif.md")
	if err := os.WriteFile(objPath, []byte("# Sif\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Use mixed case ref so direct candidates are less likely to match on case-sensitive FS.
	got, err := ResolveObjectToFileWithRoots(vaultDir, "people/Sif", "objects/", "pages/")
	if err != nil {
		t.Fatalf("ResolveObjectToFileWithRoots error: %v", err)
	}
	// On case-insensitive filesystems, a candidate like ".../Sif.md" may stat successfully
	// and be returned even though the actual file was created as ".../sif.md".
	if !strings.EqualFold(got, objPath) {
		t.Fatalf("got %q, want (case-insensitive match) %q", got, objPath)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected resolved path to exist, stat(%q) error: %v", got, err)
	}
}

func TestResolveObjectToFileWithConfig_NormalizesRoots(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "objects", "people"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	objPath := filepath.Join(vaultDir, "objects", "people", "freya.md")
	if err := os.WriteFile(objPath, []byte("# Freya\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	vc := &config.VaultConfig{
		Directories: &config.DirectoriesConfig{
			Objects: "objects", // intentionally missing trailing slash
			Pages:   "pages",   // intentionally missing trailing slash
		},
	}

	got, err := ResolveObjectToFileWithConfig(vaultDir, "people/freya", vc)
	if err != nil {
		t.Fatalf("ResolveObjectToFileWithConfig error: %v", err)
	}
	if got != objPath {
		t.Fatalf("got %q, want %q", got, objPath)
	}
}

func TestResolveObjectToFileWithRoots_NotFound(t *testing.T) {
	vaultDir := t.TempDir()
	_, err := ResolveObjectToFileWithRoots(vaultDir, "people/missing.md", "objects/", "pages/")
	if err == nil || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
