package datesvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestDateHub_BacklinksUseDailyNoteObjectID(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-02-01.md", `# February 1, 2026`).
		WithFile("planning.md", `See [[2026-02-01]] for the daily note.`).
		Build()

	vault.RunCLI("reindex").MustSucceed(t)

	result, err := DateHub(DateHubRequest{
		VaultPath: vault.Path,
		DateArg:   "2026-02-01",
	})
	if err != nil {
		t.Fatalf("DateHub returned error: %v", err)
	}
	if got, want := result.DailyNoteID, "daily/2026-02-01"; got != want {
		t.Fatalf("daily note id = %q, want %q", got, want)
	}
	if len(result.Backlinks) != 1 {
		t.Fatalf("backlinks = %#v, want 1 backlink", result.Backlinks)
	}
	if got, want := result.Backlinks[0].SourceID, "planning"; got != want {
		t.Fatalf("backlink source id = %q, want %q", got, want)
	}
	if got, want := result.Backlinks[0].TargetRaw, "2026-02-01"; got != want {
		t.Fatalf("backlink target raw = %q, want %q", got, want)
	}
}
