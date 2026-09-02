package commands

import "strings"

func init() {
	normalizeRegistryMetadata()
}

func normalizeRegistryMetadata() {
	for commandID, meta := range Registry {
		if meta.Category == "" {
			meta.Category = defaultCategoryForCommandID(commandID)
		}
		if meta.Access == "" {
			meta.Access = defaultAccessForCommandID(commandID)
		}
		if meta.Risk == "" {
			meta.Risk = defaultRiskForCommandID(commandID, meta.Access)
		}
		if meta.VaultScope == "" {
			meta.VaultScope = VaultScopeRequired
		}
		Registry[commandID] = meta
	}
}

// CLIPathSegments returns the CLI invocation path for the command as ordered
// segments. It prefers the explicit CLIPath hierarchy field and falls back to
// splitting the human-facing Name so existing entries keep working without
// requiring the field to be populated.
func (m Meta) CLIPathSegments() []string {
	if len(m.CLIPath) > 0 {
		out := make([]string, len(m.CLIPath))
		copy(out, m.CLIPath)
		return out
	}
	return strings.Fields(m.Name)
}

func EffectiveMeta(commandID string) (Meta, bool) {
	meta, ok := Registry[commandID]
	if !ok {
		return Meta{}, false
	}
	if meta.Category == "" {
		meta.Category = defaultCategoryForCommandID(commandID)
	}
	if meta.Access == "" {
		meta.Access = defaultAccessForCommandID(commandID)
	}
	if meta.Risk == "" {
		meta.Risk = defaultRiskForCommandID(commandID, meta.Access)
	}
	if meta.VaultScope == "" {
		meta.VaultScope = VaultScopeRequired
	}
	return meta, true
}

func RequiresVault(commandID string) bool {
	meta, ok := EffectiveMeta(commandID)
	if !ok {
		return true
	}
	return meta.VaultScope != VaultScopeNone
}

// RequiresVaultForInvocation adds argument-dependent vault requirements to the
// registry's static scope. Most commands use only RequiresVault; vault_list
// remains available without a configured vault for diagnostics, while its
// path-only mode resolves and validates the selected vault like other
// vault-scoped commands.
func RequiresVaultForInvocation(commandID string, args map[string]interface{}) bool {
	if commandID == "vault_list" {
		pathOnly, _ := args["path-only"].(bool)
		if pathOnly {
			return true
		}
	}
	return RequiresVault(commandID)
}

func defaultCategoryForCommandID(commandID string) Category {
	commandID = strings.ReplaceAll(commandID, " ", "_")
	switch commandID {
	case "query", "query_saved_list", "query_saved_get", "query_saved_set", "query_saved_remove",
		"search", "backlinks", "outlinks", "resolve":
		return CategoryQuery
	case "new", "add", "upsert", "set", "unset",
		"delete", "move", "reclassify", "import",
		"edit", "update":
		return CategoryContent
	case "read", "open", "daily", "date":
		return CategoryNavigation
	case "check", "reindex", "version":
		return CategoryMaintenance
	}
	if commandID == "schema" || strings.HasPrefix(commandID, "schema_") ||
		commandID == "template" || strings.HasPrefix(commandID, "template_") {
		return CategorySchema
	}
	return CategoryVault
}

func defaultAccessForCommandID(commandID string) AccessMode {
	commandID = strings.ReplaceAll(commandID, " ", "_")
	switch commandID {
	case "read", "search", "backlinks", "outlinks", "resolve", "query", "query_saved_list", "query_saved_get",
		"schema", "schema_validate", "schema_template_list", "schema_template_get",
		"docs", "docs_search",
		"version",
		"vault", "vault_list", "vault_current", "vault_path", "vault_stats",
		"config", "config_show":
		return AccessRead
	default:
		return AccessWrite
	}
}

func defaultRiskForCommandID(commandID string, access AccessMode) RiskLevel {
	commandID = strings.ReplaceAll(commandID, " ", "_")
	if access == AccessRead {
		return RiskSafe
	}
	if commandID == "delete" || commandID == "move" || commandID == "reclassify" {
		return RiskDestructive
	}
	if strings.Contains(commandID, "remove") || strings.Contains(commandID, "delete") {
		return RiskDestructive
	}
	if commandID == "schema_rename_field" || commandID == "schema_rename_type" ||
		commandID == "schema_convert_field" || commandID == "schema_convert_trait" {
		return RiskDestructive
	}
	return RiskMutating
}
