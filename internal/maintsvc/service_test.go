package maintsvc

import (
	"runtime/debug"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
)

func assertCode(t *testing.T, err error, want Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected maintsvc error, got %T: %v", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %q, want %q", svcErr.Code, want)
	}
}

func TestStats_InvalidInput(t *testing.T) {
	_, err := Stats(" ")
	assertCode(t, err, CodeInvalidInput)
}

func TestStats_HappyPath(t *testing.T) {
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

	stats, err := Stats(vaultPath)
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.ObjectCount != 2 || stats.TraitCount != 1 || stats.RefCount != 1 || stats.FileCount != 2 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestCurrentVersionInfoWithReader(t *testing.T) {
	info := CurrentVersionInfoWithReader(func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.24.2",
			Main: debug.Module{
				Path:    "example.com/custom",
				Version: "v1.2.3",
			},
			Settings: []debug.BuildSetting{
				{Key: "GOOS", Value: "linux"},
				{Key: "GOARCH", Value: "arm64"},
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	})

	if info.ModulePath != "example.com/custom" {
		t.Fatalf("module path = %q, want %q", info.ModulePath, "example.com/custom")
	}
	if info.Version != "v1.2.3" {
		t.Fatalf("version = %q, want %q", info.Version, "v1.2.3")
	}
	if info.GOOS != "linux" || info.GOARCH != "arm64" {
		t.Fatalf("unexpected target platform: %s/%s", info.GOOS, info.GOARCH)
	}
	if info.Commit != "abc123" || info.CommitTime == "" || !info.Modified {
		t.Fatalf("unexpected vcs info: %#v", info)
	}
}
