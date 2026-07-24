package reindexsvc

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func runTest(req RunRequest) (*RunResult, error) {
	rt, err := vaultruntime.New(req.VaultPath, vaultruntime.Options{})
	if err != nil {
		var setupErr *vaultruntime.SetupError
		if errors.As(err, &setupErr) {
			switch setupErr.Stage {
			case vaultruntime.StageConfig:
				return nil, newError(CodeConfigInvalid, setupErr.Error(), "Fix raven.yaml and try again", err)
			case vaultruntime.StageSchema:
				return nil, newError(CodeSchemaInvalid, setupErr.Error(), "Run 'rvn init' to create a schema", err)
			}
		}
		return nil, newError(CodeInvalidInput, "vault path is required", "", err)
	}
	defer rt.Close()
	return Run(rt, req)
}

func assertReindexCode(t *testing.T, err error, want Code) *svcerr.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := svcerr.AsError(err)
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
	_, err := runTest(RunRequest{VaultPath: "   "})
	assertReindexCode(t, err, CodeInvalidInput)
}

func TestRunSchemaInvalid(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: [\n"), 0o644); err != nil {
		t.Fatalf("failed to write malformed schema fixture: %v", err)
	}

	_, err := runTest(RunRequest{VaultPath: vaultPath})
	assertReindexCode(t, err, CodeSchemaInvalid)
}

func TestRunConfigInvalid(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("directories: [\n"), 0o644); err != nil {
		t.Fatalf("failed to write malformed raven.yaml fixture: %v", err)
	}

	_, err := runTest(RunRequest{VaultPath: vaultPath})
	assertReindexCode(t, err, CodeConfigInvalid)
}

func TestRunDryRunIndexesDiscoveredFiles(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("failed to write markdown fixture: %v", err)
	}

	result, err := runTest(RunRequest{
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

func TestRunIncrementalSkipsParsingUnchangedMarkdown(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	notePath := filepath.Join(vaultPath, "note.md")
	if err := os.WriteFile(notePath, []byte("# Original\n"), 0o644); err != nil {
		t.Fatalf("failed to write markdown fixture: %v", err)
	}
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run returned error: %v", err)
	}
	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatalf("failed to stat indexed markdown fixture: %v", err)
	}

	// Malformed content would produce a parse error if the incremental walk
	// read the file. Preserve its indexed mtime to exercise the pre-parse gate.
	if err := os.WriteFile(notePath, []byte("---\ntype: [invalid yaml\n---\n"), 0o644); err != nil {
		t.Fatalf("failed to replace markdown fixture: %v", err)
	}
	if err := os.Chtimes(notePath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("failed to restore markdown fixture mtime: %v", err)
	}

	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental Run returned error: %v", err)
	}
	if result.FilesIndexed != 0 || result.FilesSkipped != 1 {
		t.Fatalf("indexed/skipped = %d/%d, want 0/1", result.FilesIndexed, result.FilesSkipped)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v, want none because unchanged file must not be parsed", result.Errors)
	}
	if len(result.StaleFiles) != 0 {
		t.Fatalf("stale files = %#v, want none", result.StaleFiles)
	}
}

func TestRunIncrementalForcesJournaledPathWithUnchangedMtime(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	notePath := filepath.Join(vaultPath, "note.md")
	if err := os.WriteFile(notePath, []byte("# Original\n"), 0o644); err != nil {
		t.Fatalf("write markdown fixture: %v", err)
	}
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run: %v", err)
	}
	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatalf("stat indexed markdown: %v", err)
	}
	if _, err := indexjournal.SetPaths(vaultPath, "", []string{"note.md"}); err != nil {
		t.Fatalf("record pending path: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Updated\n"), 0o644); err != nil {
		t.Fatalf("update markdown fixture: %v", err)
	}
	if err := os.Chtimes(notePath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("restore indexed markdown mtime: %v", err)
	}

	dryRun, err := runTest(RunRequest{VaultPath: vaultPath, DryRun: true})
	if err != nil {
		t.Fatalf("incremental dry Run: %v", err)
	}
	if dryRun.FilesIndexed != 1 {
		t.Fatalf("dry-run indexed = %d, want 1", dryRun.FilesIndexed)
	}
	if pending, err := indexjournal.Load(vaultPath); err != nil {
		t.Fatalf("load journal after dry-run: %v", err)
	} else if !pending.Dirty() {
		t.Fatal("dry-run cleared pending journal")
	}

	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental Run: %v", err)
	}
	if result.FilesIndexed != 1 || result.FilesSkipped != 0 {
		t.Fatalf("indexed/skipped = %d/%d, want 1/0", result.FilesIndexed, result.FilesSkipped)
	}
	if pending, err := indexjournal.Load(vaultPath); err != nil {
		t.Fatalf("load index journal: %v", err)
	} else if pending.Dirty() {
		t.Fatalf("journal remains dirty after reindex: %#v", pending)
	}
}

