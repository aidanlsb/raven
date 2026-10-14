package reindexsvc

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
)

const modeTestSchema = `version: 1
types: {}
traits:
  todo:
    type: enum
    values: [todo, done]
`

type modeWant struct {
	incremental     bool
	dryRun          bool
	filesIndexed    int
	filesSkipped    int
	filesDeleted    int
	filesExcluded   int
	objects         int
	traits          int
	references      int
	stale           []string
	deleted         []string
	excluded        []string
	hasRefResult    bool
	journalDirty    bool
	trashCount      int
	indexed         []string
	warningContains []string
	errorsLen       int
	wantErr         codes.ErrorCode
	errContains     string
	suggestion      string
	openErr         error
}

func TestRunFullMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   modeWant
	}{
		{
			name:   "rebuilds remaining files and drops missing or excluded ones",
			mutate: applyStandardModeMutations,
			want: modeWant{
				filesIndexed: 4,
				objects:      4,
				traits:       1,
				references:   1,
				hasRefResult: true,
				indexed:      []string{"added.md", "keep.md", "source.md", "stale.md"},
			},
		},
		{
			name:   "clears a dirty journal after a successful rebuild",
			mutate: applyStandardModeMutationsWithJournal,
			want: modeWant{
				filesIndexed: 4,
				objects:      4,
				traits:       1,
				references:   1,
				hasRefResult: true,
				indexed:      []string{"added.md", "keep.md", "source.md", "stale.md"},
			},
		},
		{
			name:   "fails the whole rebuild when a file cannot be parsed",
			mutate: applyStandardModeMutationsWithBrokenFile,
			want: modeWant{
				wantErr:     codes.ErrFileRead,
				errContains: "file(s) failed during full reindex",
				suggestion:  "Fix invalid or unreadable vault files and run 'rvn reindex --full' again",
				openErr:     index.ErrIndexRebuildRequired,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := setupModeVault(t)
			tt.mutate(t, vaultPath)
			result, err := runTest(vaultPath, RunRequest{Full: true})
			assertModeResult(t, vaultPath, result, err, tt.want)
		})
	}
}

func TestRunIncrementalMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   modeWant
	}{
		{
			name:   "indexes stale and new files, skips unchanged, and reports deletions",
			mutate: applyStandardModeMutations,
			want: modeWant{
				incremental:   true,
				filesIndexed:  2,
				filesSkipped:  2,
				filesDeleted:  1,
				filesExcluded: 1,
				objects:       4,
				traits:        1,
				references:    1,
				stale:         []string{"added.md", "stale.md"},
				deleted:       []string{"gone.md"},
				excluded:      []string{"extra.md"},
				hasRefResult:  true,
				indexed:       []string{"added.md", "keep.md", "source.md", "stale.md"},
				warningContains: []string{
					"Cleaned up 1 files from .trash/ in index",
				},
			},
		},
		{
			name:   "reindexes a journaled path even when its mtime is unchanged",
			mutate: applyStandardModeMutationsWithJournal,
			want: modeWant{
				incremental:   true,
				filesIndexed:  3,
				filesSkipped:  1,
				filesDeleted:  1,
				filesExcluded: 1,
				objects:       4,
				traits:        1,
				references:    1,
				stale:         []string{"added.md", "keep.md", "stale.md"},
				deleted:       []string{"gone.md"},
				excluded:      []string{"extra.md"},
				hasRefResult:  true,
				indexed:       []string{"added.md", "keep.md", "source.md", "stale.md"},
			},
		},
		{
			name:   "records parse errors without failing the run",
			mutate: applyStandardModeMutationsWithBrokenFile,
			want: modeWant{
				incremental:   true,
				filesIndexed:  2,
				filesSkipped:  2,
				filesDeleted:  1,
				filesExcluded: 1,
				objects:       4,
				traits:        1,
				references:    1,
				stale:         []string{"added.md", "stale.md"},
				deleted:       []string{"gone.md"},
				excluded:      []string{"extra.md"},
				hasRefResult:  true,
				indexed:       []string{"added.md", "keep.md", "source.md", "stale.md"},
				errorsLen:     1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := setupModeVault(t)
			tt.mutate(t, vaultPath)
			result, err := runTest(vaultPath, RunRequest{})
			assertModeResult(t, vaultPath, result, err, tt.want)
		})
	}
}

func TestRunDryRunMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		full   bool
		mutate func(*testing.T, string)
		want   modeWant
	}{
		{
			name:   "incremental dry-run projects stats from deletions and reindexed files without writing",
			mutate: applyStandardModeMutations,
			want: modeWant{
				incremental:   true,
				dryRun:        true,
				filesIndexed:  2,
				filesSkipped:  2,
				filesDeleted:  2,
				filesExcluded: 2,
				objects:       4,
				traits:        1,
				references:    1,
				stale:         []string{"added.md", "stale.md"},
				deleted:       []string{".trash/old.md", "gone.md"},
				excluded:      []string{".trash/old.md", "extra.md"},
				trashCount:    1,
				indexed:       []string{".trash/old.md", "extra.md", "gone.md", "keep.md", "source.md", "stale.md"},
			},
		},
		{
			name:   "incremental dry-run leaves a dirty journal in place",
			mutate: applyStandardModeMutationsWithJournal,
			want: modeWant{
				incremental:   true,
				dryRun:        true,
				filesIndexed:  3,
				filesSkipped:  1,
				filesDeleted:  2,
				filesExcluded: 2,
				objects:       4,
				traits:        1,
				references:    1,
				stale:         []string{"added.md", "keep.md", "stale.md"},
				deleted:       []string{".trash/old.md", "gone.md"},
				excluded:      []string{".trash/old.md", "extra.md"},
				journalDirty:  true,
				trashCount:    1,
				indexed:       []string{".trash/old.md", "extra.md", "gone.md", "keep.md", "source.md", "stale.md"},
			},
		},
		{
			name:   "full dry-run reports walk stats and does not mutate the index",
			full:   true,
			mutate: applyStandardModeMutations,
			want: modeWant{
				dryRun:       true,
				filesIndexed: 4,
				objects:      4,
				traits:       1,
				references:   1,
				trashCount:   1,
				indexed:      []string{".trash/old.md", "extra.md", "gone.md", "keep.md", "source.md", "stale.md"},
			},
		},
		{
			name:   "full dry-run records parse errors without failing or wiping the index",
			full:   true,
			mutate: applyStandardModeMutationsWithBrokenFile,
			want: modeWant{
				dryRun:       true,
				filesIndexed: 4,
				objects:      4,
				traits:       1,
				references:   1,
				errorsLen:    1,
				trashCount:   1,
				indexed:      []string{".trash/old.md", "extra.md", "gone.md", "keep.md", "source.md", "stale.md"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := setupModeVault(t)
			tt.mutate(t, vaultPath)
			result, err := runTest(vaultPath, RunRequest{Full: tt.full, DryRun: true})
			assertModeResult(t, vaultPath, result, err, tt.want)
		})
	}
}

func setupModeVault(t *testing.T) string {
	t.Helper()
	vaultPath := t.TempDir()
	writeTestFile(t, vaultPath, "schema.yaml", modeTestSchema)
	writeTestFile(t, vaultPath, "keep.md", "# Keep\n")
	writeTestFile(t, vaultPath, "stale.md", "# Stale\n")
	writeTestFile(t, vaultPath, "gone.md", "# Gone\n")
	writeTestFile(t, vaultPath, "extra.md", "# Extra\n")
	writeTestFile(t, vaultPath, "source.md", "# Source\n")
	if _, err := runTest(vaultPath, RunRequest{Full: true}); err != nil {
		t.Fatalf("baseline full Run: %v", err)
	}
	seedTrashRow(t, vaultPath)
	return vaultPath
}

func applyStandardModeMutations(t *testing.T, vaultPath string) {
	t.Helper()
	writeTestFile(t, vaultPath, "stale.md", "# Stale\n\n- @todo(todo) See [[source]]\n")
	bumpMtime(t, filepath.Join(vaultPath, "stale.md"))
	if err := os.Remove(filepath.Join(vaultPath, "gone.md")); err != nil {
		t.Fatalf("remove gone.md: %v", err)
	}
	writeTestFile(t, vaultPath, "added.md", "# Added\n")
	writeTestFile(t, vaultPath, "raven.yaml", "exclude:\n  - extra.md\n")
}

