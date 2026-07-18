package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

// registryGroup describes a non-leaf (grouping) command in a generated
// subtree. Grouping commands have no registry entry of their own, so their
// user-facing metadata and default behavior are supplied here.
type registryGroup struct {
	Short string // Short description shown in help
	Long  string // Optional long description
	Use   string // Optional Use override; defaults to the last CLI path segment

	// DefaultLeafID, when set, makes the group's bare invocation execute the
	// named leaf command (via canonicalGroupDefaultRunE) and render it with
	// DefaultRender. When empty, the group's bare invocation prints help.
	DefaultLeafID string
	DefaultRender func(*cobra.Command, commandexec.Result) error
}

// registrySubtreeSpec configures generation of a Cobra subtree from registry
// hierarchy metadata. Leaf commands are discovered from the registry by CLI
// path prefix and built via newCanonicalLeafCommand; grouping commands are
// created on demand from Groups (keyed by their full CLI path joined by " ").
type registrySubtreeSpec struct {
	Prefix    []string      // CLI path of the subtree root (e.g. ["vault", "config"])
	VaultPath func() string // Vault path resolver shared by leaves and group defaults
	Root      registryGroup // Metadata/behavior for the subtree root command
	Groups    map[string]registryGroup
	Renders   map[string]func(*cobra.Command, commandexec.Result) error
}

// buildRegistrySubtree constructs and returns the root Cobra command for a
// generated subtree. Intermediate grouping commands are created as needed and
// leaves are attached under their registry-declared CLI path.
func buildRegistrySubtree(spec registrySubtreeSpec) *cobra.Command {
	root := newRegistryGroupCommand(spec.Prefix, spec.Root, spec.VaultPath)
	nodes := map[string]*cobra.Command{strings.Join(spec.Prefix, " "): root}

	for _, id := range registrySubtreeLeafIDs(spec.Prefix) {
		meta, ok := commands.EffectiveMeta(id)
		if !ok {
			continue
		}
		segs := meta.CLIPathSegments()

		parent := root
		for depth := len(spec.Prefix) + 1; depth < len(segs); depth++ {
			key := strings.Join(segs[:depth], " ")
			node, ok := nodes[key]
			if !ok {
				groupSpec, ok := spec.Groups[key]
				if !ok {
					panic(fmt.Sprintf("registry subtree %q missing group spec for %q", strings.Join(spec.Prefix, " "), key))
				}
				node = newRegistryGroupCommand(segs[:depth], groupSpec, spec.VaultPath)
				parent.AddCommand(node)
				nodes[key] = node
			}
			parent = node
		}

		parent.AddCommand(newCanonicalLeafCommand(id, canonicalLeafOptions{
			VaultPath:   spec.VaultPath,
			RenderHuman: spec.Renders[id],
		}))
	}

	return root
}

func newRegistryGroupCommand(path []string, group registryGroup, vaultPath func() string) *cobra.Command {
	use := group.Use
	if use == "" && len(path) > 0 {
		use = path[len(path)-1]
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: group.Short,
		Long:  group.Long,
		Args:  cobra.NoArgs,
	}

	if group.DefaultLeafID != "" {
		cmd.RunE = canonicalGroupDefaultRunE(group.DefaultLeafID, vaultPath, group.DefaultRender)
	} else {
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		}
	}

	return cmd
}

// registrySubtreeLeafIDs returns the registry command IDs whose CLI path lies
// strictly under the given prefix, sorted for deterministic construction.
func registrySubtreeLeafIDs(prefix []string) []string {
	var ids []string
	for id := range commands.Registry {
		meta, ok := commands.EffectiveMeta(id)
		if !ok {
			continue
		}
		segs := meta.CLIPathSegments()
		if len(segs) <= len(prefix) {
			continue
		}
		if !cliPathHasPrefix(segs, prefix) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func cliPathHasPrefix(segs, prefix []string) bool {
	if len(segs) < len(prefix) {
		return false
	}
	for i := range prefix {
		if segs[i] != prefix[i] {
			return false
		}
	}
	return true
}
