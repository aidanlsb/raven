package checkfixsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
)

func loadTestSchema(t *testing.T, vaultPath, contents string) *schema.Schema {
	t.Helper()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	return sch
}

func TestApplyMissingRefResolutionsCreatesNewTypeThenPage(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	sch := loadTestSchema(t, vaultPath, "version: 2\ntypes:\n  project:\n    default_path: projects/\n")

	vaultCfg := &config.VaultConfig{}

	result := ApplyMissingRefResolutions(
		vaultPath,
		sch,
		[]NewTypeResolution{{TypeName: "meeting", DefaultPath: "meeting/"}},
		[]MissingRefResolution{{TargetPath: "all-hands", TypeName: "meeting"}},
		vaultCfg,
	)

	if len(result.Types) != 1 || result.Types[0].Err != nil {
		t.Fatalf("unexpected type outcomes: %#v", result.Types)
	}
	if len(result.Pages) != 1 || result.Pages[0].Err != nil {
		t.Fatalf("unexpected page outcomes: %#v", result.Pages)
	}
	if got, want := result.Pages[0].ResolvedPath, "meeting/all-hands"; got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
	if result.CreatedPages() != 1 {
		t.Fatalf("CreatedPages() = %d, want 1", result.CreatedPages())
	}

	if _, ok := sch.Types["meeting"]; !ok {
		t.Fatalf("expected in-memory schema to include new type 'meeting'")
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "meeting/all-hands.md")); err != nil {
		t.Fatalf("expected created page: %v", err)
	}
}

func TestApplyMissingRefResolutionsRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	sch := loadTestSchema(t, vaultPath, "version: 2\ntypes:\n  meeting:\n    default_path: meeting/\n")

	vaultCfg := &config.VaultConfig{
		Directories: &config.DirectoriesConfig{Object: "objects/"},
	}

	result := ApplyMissingRefResolutions(
		vaultPath,
		sch,
		nil,
		[]MissingRefResolution{{TargetPath: "retro", TypeName: "meeting"}},
		vaultCfg,
	)

	if len(result.Pages) != 1 || result.Pages[0].Err != nil {
		t.Fatalf("unexpected page outcomes: %#v", result.Pages)
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "objects/meeting/retro.md")); err != nil {
		t.Fatalf("expected page under objects root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "meeting/retro.md")); err == nil {
		t.Fatalf("did not expect page outside objects root")
	}
}

func TestApplyMissingRefResolutionsSkipsPageWhenTypeMissing(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	sch := loadTestSchema(t, vaultPath, "version: 2\ntypes:\n  project:\n    default_path: projects/\n")

	result := ApplyMissingRefResolutions(
		vaultPath,
		sch,
		nil,
		[]MissingRefResolution{{TargetPath: "ghost", TypeName: "does-not-exist"}},
		&config.VaultConfig{},
	)

	if len(result.Pages) != 0 {
		t.Fatalf("expected no page outcomes for unresolvable type, got %#v", result.Pages)
	}
}

func TestApplyTraitResolutionsAddsTraits(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	sch := loadTestSchema(t, vaultPath, "version: 2\ntypes: {}\n")

	outcomes := ApplyTraitResolutions(vaultPath, sch, []TraitResolution{
		{TraitName: "priority", TraitType: "enum", EnumValues: []string{"low", "high"}, DefaultValue: "low"},
		{TraitName: "done", TraitType: "boolean", DefaultValue: "true"},
	})

	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %#v, want 2", outcomes)
	}
	for _, outcome := range outcomes {
		if outcome.Err != nil {
			t.Fatalf("unexpected error adding trait %q: %v", outcome.TraitName, outcome.Err)
		}
	}
	if _, ok := sch.Traits["priority"]; !ok {
		t.Fatalf("expected in-memory schema to include trait 'priority'")
	}
	if _, ok := sch.Traits["done"]; !ok {
		t.Fatalf("expected in-memory schema to include trait 'done'")
	}
}
