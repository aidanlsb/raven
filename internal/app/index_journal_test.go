package app

import (
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestReadCommandWarnsWhenIndexJournalIsDirtyWithoutRefresh(t *testing.T) {
	t.Parallel()

	vault := buildPhaseVault(t)
	if _, err := indexjournal.SetPaths(vault.Path, "", []string{"projects/roadmap.md"}); err != nil {
		t.Fatalf("record pending index path: %v", err)
	}

	result := runInvoked(t, vault.Path, "query", map[string]any{
		"query_string": "type:project",
		"no-refresh":   true,
	}, nil)
	if !result.OK {
		t.Fatalf("query failed: %#v", result.Error)
	}
	if !hasWarning(result, codes.WarnDatabaseOutdated) {
		t.Fatalf("warnings = %#v, want %q", result.Warnings, codes.WarnDatabaseOutdated)
	}
}

func TestReadCommandRefreshClearsIndexJournal(t *testing.T) {
	t.Parallel()

	vault := buildPhaseVault(t)
	if _, err := indexjournal.SetPaths(vault.Path, "", []string{"projects/roadmap.md"}); err != nil {
		t.Fatalf("record pending index path: %v", err)
	}

	result := runInvoked(t, vault.Path, "query", map[string]any{"query_string": "type:project"}, nil)
	if !result.OK {
		t.Fatalf("query failed: %#v", result.Error)
	}
	if hasWarning(result, codes.WarnDatabaseOutdated) {
		t.Fatalf("warnings = %#v, want journal consumed without %q", result.Warnings, codes.WarnDatabaseOutdated)
	}
	pending, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load index journal: %v", err)
	}
	if pending.Dirty() {
		t.Fatalf("journal remains dirty after auto-refresh: %#v", pending)
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
