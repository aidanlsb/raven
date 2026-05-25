package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
)

func TestWalkAssetFilesWithOptionsExcludesPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	for _, relPath := range []string{
		"assets/pdfs/keep.pdf",
		"assets/generated/drop.pdf",
	} {
		fullPath := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte("asset"), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	matcher, err := ravenignore.NewMatcher([]string{"assets/generated/**"})
	if err != nil {
		t.Fatalf("NewMatcher returned error: %v", err)
	}

	var found []string
	err = WalkAssetFilesWithOptions(tmpDir, config.DefaultVaultConfig(), &AssetWalkOptions{ExcludeMatcher: matcher}, func(result AssetWalkResult) error {
		if result.Asset != nil {
			found = append(found, result.RelativePath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkAssetFilesWithOptions returned error: %v", err)
	}
	if len(found) != 1 || found[0] != "assets/pdfs/keep.pdf" {
		t.Fatalf("found assets = %#v, want only keep asset", found)
	}
}
