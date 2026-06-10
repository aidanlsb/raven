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
	"mcp_install": {},
	"mcp_remove":  {},
	"mcp_status":  {},
	"mcp_show":    {},

	"config":   {},
	"vault":    {},
	"template": {},
}

var previewModeByCommandID = map[string]PreviewMode{
	"add":    PreviewModeBulkPreviewDefault,
	"delete": PreviewModeBulkPreviewDefault,
	"move":   PreviewModeBulkPreviewDefault,
	"set":    PreviewModeBulkPreviewDefault,
	"update": PreviewModeBulkPreviewDefault,

	"check":                PreviewModePreviewDefault,
	"check create-missing": PreviewModePreviewDefault,
	"check_fix":            PreviewModePreviewDefault,
	"edit":                 PreviewModePreviewDefault,
	"query":                PreviewModePreviewDefault,
	"schema_rename_field":  PreviewModePreviewDefault,
	"schema_rename_type":   PreviewModePreviewDefault,
	"skill_remove":         PreviewModePreviewDefault,
	"skill_sync":           PreviewModePreviewDefault,
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
