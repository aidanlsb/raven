package readsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestBacklinksWithOptionsIncludesRootedReferences(t *testing.T) {
	t.Parallel()

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES ('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`); err != nil {
		t.Fatalf("insert object: %v", err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO refs (source_id, target_raw, file_path, line_number)
		VALUES ('projects/bifrost', 'objects/people/freya', 'projects/bifrost.md', 3)
	`); err != nil {
		t.Fatalf("insert reference: %v", err)
	}

	rt := &vaultruntime.Runtime{
		VaultPath: t.TempDir(),
		VaultCfg:  &config.VaultConfig{},
		DB:        db,
	}

	unrooted, err := Backlinks(rt, "people/freya")
	if err != nil {
		t.Fatalf("Backlinks() error = %v", err)
	}
	if len(unrooted) != 0 {
		t.Fatalf("Backlinks() returned %d rooted references, want 0", len(unrooted))
	}

	rooted, err := BacklinksWithOptions(rt, "people/freya", BacklinkOptions{ObjectsRoot: "objects/"})
	if err != nil {
		t.Fatalf("BacklinksWithOptions() error = %v", err)
	}
	if len(rooted) != 1 || rooted[0].TargetRaw != "objects/people/freya" {
		t.Fatalf("BacklinksWithOptions() = %#v, want rooted reference", rooted)
	}
}
