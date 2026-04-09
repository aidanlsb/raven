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

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
