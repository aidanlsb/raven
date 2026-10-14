package reindexsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestPlanFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		req              RunRequest
		mutate           func(*testing.T, string)
		wantDeleted      []string
		wantExcluded     []string
		wantDirty        []string
		wantForceFull    bool
		wantParse        []string
		wantSkip         []string
		wantTrashWarning bool
	}{
		{
			name:             "incremental reports deleted excluded and dirty paths",
			mutate:           applyStandardModeMutationsWithJournal,
			wantDeleted:      []string{"gone.md"},
			wantExcluded:     []string{"extra.md"},
			wantDirty:        []string{"keep.md"},
			wantParse:        []string{"added.md", "keep.md", "stale.md"},
			wantSkip:         []string{"source.md"},
			wantTrashWarning: true,
		},
		{
			name:         "incremental dry-run reports trash as deleted and excluded",
			req:          RunRequest{DryRun: true},
			mutate:       applyStandardModeMutations,
			wantDeleted:  []string{".trash/old.md", "gone.md"},
			wantExcluded: []string{".trash/old.md", "extra.md"},
			wantParse:    []string{"added.md", "stale.md"},
			wantSkip:     []string{"keep.md", "source.md"},
		},
		{
			name:         "full dry-run skips incremental recovery",
			req:          RunRequest{Full: true, DryRun: true},
			mutate:       applyStandardModeMutations,
			wantDeleted:  []string{},
			wantExcluded: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := setupModeVault(t)
			tt.mutate(t, vaultPath)
			plan := planTest(t, vaultPath, tt.req)

			assertStringSet(t, "deleted files", plan.deletedFiles, tt.wantDeleted)
			assertStringSet(t, "excluded files", plan.excludedFiles, tt.wantExcluded)
			assertStringSet(t, "dirty paths", plan.pending.Paths(), tt.wantDirty)
			if plan.forceFullScan != tt.wantForceFull {
				t.Fatalf("forceFullScan = %v, want %v", plan.forceFullScan, tt.wantForceFull)
			}
			if tt.wantTrashWarning && !containsWarning(plan.warningMessages, "Cleaned up 1 files from .trash/ in index") {
				t.Fatalf("warnings = %#v, want trash cleanup", plan.warningMessages)
			}
			if !tt.wantTrashWarning && containsWarning(plan.warningMessages, "Cleaned up") {
				t.Fatalf("warnings = %#v, did not want trash cleanup", plan.warningMessages)
			}

			for _, relPath := range tt.wantParse {
				assertShouldParse(t, vaultPath, plan, relPath, true)
			}
			for _, relPath := range tt.wantSkip {
				assertShouldParse(t, vaultPath, plan, relPath, false)
			}
		})
	}
}

func TestFilePlanShouldParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		plan  filePlan
		path  string
		mtime int64
		want  bool
	}{
		{name: "force full scan", plan: filePlan{forceFullScan: true}, path: "keep.md", mtime: 1, want: true},
		{name: "dirty path ignores mtime", plan: filePlan{dirtyPaths: map[string]struct{}{"keep.md": {}}, indexedMtimes: map[string]int64{"keep.md": 50}}, path: "keep.md", mtime: 50, want: true},
		{name: "newer mtime is stale", plan: filePlan{indexedMtimes: map[string]int64{"keep.md": 10}}, path: "keep.md", mtime: 11, want: true},
		{name: "equal mtime is skipped", plan: filePlan{indexedMtimes: map[string]int64{"keep.md": 10}}, path: "keep.md", mtime: 10, want: false},
		{name: "missing index mtime is stale", plan: filePlan{indexedMtimes: map[string]int64{}}, path: "added.md", mtime: 1, want: true},
		{name: "nil mtime map parses every file", plan: filePlan{}, path: "keep.md", mtime: 1, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.plan.shouldParse(tt.path, tt.mtime)
			if got != tt.want {
				t.Fatalf("shouldParse(%q, %d) = %v, want %v", tt.path, tt.mtime, got, tt.want)
			}
		})
	}
}

func planTest(t *testing.T, vaultPath string, req RunRequest) *filePlan {
	t.Helper()
	rt, err := vaultruntime.New(vaultPath, vaultruntime.Options{})
	if err != nil {
		t.Fatalf("vault runtime: %v", err)
	}
	t.Cleanup(rt.Close)
	prep, err := prepare(rt, req)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Cleanup(prep.Close)
	plan, err := planFiles(prep)
	if err != nil {
		t.Fatalf("planFiles: %v", err)
	}
	return plan
}

func assertShouldParse(t *testing.T, vaultPath string, plan *filePlan, relPath string, want bool) {
	t.Helper()
	info, err := os.Stat(filepath.Join(vaultPath, relPath))
	if err != nil {
		t.Fatalf("stat %s: %v", relPath, err)
	}
	got := plan.shouldParse(relPath, info.ModTime().Unix())
	if got != want {
		t.Fatalf("shouldParse(%s) = %v, want %v", relPath, got, want)
	}
}
