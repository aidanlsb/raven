package objectsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestPreviewAddBulk(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()

	vaultCfg, err := config.LoadVaultConfig(v.Path)
	if err != nil {
		t.Fatalf("load vault config: %v", err)
	}

	preview, err := PreviewAddBulk(AddBulkRequest{
		VaultPath:   v.Path,
		VaultConfig: vaultCfg,
		ObjectIDs:   []string{"people/alice", "people/missing"},
		Line:        "bulk note",
	})
	if err != nil {
		t.Fatalf("PreviewAddBulk() error = %v", err)
	}

	if preview.Total != 2 {
		t.Fatalf("preview.Total = %d, want 2", preview.Total)
	}
	if len(preview.Items) != 1 || preview.Items[0].ID != "people/alice" {
		t.Fatalf("preview.Items = %#v, want one item for people/alice", preview.Items)
	}
	if len(preview.Skipped) != 1 || preview.Skipped[0].ID != "people/missing" || preview.Skipped[0].Reason != "object not found" {
		t.Fatalf("preview.Skipped = %#v, want one skipped missing object", preview.Skipped)
	}
}

func TestApplyAddBulk(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()

	vaultCfg, err := config.LoadVaultConfig(v.Path)
	if err != nil {
		t.Fatalf("load vault config: %v", err)
	}

	var reindexed []string
	summary, err := ApplyAddBulk(AddBulkRequest{
		VaultPath:   v.Path,
		VaultConfig: vaultCfg,
		ObjectIDs:   []string{"people/alice", "people/missing"},
		Line:        "bulk apply line",
	}, func(filePath string) {
		reindexed = append(reindexed, filePath)
	})
	if err != nil {
		t.Fatalf("ApplyAddBulk() error = %v", err)
	}

	if summary.Added != 1 || summary.Skipped != 1 || summary.Errors != 0 {
		t.Fatalf("summary = %#v, want added=1 skipped=1 errors=0", summary)
	}
	if len(reindexed) != 1 || !strings.HasSuffix(reindexed[0], "people/alice.md") {
		t.Fatalf("reindex callbacks = %#v, want one callback for people/alice.md", reindexed)
	}
	content := v.ReadFile("people/alice.md")
	if !strings.Contains(content, "bulk apply line\n") {
		t.Fatalf("alice file missing appended line, content:\n%s", content)
	}
}

func TestApplyAddBulk_HeadingConflictWithEmbeddedID(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\n## Tasks\n").
		Build()

	vaultCfg, err := config.LoadVaultConfig(v.Path)
	if err != nil {
		t.Fatalf("load vault config: %v", err)
	}

	summary, err := ApplyAddBulk(AddBulkRequest{
		VaultPath:   v.Path,
		VaultConfig: vaultCfg,
		ObjectIDs:   []string{"projects/roadmap#tasks"},
		Line:        "line",
		HeadingSpec: "tasks",
	}, nil)
	if err != nil {
		t.Fatalf("ApplyAddBulk() error = %v", err)
	}

	if summary.Errors != 1 || len(summary.Results) != 1 {
		t.Fatalf("summary = %#v, want one error result", summary)
	}
	if got := summary.Results[0].Reason; got != "cannot combine --heading with embedded IDs from stdin" {
		t.Fatalf("reason = %q, want heading/embedded conflict", got)
	}
}
