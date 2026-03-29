package commands

import "strings"

// Policy defines execution/discovery behavior for a command.
//
// These defaults are intentionally permissive for canonical leaf commands, with
// explicit deny overrides for runtime/bootstrap and workflow-unsafe paths.
type Policy struct {
	Invokable       bool
	Discoverable    bool
	WorkflowAllowed bool
}

// DefaultPolicy returns the default policy for canonical commands.
func DefaultPolicy() Policy {
	return Policy{
		Invokable:       true,
		Discoverable:    true,
		WorkflowAllowed: true,
	}
}

// PolicyForCommandID resolves effective policy for a canonical registry command ID.
func PolicyForCommandID(commandID string) Policy {
	policy := DefaultPolicy()

	if _, blocked := nonInvokableCommandIDs[commandID]; blocked {
		policy.Invokable = false
		policy.Discoverable = false
	}

	if _, blocked := workflowDisallowedExact[commandID]; blocked {
		policy.WorkflowAllowed = false
	} else if hasAnyPrefix(commandID, workflowDisallowedPrefixes) {
		policy.WorkflowAllowed = false
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

func IsWorkflowAllowedCommandID(commandID string) bool {
	return PolicyForCommandID(commandID).WorkflowAllowed
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
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

var workflowDisallowedExact = map[string]struct{}{
	"path":       {},
	"serve":      {},
	"open":       {},
	"init":       {},
	"docs_fetch": {},
	"workflow":   {},
	"config":     {},
	"vault":      {},
}

var workflowDisallowedPrefixes = []string{
	"mcp_",
	"config_",
	"vault_",
	"skill_",
	"workflow_",
}