func TestRunIncrementalRecoversUnknownInterruptedOperation(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	notePath := filepath.Join(vaultPath, "note.md")
	if err := os.WriteFile(notePath, []byte("# Original\n"), 0o644); err != nil {
		t.Fatalf("write markdown fixture: %v", err)
	}
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run: %v", err)
	}
	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatalf("stat indexed markdown: %v", err)
	}
	operationID, err := indexjournal.Begin(vaultPath)
	if err != nil {
		t.Fatalf("begin write-ahead operation: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Updated after guard\n"), 0o644); err != nil {
		t.Fatalf("update markdown fixture: %v", err)
	}
	if err := os.Chtimes(notePath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("restore indexed markdown mtime: %v", err)
	}
	if err := indexjournal.Abandon(vaultPath, operationID); err != nil {
		t.Fatalf("simulate interrupted process: %v", err)
	}

	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental recovery Run: %v", err)
	}
	if result.FilesIndexed != 1 || result.FilesSkipped != 0 {
		t.Fatalf("indexed/skipped = %d/%d, want 1/0", result.FilesIndexed, result.FilesSkipped)
	}
	if pending, err := indexjournal.Load(vaultPath); err != nil {
		t.Fatalf("load index journal: %v", err)
	} else if pending.Dirty() {
		t.Fatalf("unknown journal remains after successful recovery: %#v", pending)
	}
}

func TestRunIncrementalSucceedsWhileSharedIndexIsOpen(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "existing.md", "# Existing\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run returned error: %v", err)
	}

	holder, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open shared index holder: %v", err)
	}
	defer holder.Close()

	writeTestFile(t, vaultPath, "added.md", "# Added\n")
	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental Run with shared holder returned error: %v", err)
	}
	if !result.Incremental || result.FilesIndexed != 1 {
		t.Fatalf("unexpected incremental result: %#v", result)
	}
	if obj, err := holder.GetObject("added"); err != nil || obj == nil {
		t.Fatalf("shared holder did not observe added object: object=%#v err=%v", obj, err)
	}
}

func TestRunFullFailsClearlyWhileSharedIndexIsOpen(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "note.md", "# Note\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run returned error: %v", err)
	}

	holder, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open shared index holder: %v", err)
	}
	defer holder.Close()

	_, err = runTest(RunRequest{VaultPath: vaultPath, Full: true})
	svcErr := assertReindexCode(t, err, CodeDatabaseError)
	if !errors.Is(svcErr.Err, index.ErrIndexLocked) {
		t.Fatalf("underlying error = %v, want ErrIndexLocked", svcErr.Err)
	}
	if !strings.Contains(svcErr.Suggestion, "rvn lsp") || !strings.Contains(svcErr.Suggestion, "stop") {
		t.Fatalf("lock suggestion = %q, want LSP stop/wait guidance", svcErr.Suggestion)
	}
}

