package checksvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func reindexForTest(t *testing.T, vaultPath string) {
	t.Helper()
	if _, err := reindexsvc.Run(reindexsvc.RunRequest{VaultPath: vaultPath, Full: true}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
}

func loadForDetect(t *testing.T, vaultPath string) (*config.VaultConfig, *schema.Schema) {
	t.Helper()
	cfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	return cfg, sch
}

func TestDetectMissingRefs_CertainFromTypedField(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/website.md", "---\ntype: project\ntitle: Website\nowner: people/ghost\n---\n").
		Build()
	reindexForTest(t, vault.Path)

	cfg, sch := loadForDetect(t, vault.Path)
	refs, err := DetectMissingRefs(vault.Path, cfg, sch, "projects/website.md")
	if err != nil {
		t.Fatalf("DetectMissingRefs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %#v, want exactly one missing ref", refs)
	}
	got := refs[0]
	if got.TargetPath != "people/ghost" {
		t.Fatalf("target_path = %q, want people/ghost", got.TargetPath)
	}
	if got.InferredType != "person" {
		t.Fatalf("inferred_type = %q, want person", got.InferredType)
	}
	if got.Confidence != check.ConfidenceCertain {
		t.Fatalf("confidence = %v, want certain", got.Confidence)
	}
	if got.FieldSource != "owner" {
		t.Fatalf("field_source = %q, want owner", got.FieldSource)
	}
}

func TestDetectMissingRefs_ExistingTargetNotReported(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/website.md", "---\ntype: project\ntitle: Website\nowner: people/freya\n---\n").
		Build()
	reindexForTest(t, vault.Path)

	cfg, sch := loadForDetect(t, vault.Path)
	refs, err := DetectMissingRefs(vault.Path, cfg, sch, "projects/website.md")
	if err != nil {
		t.Fatalf("DetectMissingRefs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("refs = %#v, want none (target exists)", refs)
	}
}

func TestDetectMissingRefs_InferredFromPath(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("notes/mention.md", "See [[projects/ghost-project]] for details.\n").
		Build()
	reindexForTest(t, vault.Path)

	cfg, sch := loadForDetect(t, vault.Path)
	refs, err := DetectMissingRefs(vault.Path, cfg, sch, "notes/mention.md")
	if err != nil {
		t.Fatalf("DetectMissingRefs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %#v, want exactly one missing ref", refs)
	}
	got := refs[0]
	if got.TargetPath != "projects/ghost-project" {
		t.Fatalf("target_path = %q, want projects/ghost-project", got.TargetPath)
	}
	if got.InferredType != "project" {
		t.Fatalf("inferred_type = %q, want project", got.InferredType)
	}
	if got.Confidence != check.ConfidenceInferred {
		t.Fatalf("confidence = %v, want inferred", got.Confidence)
	}
}

func TestDetectMissingRefs_InferredDateFromDailyDirectory(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("directories:\n  daily: journal/\n").
		WithFile("notes/mention.md", "See [[journal/2026-06-30]] for details.\n").
		Build()
	reindexForTest(t, vault.Path)

	cfg, sch := loadForDetect(t, vault.Path)
	refs, err := DetectMissingRefs(vault.Path, cfg, sch, "notes/mention.md")
	if err != nil {
		t.Fatalf("DetectMissingRefs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %#v, want exactly one missing ref", refs)
	}
	got := refs[0]
	if got.TargetPath != "journal/2026-06-30" {
		t.Fatalf("target_path = %q, want journal/2026-06-30", got.TargetPath)
	}
	if got.InferredType != "date" {
		t.Fatalf("inferred_type = %q, want date", got.InferredType)
	}
	if got.Confidence != check.ConfidenceInferred {
		t.Fatalf("confidence = %v, want inferred", got.Confidence)
	}
}

func TestDetectMissingRefs_AmbiguousNotReported(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/freya.md", "---\ntype: project\ntitle: Freya\n---\n").
		WithFile("notes/mention.md", "See [[Freya]] for details.\n").
		Build()
	reindexForTest(t, vault.Path)

	cfg, sch := loadForDetect(t, vault.Path)
	refs, err := DetectMissingRefs(vault.Path, cfg, sch, "notes/mention.md")
	if err != nil {
		t.Fatalf("DetectMissingRefs: %v", err)
	}
	// An ambiguous reference resolves to multiple matches and is never tracked as
	// a missing (creatable) reference.
	if len(refs) != 0 {
		t.Fatalf("refs = %#v, want none (reference is ambiguous, not missing)", refs)
	}
}

func TestDetectMissingRefs_SkippedWhenNoIndex(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/website.md", "---\ntype: project\ntitle: Website\nowner: people/ghost\n---\n").
		Build()
	// Intentionally do not reindex: with no index, detection is skipped rather
	// than reporting false positives for unindexed targets.

	cfg, sch := loadForDetect(t, vault.Path)
	refs, err := DetectMissingRefs(vault.Path, cfg, sch, "projects/website.md")
	if err != nil {
		t.Fatalf("DetectMissingRefs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("refs = %#v, want none when index is unavailable", refs)
	}
}
