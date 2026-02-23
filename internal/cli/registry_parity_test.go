package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestCheckCommandFlagsMatchRegistry(t *testing.T) {
	meta, ok := commands.Registry["check"]
	if !ok {
		t.Fatal("check command missing from registry")
	}

	cmd, ok := findCommandByPath(rootCmd, "check")
	if !ok {
		t.Fatal("check command missing from CLI tree")
	}

	cliFlags := make(map[string]struct{})
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "help" {
			return
		}
		cliFlags[flag.Name] = struct{}{}
	})

	registryFlags := make(map[string]struct{}, len(meta.Flags))
	for _, flag := range meta.Flags {
		registryFlags[flag.Name] = struct{}{}
	}

	for name := range cliFlags {
		if _, ok := registryFlags[name]; !ok {
			t.Errorf("CLI check flag %q is missing from registry metadata", name)
		}
	}
	for name := range registryFlags {
		if _, ok := cliFlags[name]; !ok {
			t.Errorf("registry check flag %q is missing from CLI command", name)
		}
	}
}

func TestCommandsMissingRegistryMetadataAreAllowlisted(t *testing.T) {
	allowMissing := []string{
		"migrate",
		"migrate directories",
		"path",
		"schema add",
		"schema remove",
		"schema rename",
		"schema update",
		"serve",
	}

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
		// Grouping commands (e.g. "schema add", "workflow") intentionally
		// rely on metadata for runnable leaf commands.
		if len(cmd.Commands()) > 0 {
			if _, ok := lookupRegistryMeta(path); !ok {
				continue
			}
		}

		if _, ok := lookupRegistryMeta(path); ok {
			continue
		}
		if slices.Contains(allowMissing, path) {
			continue
		}
		t.Errorf("CLI command %q is missing registry metadata", path)
	}

	for _, allowed := range allowMissing {
		if _, ok := findCommandByPath(rootCmd, allowed); !ok {
			t.Errorf("allowlist entry %q no longer exists in CLI tree; update test", allowed)
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
