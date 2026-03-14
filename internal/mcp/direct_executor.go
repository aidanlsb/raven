package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExecuteToolDirect executes a Raven MCP tool using in-process semantic direct
// dispatch only (no CLI subprocess fallback). The returned map is the standard
// Raven JSON envelope.
func ExecuteToolDirect(vaultPath, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	server := &Server{
		vaultPath:  vaultPath,
		executable: resolveExecutablePath(),
	}
	if strings.TrimSpace(vaultPath) != "" {
		server.baseArgs = []string{"--vault-path", vaultPath}
	}

	out, isErr, handled := server.callToolDirect(toolName, args)
	if !handled {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		if isErr {
			return nil, fmt.Errorf("tool '%s' failed", toolName)
		}
		return nil, fmt.Errorf("tool '%s' returned empty response", toolName)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return nil, fmt.Errorf("tool '%s' returned invalid JSON: %w", toolName, err)
	}

	if okValue, present := envelope["ok"]; present {
		if okFlag, ok := okValue.(bool); ok && !okFlag {
			b, _ := json.Marshal(envelope)
			return nil, fmt.Errorf("tool '%s' returned error: %s", toolName, string(b))
		}
	}

	if isErr {
		return nil, fmt.Errorf("tool '%s' failed: %s", toolName, trimmed)
	}

	return envelope, nil
}
