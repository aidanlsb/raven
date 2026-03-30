package checksvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestGroupMissingRefsForInteractive(t *testing.T) {
	t.Parallel()
	refs := []*check.MissingRef{
		{TargetPath: "zeta/path", Confidence: check.ConfidenceUnknown},
		{TargetPath: "beta/path", Confidence: check.ConfidenceCertain},
		{TargetPath: "alpha/path", Confidence: check.ConfidenceCertain},
		{TargetPath: "delta/path", Confidence: check.ConfidenceInferred},
		{TargetPath: "charlie/path", Confidence: check.ConfidenceInferred},
	}

	grouped := GroupMissingRefsForInteractive(refs)

	if len(grouped.Certain) != 2 || grouped.Certain[0].TargetPath != "alpha/path" || grouped.Certain[1].TargetPath != "beta/path" {
		t.Fatalf("unexpected certain group ordering: %#v", grouped.Certain)
	}
	if len(grouped.Inferred) != 2 || grouped.Inferred[0].TargetPath != "charlie/path" || grouped.Inferred[1].TargetPath != "delta/path" {
		t.Fatalf("unexpected inferred group ordering: %#v", grouped.Inferred)
	}
	if len(grouped.Unknown) != 1 || grouped.Unknown[0].TargetPath != "zeta/path" {
		t.Fatalf("unexpected unknown group ordering: %#v", grouped.Unknown)
	}
}

func TestResolveAndSlugifyTargetPathUsesDirectoryRoots(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"meeting": {DefaultPath: "meeting/"},
		},
	}

	got := ResolveAndSlugifyTargetPath("all hands", "meeting", sch, "objects/", "")
	want := "objects/meeting/all-hands"
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestAddTypeAndAddTraitUpdateSchemaAndInMemory(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	schemaContent := `version: 2
types:
  project:
    default_path: projects/
`
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaContent), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	if err := AddType(vaultPath, sch, "meeting", "meeting/"); err != nil {
		t.Fatalf("AddType failed: %v", err)
	}
	if _, ok := sch.Types["meeting"]; !ok {
		t.Fatalf("in-memory schema missing added type")
	}

	if err := AddTrait(vaultPath, sch, "done", "boolean", nil, "true"); err != nil {
		t.Fatalf("AddTrait failed: %v", err)
	}
	trait, ok := sch.Traits["done"]
	if !ok {
		t.Fatalf("in-memory schema missing added trait")
	}
	if trait.Default != true {
		t.Fatalf("expected in-memory trait default true, got %#v", trait.Default)
	}

	reloaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("reload schema: %v", err)
	}
	if _, ok := reloaded.Types["meeting"]; !ok {
		t.Fatalf("reloaded schema missing added type")
	}
	reloadedTrait, ok := reloaded.Traits["done"]
	if !ok {
		t.Fatalf("reloaded schema missing added trait")
	}
	if reloadedTrait.Default != true {
		t.Fatalf("expected reloaded trait default true, got %#v", reloadedTrait.Default)
	}
}
