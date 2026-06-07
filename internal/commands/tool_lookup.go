package commands

import "strings"

var compatibilityToolCommandAliases = map[string]string{
	"raven_vault":    "vault_list",
	"raven_config":   "config_show",
	"raven_template": "template_list",
}

// ResolveToolCommandID resolves an MCP tool name to a registry command ID.
//
// Accepted forms:
// - MCP tool names: "raven_query"
// - Registry command IDs: "query", "schema_add_type"
// - CLI-style names: "schema add type"
func ResolveToolCommandID(toolName string) (string, bool) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", false
	}

	if commandID, ok := compatibilityToolCommandAliases[toolName]; ok {
		return commandID, true
	}

	candidates := []string{toolName}
	if strings.HasPrefix(toolName, "raven_") {
		candidates = append(candidates, strings.TrimPrefix(toolName, "raven_"))
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		if _, ok := Registry[candidate]; ok {
			return candidate, true
		}

		underscored := strings.ReplaceAll(candidate, " ", "_")
		if _, ok := Registry[underscored]; ok {
			return underscored, true
		}
	}

	return "", false
}
