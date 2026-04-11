package reindexsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/svcerror"
)

func assertReindexCode(t *testing.T, err error, want string) *svcerror.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := svcerror.As(err)
	if !ok {
		t.Fatalf("expected reindexsvc error, got %T: %v", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %q, want %q", svcErr.Code, want)
	}
	return svcErr
}

func TestRunInvalidInput(t *testing.T) {
	t.Parallel()
	_, err := Run(RunRequest{VaultPath: "   "})
	assertReindexCode(t, err, CodeInvalidInput)
}

func TestRunSchemaInvalid(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: [\n"), 0o644); err != nil {
		t.Fatalf("failed to write malformed schema fixture: %v", err)
	}

	_, err := Run(RunRequest{VaultPath: vaultPath})
	assertReindexCode(t, err, CodeSchemaInvalid)
}

func TestRunConfigInvalid(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("directories: [\n"), 0o644); err != nil {
		t.Fatalf("failed to write malformed raven.yaml fixture: %v", err)
	}

	_, err := Run(RunRequest{VaultPath: vaultPath})
	assertReindexCode(t, err, CodeConfigInvalid)
}

func TestRunDryRunIndexesDiscoveredFiles(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("failed to write markdown fixture: %v", err)
	}

	result, err := Run(RunRequest{
		VaultPath: vaultPath,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.DryRun || !result.Incremental {
		t.Fatalf("unexpected run mode flags: %#v", result)
	}
	if result.FilesIndexed != 1 {
		t.Fatalf("files indexed = %d, want 1", result.FilesIndexed)
	}
	if len(result.StaleFiles) != 1 || result.StaleFiles[0] != "note.md" {
		t.Fatalf("unexpected stale files: %#v", result.StaleFiles)
	}

	data := result.Data()
	if dryRun, ok := data["dry_run"].(bool); !ok || !dryRun {
		t.Fatalf("result data missing dry_run=true: %#v", data)
	}
	if filesIndexed, ok := data["files_indexed"].(int); !ok || filesIndexed != 1 {
		t.Fatalf("result data has unexpected files_indexed: %#v", data["files_indexed"])
	}
}

func TestRunResolvesReferencesAfterBulkReindex(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "source.md"), []byte("# Source\n\nSee [[target]].\n"), 0o644); err != nil {
		t.Fatalf("failed to write source fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "target.md"), []byte("# Target\n"), 0o644); err != nil {
		t.Fatalf("failed to write target fixture: %v", err)
	}

	result, err := Run(RunRequest{
		VaultPath: vaultPath,
		Full:      true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.HasRefResult {
		t.Fatalf("expected reference resolution result, got %#v", result)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to reopen index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var targetID string
	err = db.DB().QueryRow(`SELECT target_id FROM refs WHERE file_path = ?`, "source.md").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs table: %v", err)
	}
	if targetID != "target" {
		t.Fatalf("target_id = %q, want %q", targetID, "target")
	}
}

func TestBuildParseOptions(t *testing.T) {
	t.Parallel()
	if got := buildParseOptions(nil); got != nil {
		t.Fatalf("expected nil parse options for nil config, got %#v", got)
	}

	if got := buildParseOptions(&config.VaultConfig{}); got != nil {
		t.Fatalf("expected nil parse options without directories, got %#v", got)
	}

	cfg := &config.VaultConfig{
		Directories: &config.DirectoriesConfig{
			Object: "objects",
			Page:   "pages",
		},
	}
	got := buildParseOptions(cfg)
	if got == nil {
		t.Fatal("expected parse options when directories are configured")
	}
	if got.ObjectsRoot != "objects/" || got.PagesRoot != "pages/" {
		t.Fatalf("unexpected parse options roots: %#v", got)
	}
}
