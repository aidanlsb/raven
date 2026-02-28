package commands

import "strings"

// mutatingCommandIDs lists registry command IDs that can mutate vault state.
// This powers lifecycle trigger eligibility checks.
var mutatingCommandIDs = map[string]struct{}{
	"new":                          {},
	"add":                          {},
	"upsert":                       {},
	"delete":                       {},
	"move":                         {},
	"reclassify":                   {},
	"query_add":                    {},
	"query_remove":                 {},
	"schema_add_type":              {},
	"schema_add_trait":             {},
	"schema_add_field":             {},
	"schema_update_type":           {},
	"schema_update_trait":          {},
	"schema_update_field":          {},
	"schema_remove_type":           {},
	"schema_remove_trait":          {},
	"schema_remove_field":          {},
	"schema_rename_type":           {},
	"schema_rename_field":          {},
	"schema_template_set":          {},
	"schema_template_remove":       {},
	"schema_type_template_set":     {},
	"schema_type_template_remove":  {},
	"schema_type_template_default": {},
	"schema_core_template_set":     {},
	"schema_core_template_remove":  {},
	"schema_core_template_default": {},
	"set":                          {},
	"update":                       {},
	"edit":                         {},
	"daily":                        {},
	"workflow_add":                 {},
	"workflow_scaffold":            {},
	"workflow_remove":              {},
	"import":                       {},
}

func init() {
	for id := range mutatingCommandIDs {
		meta, ok := Registry[id]
		if !ok {
			continue
		}
		meta.MutatesVault = true
		Registry[id] = meta
	}
}

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

// LookupMetaByPath resolves a CLI command path and returns the registry metadata.
func LookupMetaByPath(path string) (string, Meta, bool) {
	id, ok := ResolveCommandID(path)
	if !ok {
		return "", Meta{}, false
	}
	meta, ok := Registry[id]
	return id, meta, ok
}
