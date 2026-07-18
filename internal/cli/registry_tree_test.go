package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandimpl"
	"github.com/aidanlsb/raven/internal/commands"
)

// TestVaultConfigSubtreeMatchesRegistryAndHandlers asserts the three-way parity
// for the registry-generated "vault config" subtree: every registry command ID
// under the subtree prefix is reachable at its declared CLI path, that path
// resolves back to the same registry ID, and a handler is registered for it.
func TestVaultConfigSubtreeMatchesRegistryAndHandlers(t *testing.T) {
	prefix := []string{"vault", "config"}

	handlers := commandexec.NewHandlerRegistry()
	commandimpl.RegisterAll(handlers)

	ids := registrySubtreeLeafIDs(prefix)
	if len(ids) == 0 {
		t.Fatal("expected the vault config subtree to expose registry leaves")
	}

	for _, id := range ids {
		meta, ok := commands.EffectiveMeta(id)
		if !ok {
			t.Errorf("registry metadata missing for %q", id)
			continue
		}

		segs := meta.CLIPathSegments()
		if !cliPathHasPrefix(segs, prefix) {
			t.Errorf("registry id %q has CLI path %v outside subtree prefix %v", id, segs, prefix)
			continue
		}
		path := strings.Join(segs, " ")

		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Errorf("generated CLI command missing for registry id %q (path %q)", id, path)
			continue
		}
		if !cmd.Runnable() {
			t.Errorf("generated CLI command %q is not runnable", path)
		}
		if cmd.Annotations[canonicalLeafAnnotationKey] != "true" {
			t.Errorf("generated leaf %q is not built via the canonical adapter", path)
		}

		gotID, ok := registryCommandIDForCommand(cmd)
		if !ok || gotID != id {
			t.Errorf("path %q resolved to registry id %q (ok=%v), want %q", path, gotID, ok, id)
		}

		if _, ok := handlers.Lookup(id); !ok {
			t.Errorf("no handler registered for registry id %q", id)
		}
	}
}

// TestVaultConfigSubtreeGroupsPrintHelpOrDefault asserts the generated grouping
// commands under "vault config" are non-leaf containers wired with a default
// RunE (either a canonical group default or help), matching the pre-generation
// hand-written tree.
func TestVaultConfigSubtreeGroupsPrintHelpOrDefault(t *testing.T) {
	groupPaths := []string{
		"vault config",
		"vault config auto-reindex",
		"vault config protected-prefixes",
		"vault config exclude",
		"vault config directories",
		"vault config capture",
		"vault config deletion",
	}

	for _, path := range groupPaths {
		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Errorf("generated group command missing for path %q", path)
			continue
		}
		if len(cmd.Commands()) == 0 {
			t.Errorf("group command %q has no subcommands", path)
		}
		if cmd.RunE == nil {
			t.Errorf("group command %q has no default RunE", path)
		}
		if cmd.Annotations[canonicalLeafAnnotationKey] == "true" {
			t.Errorf("group command %q should not be a canonical leaf", path)
		}
	}
}
