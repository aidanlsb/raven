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

func defaultCategoryForCommandID(commandID string) Category {
	commandID = strings.ReplaceAll(commandID, " ", "_")
	switch {
	case commandID == "query" || commandID == "query_saved_list" || commandID == "query_saved_get" ||
		commandID == "query_saved_set" || commandID == "query_saved_remove" ||
		commandID == "search" || commandID == "backlinks" || commandID == "outlinks" || commandID == "resolve":
		return CategoryQuery
	case commandID == "new" || commandID == "add" || commandID == "upsert" || commandID == "set" ||
		commandID == "delete" || commandID == "move" || commandID == "reclassify" || commandID == "import" ||
		commandID == "edit" || commandID == "update":
		return CategoryContent
	case commandID == "schema" || strings.HasPrefix(commandID, "schema_") || commandID == "template" || strings.HasPrefix(commandID, "template_"):
		return CategorySchema
	case commandID == "read" || commandID == "open" || commandID == "daily" || commandID == "date":
		return CategoryNavigation
	case commandID == "check" || commandID == "reindex" || commandID == "version":
		return CategoryMaintenance
	default:
		return CategoryVault
	}
}

func defaultAccessForCommandID(commandID string) AccessMode {
	commandID = strings.ReplaceAll(commandID, " ", "_")
	switch commandID {
	case "read", "search", "backlinks", "outlinks", "resolve", "query", "query_saved_list", "query_saved_get",
		"schema", "schema_validate", "schema_template_list", "schema_template_get",
		"docs", "docs_list", "docs_search",
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
	if commandID == "schema_rename_field" || commandID == "schema_rename_type" {
		return RiskDestructive
	}
	return RiskMutating
}
