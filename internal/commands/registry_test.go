package commands

import (
	"strings"
	"testing"
)

// TestRegistryHasRequiredCommands verifies that essential commands exist.
func TestRegistryHasRequiredCommands(t *testing.T) {
	t.Parallel()
	requiredCommands := []string{
		"new", "add", "delete", "read", "move", "section_create", "section_delete", "section_move", "section_rename",
		"query", "backlinks", "vault_stats", "check", "date",
		"schema",
	}

	for _, cmd := range requiredCommands {
		if _, ok := Registry[cmd]; !ok {
			t.Errorf("Registry missing required command %q", cmd)
		}
	}
}

// TestCLIPathSegments verifies the explicit CLIPath hierarchy field is used
// when present and falls back to the human-facing Name otherwise.
func TestCLIPathSegments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		meta Meta
		want []string
	}{
		{
			name: "explicit CLIPath wins",
			meta: Meta{Name: "vault config show", CLIPath: []string{"vault", "config", "show"}},
			want: []string{"vault", "config", "show"},
		},
		{
			name: "falls back to Name when CLIPath empty",
			meta: Meta{Name: "vault config auto-reindex set"},
			want: []string{"vault", "config", "auto-reindex", "set"},
		},
		{
			name: "single segment name",
			meta: Meta{Name: "new"},
			want: []string{"new"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.meta.CLIPathSegments()
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("CLIPathSegments() = %v, want %v", got, tc.want)
			}
		})
	}

	// Explicit CLIPath must match the underscore command ID mapping used by the
	// migrated vault config subtree.
	meta, ok := Registry["vault_config_protected_prefixes_add"]
	if !ok {
		t.Fatal("vault_config_protected_prefixes_add missing from registry")
	}
	if got := strings.Join(meta.CLIPathSegments(), " "); got != "vault config protected-prefixes add" {
		t.Fatalf("unexpected CLI path for protected-prefixes add: %q", got)
	}
}

// TestRegistryMetadataComplete verifies all commands have required metadata.
func TestRegistryMetadataComplete(t *testing.T) {
	t.Parallel()
	for name, meta := range Registry {
		t.Run(name, func(t *testing.T) {
			if meta.Name == "" {
				t.Error("Command has empty Name")
			}
			if meta.Description == "" {
				t.Error("Command has empty Description")
			}
			if meta.Category == "" {
				t.Error("Command has empty Category")
			}
			if meta.Access == "" {
				t.Error("Command has empty Access")
			}
			if meta.Risk == "" {
				t.Error("Command has empty Risk")
			}

			// Check args have names and descriptions
			for i, arg := range meta.Args {
				if arg.Name == "" {
					t.Errorf("Arg %d has empty Name", i)
				}
				if arg.Description == "" {
					t.Errorf("Arg %q has empty Description", arg.Name)
				}
			}

			// Check flags have names and descriptions
			for i, flag := range meta.Flags {
				if flag.Name == "" {
					t.Errorf("Flag %d has empty Name", i)
				}
				if flag.Description == "" {
					t.Errorf("Flag %q has empty Description", flag.Name)
				}
				if flag.Type == "" {
					t.Errorf("Flag %q has empty Type", flag.Name)
				}
			}
		})
	}
}

func TestRequiresVaultMetadata(t *testing.T) {
	t.Parallel()

	noVaultCommands := []string{
		"init",
		"serve",
		"version",
		"config",
		"config_show",
		"config_init",
		"config_set",
		"config_unset",
		"vault",
		"vault_list",
		"vault_current",
		"vault_use",
		"vault_focus",
		"vault_add",
		"vault_remove",
		"vault_pin",
		"vault_clear",
		"mcp_install",
		"mcp_remove",
		"mcp_status",
		"mcp_show",
		"skill_list",
		"skill_install",
		"skill_remove",
		"skill_doctor",
	}

	for _, commandID := range noVaultCommands {
		if RequiresVault(commandID) {
			t.Fatalf("expected %q to skip vault resolution", commandID)
		}
	}

	vaultCommands := []string{"query", "read", "vault_path", "vault_stats"}
	for _, commandID := range vaultCommands {
		if !RequiresVault(commandID) {
			t.Fatalf("expected %q to require a resolved vault", commandID)
		}
	}

	if RequiresVaultForInvocation("vault_list", nil) {
		t.Fatal("vault_list should remain available without a resolved vault")
	}
	if !RequiresVaultForInvocation("vault_list", map[string]interface{}{"path-only": true}) {
		t.Fatal("vault_list --path-only should require a resolved vault")
	}
}

