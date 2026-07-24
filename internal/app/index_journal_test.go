package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestBeginIndexJournalOperationWritesAheadOfDispatch(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).Build()
	req, result, ok := beginIndexJournalOperation(context.Background(), commandexec.Request{
		CommandID: "edit",
		VaultPath: vault.Path,
	})
	if !ok || result.Error != nil {
		t.Fatalf("begin result = %#v, ok=%v", result, ok)
	}
	if req.IndexJournalOperation == "" {
		t.Fatal("journal operation ID is empty")
	}
	t.Cleanup(func() {
		_ = indexjournal.CancelUnknown(vault.Path, req.IndexJournalOperation)
	})
	snapshot, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load index journal: %v", err)
	}
	if !snapshot.Dirty() || !snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want unknown write-ahead operation", snapshot)
	}
}

func TestReadCommandWarnsWhenIndexJournalIsDirty(t *testing.T) {
	t.Parallel()

	vault := buildPhaseVault(t)
	if _, err := indexjournal.SetPaths(vault.Path, "", []string{"projects/roadmap.md"}); err != nil {
		t.Fatalf("record pending index path: %v", err)
	}

	result := runInvoked(t, vault.Path, "query", map[string]any{"query_string": "type:project"}, nil)
	if !result.OK {
		t.Fatalf("query failed: %#v", result.Error)
	}
	for _, warning := range result.Warnings {
		if warning.Code == codes.WarnDatabaseOutdated {
			return
		}
	}
	t.Fatalf("warnings = %#v, want %q", result.Warnings, codes.WarnDatabaseOutdated)
}

func TestBeginIndexJournalOperationFailsClosed(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, ".raven"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocking .raven file: %v", err)
	}
	_, result, ok := beginIndexJournalOperation(context.Background(), commandexec.Request{
		CommandID: "edit",
		VaultPath: vaultPath,
	})
	if ok || result.Error == nil || result.Error.Code != codes.ErrDatabase {
		t.Fatalf("result = %#v, ok=%v; want DATABASE_ERROR rejection", result, ok)
	}
}

func TestFailedMutationPreservesRecoverableUnknownGuard(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).Build()
	req, result, ok := beginIndexJournalOperation(context.Background(), commandexec.Request{
		CommandID: "edit",
		VaultPath: vault.Path,
	})
	if !ok || result.Error != nil {
		t.Fatalf("begin result = %#v, ok=%v", result, ok)
	}

	failure := annotateMutationPhase(context.Background(), req, commandexec.Failure("TEST_FAILURE", "failed", nil, ""))
	if failure.OK {
		t.Fatal("failed result became successful")
	}
	snapshot, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load index journal: %v", err)
	}
	if !snapshot.Dirty() || !snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want recoverable unknown guard", snapshot)
	}
	if err := indexjournal.CompleteRecoveredUnknown(vault.Path, snapshot); err != nil {
		t.Fatalf("complete abandoned guard: %v", err)
	}
	if pending, err := indexjournal.Load(vault.Path); err != nil {
		t.Fatalf("reload index journal: %v", err)
	} else if pending.Dirty() {
		t.Fatalf("abandoned guard remains after recovery: %#v", pending)
	}
}

func TestAutoReindexDisabledMutationWarnsUntilReindex(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("auto_reindex: false\n").
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nOld body.\n").
		Build()
	reindexPhaseVault(t, vault.Path)

	edit := runInvoked(t, vault.Path, "edit", map[string]any{
		"reference": "projects/roadmap", "old_str": "Old body.", "new_str": "New body.",
	}, nil)
	if !edit.OK {
		t.Fatalf("edit failed: %#v", edit.Error)
	}
	pending, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load index journal: %v", err)
	}
	if got := pending.Paths(); len(got) != 1 || got[0] != "projects/roadmap.md" {
		t.Fatalf("pending paths = %#v, want projects/roadmap.md", got)
	}

	query := runInvoked(t, vault.Path, "query", map[string]any{"query_string": "type:project"}, nil)
	if !hasWarning(query, codes.WarnDatabaseOutdated) {
		t.Fatalf("query warnings = %#v, want %q", query.Warnings, codes.WarnDatabaseOutdated)
	}

	reindex := runInvoked(t, vault.Path, "reindex", map[string]any{}, nil)
	if !reindex.OK {
		t.Fatalf("reindex failed: %#v", reindex.Error)
	}
	if pending, err := indexjournal.Load(vault.Path); err != nil {
		t.Fatalf("reload index journal: %v", err)
	} else if pending.Dirty() {
		t.Fatalf("journal remains dirty after reindex: %#v", pending)
	}
}

func hasWarning(result commandexec.Result, code codes.WarningCode) bool {
	for _, warning := range result.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
