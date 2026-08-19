package commands

import (
	"strings"
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
	if len(spec.Aliases) != 0 {
		t.Fatalf("trait_ids aliases = %#v, want none", spec.Aliases)
	}
	if !containsString(contract.ParameterOrder, "trait_ids") {
		t.Fatalf("parameter order %v does not include trait_ids", contract.ParameterOrder)
	}
}

func TestValidateArgumentsStrictRejectsRetiredUpdateBulkAliases(t *testing.T) {
	t.Parallel()

	contract, ok := BuildCommandContract("update")
	if !ok {
		t.Fatal("expected update contract")
	}
	spec := BuildInvokeParamSpec(contract)

	normalized, issues := ValidateArgumentsStrict(spec, map[string]interface{}{
		"stdin":     true,
		"value":     "done",
		"trait-ids": []interface{}{"tasks/task1.md:trait:0", "tasks/task1.md:trait:1"},
	})
	if len(issues) > 0 {
		t.Fatalf("expected hyphenated canonical key to validate, got issues: %#v", issues)
	}
	if got := len(normalized["trait_ids"].([]interface{})); got != 2 {
		t.Fatalf("trait_ids length = %d, want 2", got)
	}

	for _, key := range []string{"object_ids", "ids"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			_, issues := ValidateArgumentsStrict(spec, map[string]interface{}{
				"stdin": true,
				"value": "done",
				key:     []interface{}{"tasks/task1.md:trait:0", "tasks/task1.md:trait:1"},
			})
			if len(issues) == 0 || issues[0].Code != "UNKNOWN_ARGUMENT" {
				t.Fatalf("expected %q to be rejected as unknown, got %#v", key, issues)
			}
		})
	}
}

func TestBuildCommandContractBulkPreviewModes(t *testing.T) {
	t.Parallel()

	for _, commandID := range []string{"add", "delete", "move", "reclassify", "set", "update"} {
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

func TestBuildCommandContractReclassifyBulkArguments(t *testing.T) {
	t.Parallel()

	contract, ok := BuildCommandContract("reclassify")
	if !ok {
		t.Fatal("expected reclassify contract")
	}
	if contract.Parameters["reference"].Required {
		t.Fatal("reclassify reference should be optional for references bulk mode")
	}
	if !contract.Parameters["new-type"].Required {
		t.Fatal("reclassify new-type should remain required")
	}
	if got := contract.Parameters["references"].Type; got != ParameterTypeStringArray {
		t.Fatalf("reclassify references type=%q, want %q", got, ParameterTypeStringArray)
	}
	for _, retired := range []string{"object", "object_ids"} {
		if _, ok := contract.Parameters[retired]; ok {
			t.Fatalf("reclassify contract still exposes retired argument %q", retired)
		}
	}
}

func TestTargetCommandsUseReferenceArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		commandID string
		retired   []string
		bulk      bool
	}{
		{commandID: "open", retired: []string{"object_ids"}, bulk: true},
		{commandID: "resolve"},
		{commandID: "read", retired: []string{"path"}},
		{commandID: "set", retired: []string{"object_id", "object_ids"}, bulk: true},
		{commandID: "unset", retired: []string{"object_id"}},
		{commandID: "delete", retired: []string{"object_id", "object_ids"}, bulk: true},
		{commandID: "reclassify", retired: []string{"object", "object_ids"}, bulk: true},
		{commandID: "edit", retired: []string{"path"}},
		{commandID: "check", retired: []string{"path"}},
		{commandID: "check_fix", retired: []string{"path"}},
		{commandID: "backlinks", retired: []string{"target", "targets"}, bulk: true},
		{commandID: "outlinks", retired: []string{"source", "sources"}, bulk: true},
	}

	for _, tt := range tests {
		t.Run(tt.commandID, func(t *testing.T) {
			t.Parallel()

			meta := Registry[tt.commandID]
			if len(meta.Args) == 0 || meta.Args[0].Name != "reference" {
				t.Fatalf("%s first positional = %#v, want reference", tt.commandID, meta.Args)
			}
			contract, ok := BuildCommandContract(tt.commandID)
			if !ok {
				t.Fatalf("expected %s contract", tt.commandID)
			}
			if _, ok := contract.Parameters["reference"]; !ok {
				t.Fatalf("%s contract missing reference parameter", tt.commandID)
			}
			if tt.bulk {
				if got := contract.Parameters["references"].Type; got != ParameterTypeStringArray {
					t.Fatalf("%s references type = %q, want %q", tt.commandID, got, ParameterTypeStringArray)
				}
			}
			for _, retired := range tt.retired {
				if _, ok := contract.Parameters[retired]; ok {
					t.Fatalf("%s contract still exposes retired argument %q", tt.commandID, retired)
				}
				for name, parameter := range contract.Parameters {
					if containsString(parameter.Aliases, retired) {
						t.Fatalf("%s parameter %q still aliases retired argument %q", tt.commandID, name, retired)
					}
				}
			}
		})
	}
}

