package commands

import (
	"testing"
)

// TestRegistryHasRequiredCommands verifies that essential commands exist.
func TestRegistryHasRequiredCommands(t *testing.T) {
	requiredCommands := []string{
		"new", "add", "delete", "read", "move",
		"query", "backlinks", "stats", "check", "date",
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
	for name, meta := range Registry {
		t.Run(name, func(t *testing.T) {
			if meta.Name == "" {
				t.Error("Command has empty Name")
			}
			if meta.Description == "" {
				t.Error("Command has empty Description")
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

// TestCobraCommandGeneration verifies Cobra command generation works.
func TestCobraCommandGeneration(t *testing.T) {
	// Test a command with args and flags
	cmd := GenerateCobraCommand("query", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'query'")
	}

	if cmd.Use != "query <query_string>" {
		t.Errorf("Use = %q, want 'query <query_string>'", cmd.Use)
	}

	// Check flag was added
	listFlag := cmd.Flags().Lookup("list")
	if listFlag == nil {
		t.Error("Missing 'list' flag")
	}
}

// TestCobraCommandWithNoArgs verifies commands with no args work.
func TestCobraCommandWithNoArgs(t *testing.T) {
	cmd := GenerateCobraCommand("stats", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'stats'")
	}

	if cmd.Use != "stats" {
		t.Errorf("Use = %q, want 'stats'", cmd.Use)
	}
}

// TestCobraCommandWithOptionalArgs verifies optional args are handled.
func TestCobraCommandWithOptionalArgs(t *testing.T) {
	cmd := GenerateCobraCommand("date", nil)
	if cmd == nil {
		t.Fatal("GenerateCobraCommand returned nil for 'date'")
	}

	// date has optional [date] arg
	if cmd.Use != "date [date]" {
		t.Errorf("Use = %q, want 'date [date]'", cmd.Use)
	}
}

// TestAllCommandsGeneratable verifies all registry commands can generate Cobra commands.
func TestAllCommandsGeneratable(t *testing.T) {
	for name := range Registry {
		t.Run(name, func(t *testing.T) {
			cmd := GenerateCobraCommand(name, nil)
			if cmd == nil {
				t.Errorf("GenerateCobraCommand returned nil for %q", name)
			}
		})
	}
}
