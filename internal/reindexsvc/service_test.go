package reindexsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func assertReindexCode(t *testing.T, err error, want Code) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected reindexsvc error, got %T: %v", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %q, want %q", svcErr.Code, want)
	}
	return svcErr
}

func TestRunInvalidInput(t *testing.T) {
	_, err := Run(RunRequest{VaultPath: "   "})
	assertReindexCode(t, err, CodeInvalidInput)
}

func TestRunSchemaInvalid(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: [\n"), 0o644); err != nil {
		t.Fatalf("failed to write malformed schema fixture: %v", err)
	}

	_, err := Run(RunRequest{VaultPath: vaultPath})
	assertReindexCode(t, err, CodeSchemaInvalid)
}

func TestRunConfigInvalid(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("directories: [\n"), 0o644); err != nil {
		t.Fatalf("failed to write malformed raven.yaml fixture: %v", err)
	}

	_, err := Run(RunRequest{VaultPath: vaultPath})
	assertReindexCode(t, err, CodeConfigInvalid)
}

func TestRunDryRunIndexesDiscoveredFiles(t *testing.T) {
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

func TestBuildParseOptions(t *testing.T) {
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
