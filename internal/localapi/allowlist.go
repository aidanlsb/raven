package localapi

import (
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

// AllowedCommandIDs is the Perch MVP invoke surface. Read-only vault commands
// plus a small reversible write set. Everything else is rejected with
// API_COMMAND_DENIED before dispatch.
var AllowedCommandIDs = map[string]struct{}{
	// Read / inspect.
	"query":                {},
	"query_saved_list":     {},
	"query_saved_get":      {},
	"read":                 {},
	"backlinks":            {},
	"outlinks":             {},
	"search":               {},
	"daily":                {},
	"date":                 {},
	"schema":               {},
	"schema_validate":      {},
	"schema_template_list": {},
	"schema_template_get":  {},
	"vault_stats":          {},
	"resolve":              {},
	"check":                {},
	"trash_list":           {},

	// Writes (preview/apply still follow command policy).
	"set":   {},
	"unset": {},
	"new":   {},
	"move":  {},
}

// CommandAllowed reports whether commandID may be invoked through the local API.
func CommandAllowed(commandID string) bool {
	_, ok := AllowedCommandIDs[commandID]
	return ok
}

func allowedCommandIDsSorted() []string {
	ids := make([]string, 0, len(AllowedCommandIDs))
	for commandID := range AllowedCommandIDs {
		if _, ok := commands.EffectiveMeta(commandID); ok {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids
}

// queryApplyCommand extracts the nested mutation command from query `apply`
// arguments. An empty result with ok=false means no apply was requested.
func queryApplyCommand(args map[string]interface{}) (string, bool) {
	if args == nil {
		return "", false
	}
	raw, ok := args["apply"]
	if !ok || raw == nil {
		return "", false
	}
	first := firstApplyEntry(raw)
	if first == "" {
		return "", false
	}
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

func firstApplyEntry(raw interface{}) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		if len(typed) == 0 {
			return ""
		}
		return strings.TrimSpace(typed[0])
	case []interface{}:
		if len(typed) == 0 {
			return ""
		}
		s, _ := typed[0].(string)
		return strings.TrimSpace(s)
	default:
		return ""
	}
}