func TestCLIOptionalArgsRemainRequiredInCommandContracts(t *testing.T) {
	t.Parallel()

	for _, commandID := range []string{"read", "resolve", "search"} {
		t.Run(commandID, func(t *testing.T) {
			t.Parallel()

			contract, ok := BuildCommandContract(commandID)
			if !ok {
				t.Fatalf("expected %s contract", commandID)
			}
			if len(contract.Required) != 1 {
				t.Fatalf("%s required args = %#v, want one canonical required arg", commandID, contract.Required)
			}
			requiredName := contract.Required[0]
			if !contract.Parameters[requiredName].Required {
				t.Fatalf("%s parameter %q should remain required in MCP/canonical contract", commandID, requiredName)
			}
			if !Registry[commandID].Args[0].CLIOptional {
				t.Fatalf("%s arg should document interactive CLI optionality", commandID)
			}
			if got := FullCLIUsage(commandID); !strings.Contains(got, "[") {
				t.Fatalf("%s CLI usage = %q, want optional bracket form", commandID, got)
			}
		})
	}
}

func TestVariadicCLIArgsRemainStringArraysInCommandContracts(t *testing.T) {
	t.Parallel()

	meta := Registry["skill_install"]
	if len(meta.Args) != 1 || !meta.Args[0].Variadic || meta.Args[0].Name != "names" {
		t.Fatalf("skill_install args = %#v, want variadic names positional arg", meta.Args)
	}
	for _, flag := range meta.Flags {
		if flag.Name == "names" {
			t.Fatal("skill_install names must be positional in the CLI, not registry flag metadata")
		}
	}

	contract, ok := BuildCommandContract("skill_install")
	if !ok {
		t.Fatal("expected skill_install contract")
	}
	if got := contract.Parameters["names"].Type; got != ParameterTypeStringArray {
		t.Fatalf("skill_install names type = %q, want %q", got, ParameterTypeStringArray)
	}
}

func TestBacklinksOutlinksExposeStdinReplacementContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		commandID string
	}{
		{commandID: "backlinks"},
		{commandID: "outlinks"},
	}

	for _, tt := range tests {
		t.Run(tt.commandID, func(t *testing.T) {
			t.Parallel()

			contract, ok := BuildCommandContract(tt.commandID)
			if !ok {
				t.Fatalf("expected %s contract", tt.commandID)
			}
			if containsString(contract.Required, "reference") {
				t.Fatalf("%s should not unconditionally require reference when --stdin is available", tt.commandID)
			}
			if _, ok := contract.Parameters["references"]; !ok {
				t.Fatalf("%s contract missing stdin replacement parameter references", tt.commandID)
			}
			if got := contract.Parameters["references"].Type; got != ParameterTypeStringArray {
				t.Fatalf("%s replacement type = %q, want string array", tt.commandID, got)
			}
			if !Registry[tt.commandID].Args[0].CLIOptional {
				t.Fatalf("%s arg should document interactive CLI optionality", tt.commandID)
			}
		})
	}
}

func TestBuildCommandContractPreviewDefaultForApplyCommands(t *testing.T) {
	t.Parallel()

	for _, commandID := range []string{
		"check create-missing",
		"check_fix",
		"query",
		"schema_convert_field",
		"schema_convert_trait",
		"schema_rename_field",
		"schema_rename_type",
		"skill_install",
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

	for commandID, meta := range Registry {
		if !hasConfirmFlag(meta) {
			continue
		}
		if got := PreviewModeForCommandID(commandID); got == PreviewModeNone {
			t.Fatalf("%s exposes confirm but has no explicit preview policy", commandID)
		}
	}
}

func TestSchemaConvertRequiresMapJSON(t *testing.T) {
	t.Parallel()
	for _, commandID := range []string{"schema_convert_trait", "schema_convert_field"} {
		contract, ok := BuildCommandContract(commandID)
		if !ok {
			t.Fatalf("expected %s contract", commandID)
		}
		spec := contract.Parameters["map-json"]
		if !spec.Required || !containsString(contract.Required, "map-json") {
			t.Fatalf("%s map-json should be required: %#v", commandID, contract)
		}
		if spec.Type != ParameterTypeObject {
			t.Fatalf("%s map-json type=%q, want object", commandID, spec.Type)
		}
	}
}

func TestShouldPreviewByDefaultOnlyEnablesBulkPolicyForBulkInputs(t *testing.T) {
	t.Parallel()

	if ShouldPreviewByDefault("set", map[string]interface{}{"reference": "people/alice"}) {
		t.Fatal("single set should not request preview by default")
	}
	if !ShouldPreviewByDefault("set", map[string]interface{}{
		"stdin":      true,
		"references": []interface{}{"people/alice"},
	}) {
		t.Fatal("bulk set should request preview by default")
	}
	if ShouldPreviewByDefault("edit", map[string]interface{}{"reference": "note/example"}) {
		t.Fatal("edit applies by default and should not request preview")
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
