package commands

import (
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
}

// TestCobraCommandGeneration verifies Cobra command generation works.
func TestCobraCommandGeneration(t *testing.T) {
	t.Parallel()
	// Test a command with args and flags
	cmd := GenerateCobraCommand("query", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'query'")
	}

	if cmd.Use != "query <query_string|saved-query> [inputs...]" {
		t.Errorf("Use = %q, want explicit query usage", cmd.Use)
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

// TestCobraCommandWithNoArgs verifies commands with no args work.
func TestCobraCommandWithNoArgs(t *testing.T) {
	t.Parallel()
	cmd := GenerateCobraCommand("vault_stats", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'vault_stats'")
	}

	if cmd.Use != "vault stats" {
		t.Errorf("Use = %q, want 'vault stats'", cmd.Use)
	}
}

// TestCobraCommandWithOptionalArgs verifies optional args are handled.
func TestCobraCommandWithOptionalArgs(t *testing.T) {
	t.Parallel()
	cmd := GenerateCobraCommand("date", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'date'")
	}

	// date has optional [date] arg
	if cmd.Use != "date [date]" {
		t.Errorf("Use = %q, want 'date [date]'", cmd.Use)
	}
}

func TestCobraCommandUsesExplicitUsageWhenPresent(t *testing.T) {
	t.Parallel()
	cmd := GenerateCobraCommand("set", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'set'")
	}

	if cmd.Use != "set <object-id> <field=value>..." {
		t.Errorf("Use = %q, want explicit set usage", cmd.Use)
	}
}

// TestAllCommandsGeneratable verifies all registry commands can generate Cobra commands.
func TestAllCommandsGeneratable(t *testing.T) {
	t.Parallel()
	for name := range Registry {
		t.Run(name, func(t *testing.T) {
			cmd := GenerateCobraCommand(name, nil)
			if cmd == nil {
				t.Errorf("GenerateCobraCommand returned nil for %q", name)
			}
		})
	}
}