func TestSkillInstallIsOnlyPackagedSkillInstallSurface(t *testing.T) {
	t.Parallel()

	if _, ok := Registry["skill_sync"]; ok {
		t.Fatal("skill_sync must not remain registered")
	}
	meta, ok := Registry["skill_install"]
	if !ok {
		t.Fatal("skill_install missing from registry")
	}
	flagNames := make([]string, 0, len(meta.Flags))
	for _, flag := range meta.Flags {
		flagNames = append(flagNames, flag.Name)
	}
	if got := strings.Join(flagNames, ","); got != "scope,dest,confirm" {
		t.Fatalf("skill_install flags = %q, want scope,dest,confirm", got)
	}
}

func TestGlobalConfigCommandContractsUsePositionalKeys(t *testing.T) {
	t.Parallel()

	setContract, ok := BuildCommandContract("config_set")
	if !ok {
		t.Fatal("config_set contract missing")
	}
	setSpec, ok := setContract.Parameters["settings"]
	if !ok || setSpec.Type != ParameterTypeStringArray || !setSpec.Required {
		t.Fatalf("config_set settings = %#v, want required string array", setSpec)
	}
	if len(setContract.Parameters) != 1 {
		t.Fatalf("config_set parameters = %#v, want only settings", setContract.Parameters)
	}

	unsetContract, ok := BuildCommandContract("config_unset")
	if !ok {
		t.Fatal("config_unset contract missing")
	}
	unsetSpec, ok := unsetContract.Parameters["keys"]
	if !ok || unsetSpec.Type != ParameterTypeStringArray || !unsetSpec.Required {
		t.Fatalf("config_unset keys = %#v, want required string array", unsetSpec)
	}
	if len(unsetContract.Parameters) != 1 {
		t.Fatalf("config_unset parameters = %#v, want only keys", unsetContract.Parameters)
	}
}

func TestRegistryLongDescriptionsUseCompactReindexGuidance(t *testing.T) {
	t.Parallel()

	for name, meta := range Registry {
		if strings.Contains(meta.LongDesc, "raven_reindex") {
			t.Fatalf("%s LongDesc references obsolete raven_reindex tool", name)
		}
	}
}

func TestQuerySavedSetExposesDefinitionFlagsOnly(t *testing.T) {
	t.Parallel()

	meta := Registry["query_saved_set"]
	names := make([]string, 0, len(meta.Flags))
	for _, flag := range meta.Flags {
		names = append(names, flag.Name)
	}
	if got := strings.Join(names, ","); got != "description,arg" {
		t.Fatalf("query_saved_set flags = %q, want definition flags only", got)
	}
}

func TestCheckFixMetadataListsSupportedFixes(t *testing.T) {
	t.Parallel()

	meta := Registry["check_fix"]
	for _, issueType := range []string{
		"short_ref_could_be_full_path",
		"invalid_enum_value",
		"non_canonical_ref",
		"non_canonical_path",
	} {
		if !strings.Contains(meta.LongDesc, issueType) {
			t.Fatalf("check_fix LongDesc missing supported fix issue type %q", issueType)
		}
	}
}

func TestUsageForMetaUsesExplicitQueryUsage(t *testing.T) {
	t.Parallel()

	meta := Registry["query"]
	if got := UsageForMeta("query", meta); got != "query <query_string|saved-query> [inputs...]" {
		t.Errorf("UsageForMeta(query) = %q, want explicit query usage", got)
	}

	// Check query_string remains required on the runnable query command.
	queryStringArgFound := false
	for _, arg := range Registry["query"].Args {
		if arg.Name == "query_string" {
			queryStringArgFound = arg.Required
			break
		}
	}
	if !queryStringArgFound {
		t.Error("Expected required query_string arg in registry metadata")
	}
}

func TestUsageForMetaDerivesNoArgCommandUsage(t *testing.T) {
	t.Parallel()

	meta := Registry["vault_stats"]
	if got := UsageForMeta("vault_stats", meta); got != "vault stats" {
		t.Errorf("UsageForMeta(vault_stats) = %q, want 'vault stats'", got)
	}
}

func TestUsageForMetaDerivesOptionalArgs(t *testing.T) {
	t.Parallel()

	meta := Registry["date"]
	if got := UsageForMeta("date", meta); got != "date [date]" {
		t.Errorf("UsageForMeta(date) = %q, want 'date [date]'", got)
	}
}

func TestUsageForMetaUsesExplicitUsageWhenPresent(t *testing.T) {
	t.Parallel()

	meta := Registry["set"]
	if got := UsageForMeta("set", meta); got != "set <reference> <field=value>..." {
		t.Errorf("UsageForMeta(set) = %q, want explicit set usage", got)
	}
}

func TestAllCommandsHaveUsage(t *testing.T) {
	t.Parallel()
	for name := range Registry {
		t.Run(name, func(t *testing.T) {
			if got := UsageForMeta(name, Registry[name]); got == "" {
				t.Errorf("UsageForMeta(%q) returned empty usage", name)
			}
		})
	}
}
