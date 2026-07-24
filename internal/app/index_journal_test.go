package app

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestBeginIndexJournalOperationWritesAheadOfDispatch(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).Build()
	req, warnings := beginIndexJournalOperation(context.Background(), commandexec.Request{
		CommandID: "edit",
		VaultPath: vault.Path,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
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
