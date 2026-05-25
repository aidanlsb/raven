package reindexsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
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

func TestRunDryRunProjectsIndexStats(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(`version: 1
types: {}
traits:
  todo:
    type: enum
    values: [todo, done]
`), 0o644); err != nil {
		t.Fatalf("failed to write schema fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "source.md"), []byte("# Source\n"), 0o644); err != nil {
		t.Fatalf("failed to write source fixture: %v", err)
	}

	fullResult, err := Run(RunRequest{
		VaultPath: vaultPath,
		Full:      true,
	})
	if err != nil {
		t.Fatalf("full Run returned error: %v", err)
	}
	if fullResult.Objects == 0 || fullResult.Traits != 0 || fullResult.References != 0 {
		t.Fatalf("unexpected baseline stats: %#v", fullResult)
	}

	if err := os.WriteFile(filepath.Join(vaultPath, "next.md"), []byte("# Next\n\n- @todo(todo) Link [[source]]\n"), 0o644); err != nil {
		t.Fatalf("failed to write next fixture: %v", err)
	}

	result, err := Run(RunRequest{
		VaultPath: vaultPath,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry-run Run returned error: %v", err)
	}
	if result.FilesIndexed != 1 {
		t.Fatalf("files indexed = %d, want 1", result.FilesIndexed)
	}
	if result.Objects != fullResult.Objects+2 {
		t.Fatalf("objects = %d, want %d", result.Objects, fullResult.Objects+2)
	}
	if result.Traits != 1 {
		t.Fatalf("traits = %d, want 1", result.Traits)
	}
	if result.References != 1 {
		t.Fatalf("references = %d, want 1", result.References)
	}

	data := result.Data()
	if objects, ok := data["objects"].(int); !ok || objects != fullResult.Objects+2 {
		t.Fatalf("result data has unexpected objects: %#v", data["objects"])
	}
	if traits, ok := data["traits"].(int); !ok || traits != 1 {
		t.Fatalf("result data has unexpected traits: %#v", data["traits"])
	}
	if refs, ok := data["references"].(int); !ok || refs != 1 {
		t.Fatalf("result data has unexpected references: %#v", data["references"])
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

func TestRunIndexesAssetsAndResolvesMarkdownAssetLinks(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultPath, "assets", "pdfs"), 0o755); err != nil {
		t.Fatalf("failed to create asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "assets", "pdfs", "paper.pdf"), []byte("%PDF test\n"), 0o644); err != nil {
		t.Fatalf("failed to write asset fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "note.md"), []byte("# Note\n\nRead [paper](assets/pdfs/paper.pdf).\n"), 0o644); err != nil {
		t.Fatalf("failed to write markdown fixture: %v", err)
	}

	result, err := Run(RunRequest{
		VaultPath: vaultPath,
		Full:      true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Assets != 1 {
		t.Fatalf("assets = %d, want 1", result.Assets)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to reopen index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assets, err := db.QueryAssets("pdf")
	if err != nil {
		t.Fatalf("QueryAssets returned error: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != "assets/pdfs/paper.pdf" {
		t.Fatalf("assets = %#v, want paper asset", assets)
	}

	var targetID string
	err = db.DB().QueryRow(`SELECT target_id FROM refs WHERE file_path = ?`, "note.md").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs table: %v", err)
	}
	if targetID != "assets/pdfs/paper.pdf" {
		t.Fatalf("target_id = %q, want assets/pdfs/paper.pdf", targetID)
	}
}

func TestRunSkipsExcludedMarkdownAndAssets(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "raven.yaml", "exclude:\n  - AGENTS.md\n  - .cursor/\n  - assets/generated/**\n")
	writeTestFile(t, vaultPath, "keep.md", "# Keep\n")
	writeTestFile(t, vaultPath, "AGENTS.md", "# Agents\n")
	writeTestFile(t, vaultPath, ".cursor/plans/work.plan.md", "# Plan\n")
	writeTestFile(t, vaultPath, "assets/pdfs/keep.pdf", "%PDF keep\n")
	writeTestFile(t, vaultPath, "assets/generated/drop.pdf", "%PDF drop\n")

	result, err := Run(RunRequest{VaultPath: vaultPath, Full: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.FilesIndexed != 2 {
		t.Fatalf("files indexed = %d, want 2", result.FilesIndexed)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to reopen index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	paths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths returned error: %v", err)
	}
	for _, excluded := range []string{"AGENTS.md", ".cursor/plans/work.plan.md", "assets/generated/drop.pdf"} {
		if containsString(paths, excluded) {
			t.Fatalf("indexed paths = %#v, did not expect excluded %s", paths, excluded)
		}
	}
	for _, included := range []string{"keep.md", "assets/pdfs/keep.pdf"} {
		if !containsString(paths, included) {
			t.Fatalf("indexed paths = %#v, expected %s", paths, included)
		}
	}
}

func TestRunIncrementalPurgesNewlyExcludedFiles(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "keep.md", "# Keep\n")
	writeTestFile(t, vaultPath, "AGENTS.md", "# Agents\n")

	if _, err := Run(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}
	writeTestFile(t, vaultPath, "raven.yaml", "exclude:\n  - AGENTS.md\n")

	result, err := Run(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental Run returned error: %v", err)
	}
	if result.FilesExcluded != 1 || len(result.ExcludedFiles) != 1 || result.ExcludedFiles[0] != "AGENTS.md" {
		t.Fatalf("excluded files = %d %#v, want AGENTS.md", result.FilesExcluded, result.ExcludedFiles)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to reopen index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	paths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths returned error: %v", err)
	}
	if containsString(paths, "AGENTS.md") {
		t.Fatalf("indexed paths = %#v, did not expect AGENTS.md", paths)
	}
	if !containsString(paths, "keep.md") {
		t.Fatalf("indexed paths = %#v, expected keep.md", paths)
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

func writeTestFile(t *testing.T, vaultPath, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
