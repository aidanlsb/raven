package commands

import (
	"reflect"
	"testing"
)

func TestBuildCommandContractUpdateUsesTraitIDsForBulkStdin(t *testing.T) {
	t.Parallel()

	contract, ok := BuildCommandContract("update")
	if !ok {
		t.Fatal("expected update contract")
	}

	spec, ok := contract.Parameters["trait_ids"]
	if !ok {
		t.Fatalf("expected trait_ids bulk parameter, got %#v", contract.Parameters)
	}
	if _, ok := contract.Parameters["object_ids"]; ok {
		t.Fatalf("did not expect object_ids to remain canonical for update: %#v", contract.Parameters)
	}
	if !reflect.DeepEqual(spec.Aliases, []string{"object_ids", "ids"}) {
		t.Fatalf("trait_ids aliases = %#v, want object_ids + ids", spec.Aliases)
	}
	if !containsString(contract.ParameterOrder, "trait_ids") {
		t.Fatalf("parameter order %v does not include trait_ids", contract.ParameterOrder)
	}
}

func TestValidateArgumentsStrictNormalizesUpdateBulkAliases(t *testing.T) {
	t.Parallel()

	contract, ok := BuildCommandContract("update")
	if !ok {
		t.Fatal("expected update contract")
	}
	spec := BuildInvokeParamSpec(contract)

	cases := []struct {
		name string
		key  string
	}{
		{name: "legacy object_ids alias", key: "object_ids"},
		{name: "hyphenated canonical key", key: "trait-ids"},
		{name: "legacy ids alias", key: "ids"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			normalized, issues := ValidateArgumentsStrict(spec, map[string]interface{}{
				"stdin": true,
				"value": "done",
				tc.key:  []interface{}{"tasks/task1.md:trait:0", "tasks/task1.md:trait:1"},
			})
			if len(issues) > 0 {
				t.Fatalf("expected alias %q to validate, got issues: %#v", tc.key, issues)
			}

			rawIDs, ok := normalized["trait_ids"].([]interface{})
			if !ok {
				t.Fatalf("trait_ids missing or wrong type: %#v", normalized)
			}
			if len(rawIDs) != 2 {
				t.Fatalf("trait_ids = %#v, want 2 entries", rawIDs)
			}
			if _, ok := normalized["object_ids"]; ok {
				t.Fatalf("expected object_ids alias to normalize away, got %#v", normalized)
			}
			if _, ok := normalized["ids"]; ok {
				t.Fatalf("expected ids alias to normalize away, got %#v", normalized)
			}
		})
	}
}

func TestBuildCommandContractBulkPreviewModes(t *testing.T) {
	t.Parallel()

	for _, commandID := range []string{"add", "delete", "move", "set", "update"} {
		t.Run(commandID, func(t *testing.T) {
			t.Parallel()

			contract, ok := BuildCommandContract(commandID)
			if !ok {
				t.Fatalf("expected %s contract", commandID)
			}
			if got := contract.PreviewMode; got != "bulk_preview_default" {
				t.Fatalf("%s preview mode=%q, want bulk_preview_default", commandID, got)
			}
			if got := PreviewModeForCommandID(commandID); got != PreviewModeBulkPreviewDefault {
				t.Fatalf("%s policy preview mode=%q, want %q", commandID, got, PreviewModeBulkPreviewDefault)
			}
		})
	}
}

func TestBuildCommandContractPreviewDefaultForApplyCommands(t *testing.T) {
	t.Parallel()

	for _, commandID := range []string{
		"check",
		"check create-missing",
		"check_fix",
		"edit",
		"query",
		"schema_rename_field",
		"schema_rename_type",
		"skill_remove",
		"skill_sync",
	} {
		t.Run(commandID, func(t *testing.T) {
			t.Parallel()

			contract, ok := BuildCommandContract(commandID)
			if !ok {
				t.Fatalf("expected %s contract", commandID)
			}
			if got := contract.PreviewMode; got != "preview_default" {
				t.Fatalf("%s preview mode=%q, want preview_default", commandID, got)
			}
			if got := PreviewModeForCommandID(commandID); got != PreviewModePreviewDefault {
				t.Fatalf("%s policy preview mode=%q, want %q", commandID, got, PreviewModePreviewDefault)
			}
		})
	}
}

func TestConfirmFlagsHaveExplicitPreviewPolicy(t *testing.T) {
	t.Parallel()

	confirmFlagNonApplyCommands := map[string]struct{}{
		"query_saved_set": {},
	}
	for commandID, meta := range Registry {
		if !hasConfirmFlag(meta) {
			continue
		}
		if _, ok := confirmFlagNonApplyCommands[commandID]; ok {
			continue
		}
		if got := PreviewModeForCommandID(commandID); got == PreviewModeNone {
			t.Fatalf("%s exposes confirm but has no explicit preview policy", commandID)
		}
	}
}

func TestShouldPreviewByDefaultOnlyEnablesBulkPolicyForBulkInputs(t *testing.T) {
	t.Parallel()

	if ShouldPreviewByDefault("set", map[string]interface{}{"object_id": "people/alice"}) {
		t.Fatal("single set should not request preview by default")
	}
	if !ShouldPreviewByDefault("set", map[string]interface{}{
		"stdin":      true,
		"object_ids": []interface{}{"people/alice"},
	}) {
		t.Fatal("bulk set should request preview by default")
	}
	if !ShouldPreviewByDefault("edit", map[string]interface{}{"path": "note/example"}) {
		t.Fatal("edit should request preview by default")
	}
	if ShouldPreviewByDefault("query_saved_set", map[string]interface{}{"confirm": true}) {
		t.Fatal("query saved set confirm option should not imply command preview")
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasConfirmFlag(meta Meta) bool {
	for _, flag := range meta.Flags {
		if flag.Name == "confirm" && flag.Type == FlagTypeBool {
			return true
		}
	}
	return false
}
