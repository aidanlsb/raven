package commands

// Policy defines execution/discovery behavior for a command.
//
// These defaults are intentionally permissive for canonical leaf commands, with
// explicit deny overrides for runtime/bootstrap paths.
type Policy struct {
	Invokable    bool
	Discoverable bool
}

// PreviewMode describes whether a command previews changes by default.
type PreviewMode string

const (
	PreviewModeNone               PreviewMode = "none"
	PreviewModePreviewDefault     PreviewMode = "preview_default"
	PreviewModeBulkPreviewDefault PreviewMode = "bulk_preview_default"
)

// DefaultPolicy returns the default policy for canonical commands.
func DefaultPolicy() Policy {
	return Policy{
		Invokable:    true,
		Discoverable: true,
	}
}

// PolicyForCommandID resolves effective policy for a canonical registry command ID.
func PolicyForCommandID(commandID string) Policy {
	policy := DefaultPolicy()

	if _, blocked := nonInvokableCommandIDs[commandID]; blocked {
		policy.Invokable = false
		policy.Discoverable = false
	}

	return policy
}

// ResolveToolPolicy resolves a tool name to a command ID and policy.
func ResolveToolPolicy(toolName string) (commandID string, policy Policy, ok bool) {
	commandID, ok = ResolveToolCommandID(toolName)
	if !ok {
		return "", Policy{}, false
	}
	return commandID, PolicyForCommandID(commandID), true
}

func IsInvokableCommandID(commandID string) bool {
	return PolicyForCommandID(commandID).Invokable
}

// PreviewModeForCommandID resolves explicit preview/apply behavior for a command.
func PreviewModeForCommandID(commandID string) PreviewMode {
	if mode, ok := previewModeByCommandID[commandID]; ok {
		return mode
	}
	return PreviewModeNone
}

// ShouldPreviewByDefault reports whether a normalized request should default to
// preview mode when it is not confirmed.
func ShouldPreviewByDefault(commandID string, args map[string]interface{}) bool {
	switch PreviewModeForCommandID(commandID) {
	case PreviewModePreviewDefault:
		return true
	case PreviewModeBulkPreviewDefault:
		return hasBulkPreviewInput(args)
	default:
		return false
	}
}

var nonInvokableCommandIDs = map[string]struct{}{
	"path":        {},
	"serve":       {},
	"lsp":         {},
	"mcp_install": {},
	"mcp_remove":  {},
	"mcp_status":  {},
	"mcp_show":    {},

	"config":   {},
	"vault":    {},
	"template": {},
}

// previewModeByCommandID controls default preview behavior.
//
// Single-object reversible writes (edit, single set/add/update/delete/move)
// apply immediately and only preview when the caller passes `dry-run`; these
// are either absent (PreviewModeNone) or use PreviewModeBulkPreviewDefault,
// which previews only when a bulk input (stdin/object_ids/trait_ids) is
// present. High-blast-radius operations (bulk writes, query --apply, schema
// rename, check fixes, skill install/sync/remove) preview by default and require
// `confirm` to apply.
var previewModeByCommandID = map[string]PreviewMode{
	"add":    PreviewModeBulkPreviewDefault,
	"delete": PreviewModeBulkPreviewDefault,
	"move":   PreviewModeBulkPreviewDefault,
	"set":    PreviewModeBulkPreviewDefault,
	"update": PreviewModeBulkPreviewDefault,

	"check create-missing": PreviewModePreviewDefault,
	"check_fix":            PreviewModePreviewDefault,
	"query":                PreviewModePreviewDefault,
	"schema_convert_field": PreviewModePreviewDefault,
	"schema_convert_trait": PreviewModePreviewDefault,
	"schema_rename_field":  PreviewModePreviewDefault,
	"schema_rename_type":   PreviewModePreviewDefault,
	"skill_install":        PreviewModePreviewDefault,
	"skill_remove":         PreviewModePreviewDefault,
	"skill_sync":           PreviewModePreviewDefault,
}

// mutationPhaseCommandIDs enumerates canonical commands that write durable
// vault data (markdown content, schema.yaml, raven.yaml saved queries, or
// template files) and therefore report a standard meta.mutation.phase signal.
//
// The set intentionally excludes:
//   - read-only and discovery commands,
//   - derived-cache maintenance (reindex),
//   - vault bootstrap (init),
//   - global config (config_*) and the vault registry (vault_*), which mutate
//     ambient client state rather than vault content.
//
// Plain `query` is excluded here even though `query ... apply=...` mutates: the
// apply path delegates to a nested command (set/delete/add/move/update), whose
// result already carries the phase. See HandleQuery.
var mutationPhaseCommandIDs = map[string]struct{}{
	// Content writes.
	"new":            {},
	"upsert":         {},
	"add":            {},
	"set":            {},
	"unset":          {},
	"delete":         {},
	"move":           {},
	"section_rename": {},
	"reclassify":     {},
	"update":         {},
	"edit":           {},
	"import":         {},

	// Check repairs.
	"check_fix":            {},
	"check create-missing": {},

	// Saved queries (raven.yaml).
	"query_saved_set":    {},
	"query_saved_remove": {},

	// Schema (schema.yaml) writes.
	"schema_add_type":         {},
	"schema_add_trait":        {},
	"schema_add_field":        {},
	"schema_update_type":      {},
	"schema_update_trait":     {},
	"schema_update_field":     {},
	"schema_remove_type":      {},
	"schema_remove_trait":     {},
	"schema_remove_field":     {},
	"schema_convert_trait":    {},
	"schema_convert_field":    {},
	"schema_rename_type":      {},
	"schema_rename_field":     {},
	"schema_template_set":     {},
	"schema_template_remove":  {},
	"schema_template_bind":    {},
	"schema_template_unbind":  {},
	"schema_template_default": {},

	// Template files.
	"template_write":  {},
	"template_delete": {},

	// Managed skill files.
	"skill_install": {},
	"skill_sync":    {},
	"skill_remove":  {},
}

// EmitsMutationPhase reports whether a command carries the standard
// meta.mutation.phase signal on successful responses.
func EmitsMutationPhase(commandID string) bool {
	_, ok := mutationPhaseCommandIDs[commandID]
	return ok
}

func hasBulkPreviewInput(args map[string]interface{}) bool {
	if args == nil {
		return false
	}
	if value, ok := args["stdin"].(bool); ok && value {
		return true
	}
	for _, key := range []string{"object_ids", "trait_ids"} {
		if lenInterfaceSlice(args[key]) > 0 {
			return true
		}
	}
	return false
}

func lenInterfaceSlice(raw interface{}) int {
	switch values := raw.(type) {
	case []interface{}:
		return len(values)
	case []string:
		return len(values)
	default:
		return 0
	}
}
