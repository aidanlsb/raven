package readsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
)

func TestResolveOperationCachesResolverWithinOperation(t *testing.T) {
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
