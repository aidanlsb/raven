package commands

// Policy defines execution/discovery behavior for a command.
//
// These defaults are intentionally permissive for canonical leaf commands, with
// explicit deny overrides for runtime/bootstrap paths.
type Policy struct {
	Invokable    bool
	Discoverable bool
}

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

func IsDiscoverableCommandID(commandID string) bool {
	return PolicyForCommandID(commandID).Discoverable
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
