package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExecuteToolDirect executes a canonical Raven command through the compact MCP
// invoke path using the in-process command runtime only and a pinned vault path
// when one is provided. The returned map is the standard Raven JSON envelope.
func ExecuteToolDirect(vaultPath, commandRef string, args map[string]interface{}) (map[string]interface{}, error) {
	server := &Server{
		vaultPath:  vaultPath,
		executable: resolveExecutablePath(),
	}
	if strings.TrimSpace(vaultPath) != "" {
		server.baseArgs = []string{"--vault-path", vaultPath}
	}

	invokeArgs := map[string]interface{}{
		"command": strings.TrimSpace(commandRef),
	}
	if args != nil {
		invokeArgs["args"] = args
	}
	out, isErr := server.callCompactInvoke(invokeArgs)

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		if isErr {
			return nil, fmt.Errorf("command '%s' failed", commandRef)
		}
		return nil, fmt.Errorf("command '%s' returned empty response", commandRef)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return nil, fmt.Errorf("command '%s' returned invalid JSON: %w", commandRef, err)
	}

	if okValue, present := envelope["ok"]; present {
		if okFlag, ok := okValue.(bool); ok && !okFlag {
			b, _ := json.Marshal(envelope)
			return nil, fmt.Errorf("command '%s' returned error: %s", commandRef, string(b))
		}
	}

	if isErr {
		return nil, fmt.Errorf("command '%s' failed: %s", commandRef, trimmed)
	}

	return envelope, nil
}
