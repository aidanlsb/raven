package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandimpl"
	"github.com/aidanlsb/raven/internal/commands"
)

// registryGeneratedSubtreePrefixes lists the CLI subtree roots whose command
// hierarchy is generated from registry metadata via buildRegistrySubtree. New
// migrated groups should be added here so the parity assertions cover them.
var registryGeneratedSubtreePrefixes = [][]string{
	{"section"},
	{"vault", "config"},
	{"schema", "template"},
	{"schema", "add"},
	{"schema", "update"},
	{"schema", "remove"},
	{"schema", "convert"},
	{"schema", "rename"},
}

// TestRegistryGeneratedSubtreesMatchRegistryAndHandlers asserts the three-way
// parity for every registry-generated subtree: each registry command ID under
// the subtree prefix is reachable at its declared CLI path, that path resolves
// back to the same registry ID, and a handler is registered for it.
func TestRegistryGeneratedSubtreesMatchRegistryAndHandlers(t *testing.T) {
	handlers := commandexec.NewHandlerRegistry()
	commandimpl.RegisterAll(handlers)

	for _, prefix := range registryGeneratedSubtreePrefixes {
		t.Run(strings.Join(prefix, " "), func(t *testing.T) {
			ids := registrySubtreeLeafIDs(prefix)
			if len(ids) == 0 {
				t.Fatalf("expected the %q subtree to expose registry leaves", strings.Join(prefix, " "))
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
		})
	}
}

func TestRemovedAssetCommandIsAbsent(t *testing.T) {
	t.Parallel()

	if _, ok := findCommandByPath(rootCmd, "asset"); ok {
		t.Fatal("removed asset command is still registered")
	}
	if _, ok := commands.EffectiveMeta("asset_import"); ok {
		t.Fatal("removed asset import command is still in the registry")
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

// TestSchemaEditSubtreeGroupsArePureContainers locks in the help layout for the
// registry-generated schema-edit subtrees. These groups were hand-wired without
// a RunE, so their usage block omits the "[flags]" line; the generated
// ParentOnly groups must stay non-runnable to preserve that.
func TestSchemaEditSubtreeGroupsArePureContainers(t *testing.T) {
	groupPaths := []string{
		"schema add",
		"schema update",
		"schema remove",
		"schema convert",
		"schema rename",
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
		if cmd.Runnable() {
			t.Errorf("group command %q should be a non-runnable pure container", path)
		}
		if cmd.Annotations[canonicalLeafAnnotationKey] == "true" {
			t.Errorf("group command %q should not be a canonical leaf", path)
		}
	}
}

// TestSchemaTemplateSubtreeRootPrintsHelp asserts the "schema template" root is
// a runnable help group (matching its hand-wired predecessor, which set a
// help RunE), so its usage layout is preserved.
func TestSchemaTemplateSubtreeRootPrintsHelp(t *testing.T) {
	cmd, ok := findCommandByPath(rootCmd, "schema template")
	if !ok {
		t.Fatal("generated group command missing for path \"schema template\"")
	}
	if len(cmd.Commands()) == 0 {
		t.Error("group command \"schema template\" has no subcommands")
	}
	if cmd.RunE == nil {
		t.Error("group command \"schema template\" has no default RunE")
	}
	if cmd.Annotations[canonicalLeafAnnotationKey] == "true" {
		t.Error("group command \"schema template\" should not be a canonical leaf")
	}
}

func TestSchemaConvertHelpDocumentsExhaustiveMapping(t *testing.T) {
	for _, path := range []string{"schema convert trait", "schema convert field"} {
		cmd, ok := findCommandByPath(rootCmd, path)
		if !ok {
			t.Fatalf("command missing for path %q", path)
		}
		if cmd.Flags().Lookup("map-json") == nil {
			t.Fatalf("%s missing --map-json", path)
		}
		if !strings.Contains(cmd.Long, "must cover every") {
			t.Fatalf("%s help does not document exhaustive mapping:\n%s", path, cmd.Long)
		}
		if !strings.Contains(strings.ToLower(cmd.Long), "array-to-array") {
			t.Fatalf("%s help does not document collection member mapping:\n%s", path, cmd.Long)
		}
	}
}