func bumpMtime(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func applyStandardModeMutationsWithJournal(t *testing.T, vaultPath string) {
	t.Helper()
	keepPath := filepath.Join(vaultPath, "keep.md")
	info, err := os.Stat(keepPath)
	if err != nil {
		t.Fatalf("stat keep.md: %v", err)
	}
	applyStandardModeMutations(t, vaultPath)
	if _, err := indexjournal.SetPaths(vaultPath, "", []string{"keep.md"}); err != nil {
		t.Fatalf("record pending keep.md: %v", err)
	}
	if err := os.Chtimes(keepPath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("restore keep.md mtime: %v", err)
	}
}

func applyStandardModeMutationsWithBrokenFile(t *testing.T, vaultPath string) {
	t.Helper()
	applyStandardModeMutations(t, vaultPath)
	writeTestFile(t, vaultPath, "broken.md", "---\ntype: [\n---\n")
}

func seedTrashRow(t *testing.T, vaultPath string) {
	t.Helper()
	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index to seed trash: %v", err)
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
}

func assertModeResult(t *testing.T, vaultPath string, result *RunResult, err error, want modeWant) {
	t.Helper()
	if want.wantErr != "" {
		svcErr := assertReindexCode(t, err, want.wantErr)
		if want.errContains != "" && !strings.Contains(svcErr.Error(), want.errContains) {
			t.Fatalf("error %q, want substring %q", svcErr.Error(), want.errContains)
		}
		if want.suggestion != "" && svcErr.Suggestion != want.suggestion {
			t.Fatalf("suggestion = %q, want %q", svcErr.Suggestion, want.suggestion)
		}
		if want.openErr != nil {
			_, openErr := index.Open(vaultPath)
			if !errors.Is(openErr, want.openErr) {
				t.Fatalf("Open after failed run error = %v, want %v", openErr, want.openErr)
			}
		}
		return
	}
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Incremental != want.incremental || result.DryRun != want.dryRun {
		t.Fatalf("mode flags incremental=%v dry_run=%v, want incremental=%v dry_run=%v",
			result.Incremental, result.DryRun, want.incremental, want.dryRun)
	}
	if result.FilesIndexed != want.filesIndexed || result.FilesSkipped != want.filesSkipped {
		t.Fatalf("indexed/skipped = %d/%d, want %d/%d",
			result.FilesIndexed, result.FilesSkipped, want.filesIndexed, want.filesSkipped)
	}
	if result.FilesDeleted != want.filesDeleted || result.FilesExcluded != want.filesExcluded {
		t.Fatalf("deleted/excluded = %d/%d, want %d/%d",
			result.FilesDeleted, result.FilesExcluded, want.filesDeleted, want.filesExcluded)
	}
	if result.Objects != want.objects || result.Traits != want.traits || result.References != want.references {
		t.Fatalf("stats objects/traits/refs = %d/%d/%d, want %d/%d/%d",
			result.Objects, result.Traits, result.References, want.objects, want.traits, want.references)
	}
	assertStringSet(t, "stale files", result.StaleFiles, want.stale)
	assertStringSet(t, "deleted files", result.DeletedFiles, want.deleted)
	assertStringSet(t, "excluded files", result.ExcludedFiles, want.excluded)
	if result.HasRefResult != want.hasRefResult {
		t.Fatalf("HasRefResult = %v, want %v", result.HasRefResult, want.hasRefResult)
	}
	if len(result.Errors) != want.errorsLen {
		t.Fatalf("errors = %#v, want %d", result.Errors, want.errorsLen)
	}
	for _, fragment := range want.warningContains {
		if !containsWarning(result.WarningMessages, fragment) {
			t.Fatalf("warnings = %#v, want substring %q", result.WarningMessages, fragment)
		}
	}

	data := result.Data()
	if dryRun, ok := data["dry_run"].(bool); !ok || dryRun != want.dryRun {
		t.Fatalf("data dry_run = %#v, want %v", data["dry_run"], want.dryRun)
	}
	if incremental, ok := data["incremental"].(bool); !ok || incremental != want.incremental {
		t.Fatalf("data incremental = %#v, want %v", data["incremental"], want.incremental)
	}
	if _, ok := data["stale_files"]; ok != want.incremental {
		t.Fatalf("data stale_files presence = %v, want %v: %#v", ok, want.incremental, data)
	}
	if _, ok := data["refs_resolved"]; ok != want.hasRefResult {
		t.Fatalf("data refs_resolved presence = %v, want %v: %#v", ok, want.hasRefResult, data)
	}

	pending, journalErr := indexjournal.Load(vaultPath)
	if journalErr != nil {
		t.Fatalf("load journal: %v", journalErr)
	}
	if pending.Dirty() != want.journalDirty {
		t.Fatalf("journal dirty = %v (%#v), want %v", pending.Dirty(), pending, want.journalDirty)
	}
	if got := trashRowCount(t, vaultPath); got != want.trashCount {
		t.Fatalf("trash rows = %d, want %d", got, want.trashCount)
	}
	if want.indexed != nil {
		assertStringSet(t, "indexed paths", indexedFilePaths(t, vaultPath), want.indexed)
	}

	if want.errorsLen == 1 && !containsErrorPath(result.Errors, "broken.md") {
		t.Fatalf("errors = %#v, want broken.md", result.Errors)
	}
}

func assertStringSet(t *testing.T, label string, got, want []string) {
	t.Helper()
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	slices.Sort(gotCopy)
	slices.Sort(wantCopy)
	if !slices.Equal(gotCopy, wantCopy) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func containsWarning(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}

func containsErrorPath(errs []string, relPath string) bool {
	for _, message := range errs {
		if strings.HasPrefix(message, relPath+":") {
			return true
		}
	}
	return false
}

func indexedFilePaths(t *testing.T, vaultPath string) []string {
	t.Helper()
	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()
	paths, err := db.AllIndexedFilePaths()
	if err != nil {
		t.Fatalf("AllIndexedFilePaths: %v", err)
	}
	return paths
}

func trashRowCount(t *testing.T, vaultPath string) int {
	t.Helper()
	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM objects WHERE file_path = '.trash/old.md'`).Scan(&count); err != nil {
		t.Fatalf("count trash rows: %v", err)
	}
	return count
}
