//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_AutoReindexDisabledJournalWarnsAndRecovers(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("auto_reindex: false\n").
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nOld body.\n").
		Build()
	vault.RunCLI("reindex", "--full").MustSucceed(t)

	vault.RunCLI("edit", "projects/roadmap", "Old body.", "New body.").MustSucceed(t)
	query := vault.RunCLI("query", "type:project").MustSucceed(t)
	query.AssertHasWarning(t, "DATABASE_OUTDATED")

	pending, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load index journal: %v", err)
	}
	if got := pending.Paths(); len(got) != 1 || got[0] != "projects/roadmap.md" {
		t.Fatalf("pending paths = %#v, want projects/roadmap.md", got)
	}

	vault.RunCLI("reindex").MustSucceed(t)
	if pending, err := indexjournal.Load(vault.Path); err != nil {
		t.Fatalf("reload index journal: %v", err)
	} else if pending.Dirty() {
		t.Fatalf("journal remains dirty after reindex: %#v", pending)
	}
}
