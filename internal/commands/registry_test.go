package commands

import (
	"strings"
	"testing"
)

// TestRegistryHasRequiredCommands verifies that essential commands exist.
func TestRegistryHasRequiredCommands(t *testing.T) {
	t.Parallel()
	requiredCommands := []string{
		"new", "add", "delete", "read", "move",
		"query", "backlinks", "vault_stats", "check", "date",
		"schema",
	}

	for _, cmd := range requiredCommands {
		if _, ok := Registry[cmd]; !ok {
			t.Errorf("Registry missing required command %q", cmd)
		}
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
		"vault_add",
		"vault_remove",
		"vault_pin",
		"vault_clear",
		"mcp_install",
		"mcp_remove",
		"mcp_status",
		"mcp_show",
		"skill_list",
		"skill_sync",
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
}

func TestRegistryLongDescriptionsUseCompactReindexGuidance(t *testing.T) {
	t.Parallel()

	for name, meta := range Registry {
		if strings.Contains(meta.LongDesc, "raven_reindex") {
			t.Fatalf("%s LongDesc references obsolete raven_reindex tool", name)
		}
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
	if got := UsageForMeta("set", meta); got != "set <object-id> <field=value>..." {
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
