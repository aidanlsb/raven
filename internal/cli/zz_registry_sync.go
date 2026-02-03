package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
)

func syncRegistryMetadata(root *cobra.Command) {
	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		if path != "" {
			applyRegistryMetadata(cmd, path)
		}

		for _, child := range cmd.Commands() {
			childPath := child.Name()
			if path != "" {
				childPath = path + " " + child.Name()
			}
			walk(child, childPath)
		}
	}

	walk(root, "")
}

func applyRegistryMetadata(cmd *cobra.Command, path string) {
	meta, ok := lookupRegistryMeta(path)
	if !ok {
		return
	}

	// Keep Use strings defined in CLI because the registry does not model
	// conditional or variadic usage (e.g., positional key=value arguments).
	if meta.Description != "" {
		cmd.Short = meta.Description
	}

	if meta.LongDesc != "" || len(meta.Examples) > 0 {
		longDesc := buildLongDesc(meta)
		if meta.LongDesc != "" || cmd.Long == "" {
			cmd.Long = longDesc
		}
	}
}

func lookupRegistryMeta(path string) (commands.Meta, bool) {
	if meta, ok := commands.Registry[path]; ok {
		return meta, true
	}

	underscored := strings.ReplaceAll(path, " ", "_")
	if meta, ok := commands.Registry[underscored]; ok {
		return meta, true
	}

	return commands.Meta{}, false
}

func buildLongDesc(meta commands.Meta) string {
	longDesc := meta.Description
	if meta.LongDesc != "" {
		longDesc = meta.LongDesc
	}
	if len(meta.Examples) == 0 {
		return longDesc
	}

	var b strings.Builder
	b.WriteString(longDesc)
	b.WriteString("\n\nExamples:\n")
	for _, ex := range meta.Examples {
		b.WriteString("  ")
		b.WriteString(ex)
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
