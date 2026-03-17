package mcp

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestAllInvokableCommandsHaveDirectSemanticMapping(t *testing.T) {
	missing := make([]string, 0)
	for commandID, meta := range commands.Registry {
		if meta.HideFromMCP || !commands.IsInvokableCommandID(commandID) {
			continue
		}
		toolName := mcpToolName(commandID)
		if _, ok := compatibilityToolSemanticMap[toolName]; !ok {
			missing = append(missing, commandID)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("invokable commands missing direct semantic mappings: %v", missing)
	}
}
