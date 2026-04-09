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

	if meta.Use != "" {
		cmd.Use = meta.Use
	}

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
	commandID, ok := lookupRegistryCommandID(path)
	if !ok {
		return commands.Meta{}, false
	}
	return commands.EffectiveMeta(commandID)
}

func lookupRegistryCommandID(path string) (string, bool) {
	if _, ok := commands.EffectiveMeta(path); ok {
		return path, true
	}

	underscored := strings.ReplaceAll(path, " ", "_")
	underscored = strings.ReplaceAll(underscored, "-", "_")
	if _, ok := commands.EffectiveMeta(underscored); ok {
		return underscored, true
	}

	return "", false
}

func registryCommandIDForCommand(cmd *cobra.Command) (string, bool) {
	path := commandPathForCommand(cmd)
	if path == "" {
		return "", false
	}
	return lookupRegistryCommandID(path)
}

func commandPathForCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}

	parts := make([]string, 0)
	for cur := cmd; cur != nil && cur.Parent() != nil; cur = cur.Parent() {
		parts = append(parts, cur.Name())
	}
	if len(parts) == 0 {
		return ""
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, " ")
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