func TestRunSchemaRebuildFailsWhileSharedIndexIsOpen(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "note.md", "# Note\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run returned error: %v", err)
	}

	holder, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open shared index holder: %v", err)
	}
	defer holder.Close()
	downgradeIndexVersion(t, vaultPath)

	_, err = runTest(RunRequest{VaultPath: vaultPath})
	svcErr := assertReindexCode(t, err, CodeDatabaseError)
	if !errors.Is(svcErr.Err, index.ErrIndexLocked) {
		t.Fatalf("underlying error = %v, want ErrIndexLocked", svcErr.Err)
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

	fullResult, err := runTest(RunRequest{
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

	result, err := runTest(RunRequest{
		VaultPath: vaultPath,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry-run Run returned error: %v", err)
	}
	if result.FilesIndexed != 1 {
		t.Fatalf("files indexed = %d, want 1", result.FilesIndexed)
	}
	if result.Objects != fullResult.Objects+1 {
		t.Fatalf("objects = %d, want %d", result.Objects, fullResult.Objects+1)
	}
	if result.Traits != 1 {
		t.Fatalf("traits = %d, want 1", result.Traits)
	}
	if result.References != 1 {
		t.Fatalf("references = %d, want 1", result.References)
	}

	data := result.Data()
	if objects, ok := data["objects"].(int); !ok || objects != fullResult.Objects+1 {
		t.Fatalf("result data has unexpected objects: %#v", data["objects"])
	}
	if traits, ok := data["traits"].(int); !ok || traits != 1 {
		t.Fatalf("result data has unexpected traits: %#v", data["traits"])
	}
	if refs, ok := data["references"].(int); !ok || refs != 1 {
		t.Fatalf("result data has unexpected references: %#v", data["references"])
	}
}

func TestRunDryRunDoesNotCleanTrashRows(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "note.md", "# Note\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES ('trash/old', '.trash/old.md', 'page', '{}', 1)
	`); err != nil {
		_ = db.Close()
		t.Fatalf("seed trash row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded index: %v", err)
	}

	if _, err := runTest(RunRequest{VaultPath: vaultPath, DryRun: true}); err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}

	db, err = index.Open(vaultPath)
	if err != nil {
		t.Fatalf("reopen index: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM objects WHERE file_path = '.trash/old.md'`).Scan(&count); err != nil {
		t.Fatalf("count trash rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("trash row count after dry-run = %d, want 1", count)
	}
}

func TestRunVersionMismatchCompletesFullReindexBeforeReopening(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "note.md", "# Rebuild me\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}
	downgradeIndexVersion(t, vaultPath)

	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("Run after version mismatch returned error: %v", err)
	}
	if !result.SchemaRebuilt || result.Incremental {
		t.Fatalf("expected schema rebuild in full mode, got %#v", result)
	}
	if result.FilesIndexed != 1 || result.Objects != 1 {
		t.Fatalf("expected rebuilt note in index, got %#v", result)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("Open after completed rebuild: %v", err)
	}
	defer db.Close()
	stats, err := db.Stats()
	if err != nil {
		t.Fatalf("Stats after completed rebuild: %v", err)
	}
	if stats.ObjectCount != 1 {
		t.Fatalf("object count after completed rebuild = %d, want 1", stats.ObjectCount)
	}
}

func TestRunVersionMismatchFailureLeavesIndexUnavailable(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "note.md", "# Initially valid\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}
	writeTestFile(t, vaultPath, "note.md", "---\ntype: [\n---\n")
	downgradeIndexVersion(t, vaultPath)

	if _, err := runTest(RunRequest{VaultPath: vaultPath}); err == nil {
		t.Fatal("expected failed full rebuild")
	} else {
		assertReindexCode(t, err, CodeFileReadError)
	}
	if _, err := index.Open(vaultPath); !errors.Is(err, index.ErrIndexRebuildRequired) {
		t.Fatalf("Open after failed rebuild error = %v, want ErrIndexRebuildRequired", err)
	}

	writeTestFile(t, vaultPath, "note.md", "# Repaired\n")
	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("retry Run returned error: %v", err)
	}
	if !result.SchemaRebuilt || result.Objects != 1 {
		t.Fatalf("unexpected retry result: %#v", result)
	}
}

func TestRunDryRunDoesNotPublishVersionMismatchWipe(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "note.md", "# Keep the old index\n")
	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}
	downgradeIndexVersion(t, vaultPath)

	result, err := runTest(RunRequest{VaultPath: vaultPath, DryRun: true})
	if err != nil {
		t.Fatalf("dry-run after version mismatch returned error: %v", err)
	}
	if !result.SchemaRebuilt || result.Incremental {
		t.Fatalf("expected dry-run to plan a full schema rebuild, got %#v", result)
	}

	if _, err := index.Open(vaultPath); !errors.Is(err, index.ErrIndexRebuildRequired) {
		t.Fatalf("Open after dry-run error = %v, want ErrIndexRebuildRequired", err)
	}

	rawDB, err := sql.Open("sqlite", filepath.Join(vaultPath, ".raven", "index.db"))
	if err != nil {
		t.Fatalf("open raw index after dry-run: %v", err)
	}
	defer rawDB.Close()
	var version string
	if err := rawDB.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&version); err != nil {
		t.Fatalf("read raw index version after dry-run: %v", err)
	}
	if version != strconv.Itoa(index.CurrentDBVersion-1) {
		t.Fatalf("index version after dry-run = %q, want stale version %d", version, index.CurrentDBVersion-1)
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

	result, err := runTest(RunRequest{
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

func TestRunHealsUnresolvedRefsWhenNoFilesAreStale(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "source.md"), []byte("# Source\n\nSee [[target]].\n"), 0o644); err != nil {
		t.Fatalf("failed to write source fixture: %v", err)
	}

	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial full Run returned error: %v", err)
	}

	// Create the missing target and index it the way legacy/stale indexes got
	// into this state: the target row is fresh in the index, but the pending
	// ref in source.md was never re-resolved.
	targetPath := filepath.Join(vaultPath, "target.md")
	if err := os.WriteFile(targetPath, []byte("# Target\n"), 0o644); err != nil {
		t.Fatalf("failed to write target fixture: %v", err)
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat target fixture: %v", err)
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("failed to load schema: %v", err)
	}
	doc, err := parser.ParseDocumentWithOptions("# Target\n", targetPath, vaultPath, nil)
	if err != nil {
		t.Fatalf("failed to parse target fixture: %v", err)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to open index: %v", err)
	}
	db.SetAutoResolveRefs(false)
	if err := db.IndexDocumentWithMtime(doc, sch, targetInfo.ModTime().Unix()); err != nil {
		_ = db.Close()
		t.Fatalf("failed to index target fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close index: %v", err)
	}

	// Incremental reindex with zero stale files must still run the resolve
	// pass and heal the pending ref.
	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental Run returned error: %v", err)
	}
	if result.FilesIndexed != 0 {
		t.Fatalf("files indexed = %d, want 0 (test must exercise the no-stale-files path)", result.FilesIndexed)
	}
	if !result.HasRefResult {
		t.Fatalf("expected reference resolution result on incremental run, got %#v", result)
	}
	if result.RefsResolved < 1 {
		t.Fatalf("refs resolved = %d, want >= 1", result.RefsResolved)
	}

	db, err = index.Open(vaultPath)
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

	result, err := runTest(RunRequest{
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

	assets, err := db.QueryAssets()
	if err != nil {
		t.Fatalf("QueryAssets returned error: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != "assets/pdfs/paper.pdf" {
		t.Fatalf("assets = %#v, want paper asset", assets)
	}
	if assets[0].Extension != "pdf" || assets[0].MediaType != "application/pdf" {
		t.Fatalf("asset metadata = %#v, want pdf extension and media type", assets[0])
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

func TestRunIncrementalReindexesAssetsAfterConfigChange(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "assets/raw/logo.svg", "<svg></svg>\n")

	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}

	writeTestFile(t, vaultPath, "raven.yaml", "directories:\n  assets: assets/\n")

	result, err := runTest(RunRequest{VaultPath: vaultPath})
	if err != nil {
		t.Fatalf("incremental Run returned error: %v", err)
	}
	if result.Assets != 1 {
		t.Fatalf("assets = %d, want 1", result.Assets)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("failed to reopen index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assets, err := db.QueryAssets()
	if err != nil {
		t.Fatalf("QueryAssets returned error: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets = %#v, want logo.svg", assets)
	}
	if assets[0].ID != "assets/raw/logo.svg" || assets[0].Extension != "svg" {
		t.Fatalf("asset metadata = %#v, want svg asset", assets[0])
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

	result, err := runTest(RunRequest{VaultPath: vaultPath, Full: true})
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

	if _, err := runTest(RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("initial Run returned error: %v", err)
	}
	writeTestFile(t, vaultPath, "raven.yaml", "exclude:\n  - AGENTS.md\n")

	result, err := runTest(RunRequest{VaultPath: vaultPath})
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

func downgradeIndexVersion(t *testing.T, vaultPath string) {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(vaultPath, ".raven", "index.db"))
	if err != nil {
		t.Fatalf("open index to downgrade version: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE meta SET value = ? WHERE key = 'version'`,
		strconv.Itoa(index.CurrentDBVersion-1),
	); err != nil {
		_ = db.Close()
		t.Fatalf("downgrade index version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close downgraded index: %v", err)
	}
}
