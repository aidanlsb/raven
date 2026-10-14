package localapi

import "github.com/aidanlsb/raven/internal/commands"

// MVP write commands allowed on the local HTTP API. Everything else that is
// not in allowedReadCommands is rejected with COMMAND_NOT_ALLOWED.
var allowedWriteCommands = map[string]struct{}{
	"set":   {},
	"unset": {},
	"new":   {},
	"move":  {},
}

// Read-only vault commands for the Perch MVP. The set is explicit: AccessRead
// in the registry includes global surfaces (docs, config, vault list) that do
// not belong on a vault-scoped companion API.
var allowedReadCommands = map[string]struct{}{
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
}

// CommandAllowed reports whether commandID may be invoked through POST /v1/invoke.
func CommandAllowed(commandID string) bool {
	if _, ok := allowedWriteCommands[commandID]; ok {
		return true
	}
	if _, ok := allowedReadCommands[commandID]; ok {
		return true
	}
	return false
}

func allowedCommandIDs() []string {
	ids := make([]string, 0, len(allowedReadCommands)+len(allowedWriteCommands))
	for id := range allowedReadCommands {
		if commands.IsInvokableCommandID(id) {
			ids = append(ids, id)
		}
	}
	for id := range allowedWriteCommands {
		if commands.IsInvokableCommandID(id) {
			ids = append(ids, id)
		}
	}
	return ids
}
