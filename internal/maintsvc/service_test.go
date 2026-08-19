package maintsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func assertCode(t *testing.T, err error, want codes.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected maintsvc error, got %T: %v", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %q, want %q", svcErr.Code, want)
	}
}

func TestStats_InvalidInput(t *testing.T) {
	t.Parallel()
	_, err := Stats(&vaultruntime.Runtime{VaultPath: " "})
	assertCode(t, err, codes.ErrInvalidInput)
}

func TestStats_HappyPath(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to open index db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	_, err = db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('page/one', 'pages/one.md', 'page', 1, '{}'),
			('project/raven', 'projects/raven.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}
	_, err = db.DB().Exec(`
		INSERT INTO traits (id, trait_type, value, content, file_path, line_number, parent_object_id) VALUES
			('pages/one.md:trait:0', 'todo', 'open', 'Task', 'pages/one.md', 3, 'page/one')
	`)
	if err != nil {
		t.Fatalf("failed to insert traits: %v", err)
	}
	_, err = db.DB().Exec(`
		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number) VALUES
			('page/one', 'project/raven', 'project/raven', 'pages/one.md', 4)
	`)
	if err != nil {
		t.Fatalf("failed to insert refs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close db: %v", err)
	}

	rt := testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{
		SkipConfig: true,
		SkipSchema: true,
	})
	stats, err := Stats(rt)
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.ObjectCount != 2 || stats.TraitCount != 1 || stats.RefCount != 1 || stats.FileCount != 2 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}
