package schemasvc

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestUpdateField_RejectsInvalidFieldSpecs(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()

	_, err := UpdateField(UpdateFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "project",
		FieldName: "status",
		FieldType: "ref",
	})
	if err == nil {
		t.Fatal("expected update field to reject ref without target")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected schemasvc error, got %T: %v", err, err)
	}
	if svcErr.Code != ErrorInvalidInput {
		t.Fatalf("error code = %q, want %q", svcErr.Code, ErrorInvalidInput)
	}
	if !strings.Contains(svcErr.Message, "require --target") {
		t.Fatalf("unexpected error message: %q", svcErr.Message)
	}
}

func TestUpdateField_NormalizesValuesAndClearsIncompatibleTarget(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()

	_, err := UpdateField(UpdateFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "project",
		FieldName: "status",
		Values:    "active, paused, ,done, archived",
	})
	if err != nil {
		t.Fatalf("UpdateField values returned error: %v", err)
	}

	loaded, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	status := loaded.Types["project"].Fields["status"]
	wantValues := []string{"active", "paused", "done", "archived"}
	if len(status.Values) != len(wantValues) {
		t.Fatalf("status values = %v, want %v", status.Values, wantValues)
	}
	for i := range wantValues {
		if status.Values[i] != wantValues[i] {
			t.Fatalf("status values = %v, want %v", status.Values, wantValues)
		}
	}

	_, err = UpdateField(UpdateFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "project",
		FieldName: "owner",
		FieldType: "string",
	})
	if err != nil {
		t.Fatalf("UpdateField type change returned error: %v", err)
	}

	loaded, err = schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("reload schema: %v", err)
	}
	owner := loaded.Types["project"].Fields["owner"]
	if owner.Type != schema.FieldTypeString {
		t.Fatalf("owner type = %q, want %q", owner.Type, schema.FieldTypeString)
	}
	if owner.Target != "" {
		t.Fatalf("owner target = %q, want empty", owner.Target)
	}
	if strings.Contains(vault.ReadFile("schema.yaml"), "target: person") {
		t.Fatalf("schema.yaml still contains ref target after changing owner to string")
	}
}

func TestUpdateTrait_NormalizesValuesAndBooleanDefaults(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(`version: 1
types: {}
traits:
  done:
    type: boolean
  priority:
    type: enum
    values: [low]
`).Build()

	_, err := UpdateTrait(UpdateTraitRequest{
		VaultPath: vault.Path,
		TraitName: "priority",
		Values:    "low, medium, ,high",
	})
	if err != nil {
		t.Fatalf("UpdateTrait values returned error: %v", err)
	}
	_, err = UpdateTrait(UpdateTraitRequest{
		VaultPath: vault.Path,
		TraitName: "done",
		Default:   "true",
	})
	if err != nil {
		t.Fatalf("UpdateTrait default returned error: %v", err)
	}

	loaded, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	priority := loaded.Traits["priority"]
	wantValues := []string{"low", "medium", "high"}
	if len(priority.Values) != len(wantValues) {
		t.Fatalf("priority values = %v, want %v", priority.Values, wantValues)
	}
	for i := range wantValues {
		if priority.Values[i] != wantValues[i] {
			t.Fatalf("priority values = %v, want %v", priority.Values, wantValues)
		}
	}
	if got, ok := loaded.Traits["done"].Default.(bool); !ok || !got {
		t.Fatalf("done default = %#v, want bool(true)", loaded.Traits["done"].Default)
	}
}
