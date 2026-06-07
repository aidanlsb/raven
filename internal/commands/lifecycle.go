package commands

import "strings"

// ResolveCommandID resolves a CLI command path to a registry command ID.
// Example: "schema add field" -> "schema_add_field"
func ResolveCommandID(path string) (string, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", false
	}

	if _, ok := Registry[trimmed]; ok {
		return trimmed, true
	}

	underscored := strings.ReplaceAll(trimmed, " ", "_")
	if _, ok := Registry[underscored]; ok {
		return underscored, true
	}

	return "", false
}
