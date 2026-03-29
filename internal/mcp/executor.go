package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

// ExecuteToolDirect executes a canonical Raven command through the compact MCP
// invoke path using the in-process command runtime only. The returned map is
// the standard Raven JSON envelope.
func ExecuteToolDirect(vaultPath, commandRef string, args map[string]interface{}) (map[string]interface{}, error) {
	commandID, ok := commands.ResolveCommandID(strings.TrimSpace(commandRef))
	if !ok {
		return nil, fmt.Errorf("unknown command: %s", commandRef)
	}

	server := &Server{
		vaultPath:  vaultPath,
		executable: resolveExecutablePath(),
	}
	if strings.TrimSpace(vaultPath) != "" {
		server.baseArgs = []string{"--vault-path", vaultPath}
	}

	out, isErr, handled := server.callCanonicalCommand(commandID, args, "", "")
	if !handled {
		return nil, fmt.Errorf("command '%s' has no canonical handler", commandID)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		if isErr {
			return nil, fmt.Errorf("command '%s' failed", commandID)
		}
		return nil, fmt.Errorf("command '%s' returned empty response", commandID)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return nil, fmt.Errorf("command '%s' returned invalid JSON: %w", commandID, err)
	}

	if okValue, present := envelope["ok"]; present {
		if okFlag, ok := okValue.(bool); ok && !okFlag {
			b, _ := json.Marshal(envelope)
			return nil, fmt.Errorf("command '%s' returned error: %s", commandID, string(b))
		}
	}

	if isErr {
		return nil, fmt.Errorf("command '%s' failed: %s", commandID, trimmed)
	}

	return envelope, nil
}

// ExecuteWorkflowToolDirect executes a workflow tool step through the canonical
// in-process command runtime while enforcing workflow policy.
func ExecuteWorkflowToolDirect(vaultPath, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	commandID, ok := commands.ResolveToolCommandID(strings.TrimSpace(toolName))
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	if !commands.IsWorkflowAllowedCommandID(commandID) {
		return nil, fmt.Errorf("tool '%s' is not allowed in workflow steps", toolName)
	}

	return ExecuteToolDirect(vaultPath, commandID, args)
}
