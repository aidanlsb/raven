package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestRegistryBackedCanonicalCommandFlagsMatchRegistry(t *testing.T) {
	for _, path := range commandPaths(rootCmd) {
		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Errorf("failed to locate command for path %q", path)
			continue
		}
		if !cmd.Runnable() || cmd.Annotations[canonicalLeafAnnotationKey] != "true" {
			continue
		}

		commandID, ok := registryCommandIDForCommand(cmd)
		if !ok {
			t.Errorf("canonical command %q is missing registry metadata", path)
			continue
		}
		meta, ok := commands.EffectiveMeta(commandID)
		if !ok {
			t.Errorf("canonical command %q resolved to missing registry id %q", path, commandID)
			continue
		}

		t.Run(path, func(t *testing.T) {
			cliFlags := make(map[string]*pflag.Flag)
			cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
				if flag.Name == "help" {
					return
				}
				cliFlags[flag.Name] = flag
			})

			registryFlags := make(map[string]commands.FlagMeta, len(meta.Flags))
			for _, flag := range meta.Flags {
				if flag.Type == commands.FlagTypePosKeyValue {
					continue
				}
				registryFlags[flag.Name] = flag
			}

			for name := range cliFlags {
				if _, ok := registryFlags[name]; !ok {
					t.Errorf("CLI flag %q is missing from registry metadata", name)
				}
			}
			for name := range registryFlags {
				cliFlag, ok := cliFlags[name]
				if !ok {
					t.Errorf("registry flag %q is missing from CLI command", name)
					continue
				}
				metaFlag := registryFlags[name]
				if cliFlag.Usage != metaFlag.Description {
					t.Errorf("CLI flag %q description = %q, want registry description %q", name, cliFlag.Usage, metaFlag.Description)
				}
				if cliFlag.Shorthand != metaFlag.Short {
					t.Errorf("CLI flag %q shorthand = %q, want registry shorthand %q", name, cliFlag.Shorthand, metaFlag.Short)
				}
				if cliFlag.Value.Type() != cobraFlagType(metaFlag.Type) {
					t.Errorf("CLI flag %q type = %q, want %q", name, cliFlag.Value.Type(), cobraFlagType(metaFlag.Type))
				}
				if cliFlag.DefValue != cobraFlagDefault(metaFlag) {
					t.Errorf("CLI flag %q default = %q, want %q", name, cliFlag.DefValue, cobraFlagDefault(metaFlag))
				}
			}
		})
	}
}

func cobraFlagType(flagType commands.FlagType) string {
	switch flagType {
	case commands.FlagTypeBool:
		return "bool"
	case commands.FlagTypeInt:
		return "int"
	case commands.FlagTypeKeyValue, commands.FlagTypeStringSlice:
		return "stringArray"
	default:
		return "string"
	}
}

func cobraFlagDefault(flag commands.FlagMeta) string {
	switch flag.Type {
	case commands.FlagTypeBool:
		if flag.Default == "true" {
			return "true"
		}
		return "false"
	case commands.FlagTypeInt:
		if strings.TrimSpace(flag.Default) == "" {
			return "0"
		}
		return strings.TrimSpace(flag.Default)
	case commands.FlagTypeKeyValue, commands.FlagTypeStringSlice:
		return "[]"
	default:
		return flag.Default
	}
}

func TestCommandsMissingRegistryMetadataAreAllowlisted(t *testing.T) {
	paths := commandPaths(rootCmd)
	for _, path := range paths {
		if path == "" {
			continue
		}

		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Errorf("failed to locate command for path %q", path)
			continue
		}
		if !cmd.Runnable() {
			continue
		}
		// Grouping commands (e.g. "schema add") intentionally
		// rely on metadata for runnable leaf commands.
		if len(cmd.Commands()) > 0 {
			if _, ok := lookupRegistryMeta(path); !ok {
				continue
			}
		}

		if _, ok := lookupRegistryMeta(path); ok {
			continue
		}
		if cmd.Annotations[localLeafAnnotationKey] == "true" {
			continue
		}
		t.Errorf("CLI command %q is missing registry metadata", path)
	}
}

func TestRegistryBackedLeafCommandsUseCanonicalAdapterOrMarkLocal(t *testing.T) {
	paths := commandPaths(rootCmd)
	for _, path := range paths {
		if path == "" {
			continue
		}

		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Errorf("failed to locate command for path %q", path)
			continue
		}
		if !cmd.Runnable() || len(cmd.Commands()) > 0 {
			continue
		}
		if _, ok := lookupRegistryMeta(path); !ok {
			continue
		}
		if cmd.Annotations[canonicalLeafAnnotationKey] == "true" {
			continue
		}
		if cmd.Annotations[localLeafAnnotationKey] == "true" {
			continue
		}

		t.Errorf("registry-backed CLI leaf %q is still manually wired; use canonical adapter or mark it local", path)
	}
}

func TestRegistryBackedCommandsMatchRegistryVaultPolicy(t *testing.T) {
	paths := commandPaths(rootCmd)
	for _, path := range paths {
		if path == "" {
			continue
		}

		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Errorf("failed to locate command for path %q", path)
			continue
		}
		if !cmd.Runnable() {
			continue
		}

		commandID, ok := registryCommandIDForCommand(cmd)
		if !ok {
			continue
		}

		got := shouldResolveVaultForCommand(cmd)
		want := commands.RequiresVault(commandID)
		if got != want {
			t.Errorf("command %q (registry id %q) resolveVault=%v, want %v", path, commandID, got, want)
		}
	}
}

func TestExplicitNoVaultRuntimeCommands(t *testing.T) {
	cases := make([]*cobra.Command, 0, 2)
	for _, path := range []string{"mcp", "skill"} {
		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Fatalf("failed to locate command for path %q", path)
		}
		cases = append(cases, cmd)
	}

	for _, cmd := range cases {
		if shouldResolveVaultForCommand(cmd) {
			t.Fatalf("expected %q to skip vault resolution", cmd.CommandPath())
		}
	}
}

func commandPaths(root *cobra.Command) []string {
	var out []string
	var walk func(cmd *cobra.Command, prefix string)

	walk = func(cmd *cobra.Command, prefix string) {
		for _, child := range cmd.Commands() {
			path := child.Name()
			if prefix != "" {
				path = strings.TrimSpace(prefix + " " + child.Name())
			}
			out = append(out, path)
			walk(child, path)
		}
	}

	walk(root, "")
	return out
}

func findCommandByPath(root *cobra.Command, path string) (*cobra.Command, bool) {
	parts := strings.Fields(path)
	cur := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range cur.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil, false
		}
		cur = next
	}
	return cur, true
}
