package schemasvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func schemaTestRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{})
}

func TestAddTrait_TrimsValuesAndCoercesBooleanDefault(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	rt := schemaTestRuntime(t, vaultPath)
	_, err := AddTrait(rt, AddTraitRequest{
		VaultPath: vaultPath,
		TraitName: "priority",
		TraitType: "boolean",
		Values:    "low, medium, ,high",
		Default:   "true",
	})
	if err != nil {
		t.Fatalf("AddTrait failed: %v", err)
	}
	if _, ok := rt.Schema.Traits["priority"]; !ok {
		t.Fatal("runtime schema was not refreshed after AddTrait")
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	trait, ok := loaded.Traits["priority"]
	if !ok {
		t.Fatalf("expected trait priority to exist")
	}

	if got, want := trait.Type, schema.FieldType("boolean"); got != want {
		t.Fatalf("trait type = %q, want %q", got, want)
	}

	wantValues := []string{"low", "medium", "high"}
	if len(trait.Values) != len(wantValues) {
		t.Fatalf("trait values len = %d, want %d (%v)", len(trait.Values), len(wantValues), trait.Values)
	}
	for i := range wantValues {
		if trait.Values[i] != wantValues[i] {
			t.Fatalf("trait value[%d] = %q, want %q", i, trait.Values[i], wantValues[i])
		}
	}

	if got, ok := trait.Default.(bool); !ok || !got {
		t.Fatalf("trait default = %#v, want bool(true)", trait.Default)
	}
}

func TestAddTrait_PreservesStringDefaultForNonBooleanTypes(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	_, err := AddTrait(schemaTestRuntime(t, vaultPath), AddTraitRequest{
		VaultPath: vaultPath,
		TraitName: "status",
		TraitType: "enum",
		Values:    "todo,doing,done",
		Default:   "doing",
	})
	if err != nil {
		t.Fatalf("AddTrait failed: %v", err)
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	trait, ok := loaded.Traits["status"]
	if !ok {
		t.Fatalf("expected trait status to exist")
	}
	if got, ok := trait.Default.(string); !ok || got != "doing" {
		t.Fatalf("trait default = %#v, want string(\"doing\")", trait.Default)
	}
}
