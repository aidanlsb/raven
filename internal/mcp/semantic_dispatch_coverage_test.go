package mcp

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestCompatibilityToolResolutionCoversInvokableCommands(t *testing.T) {
	missing := make([]string, 0)
	for commandID, meta := range commands.Registry {
		if meta.HideFromMCP || !commands.IsInvokableCommandID(commandID) {
			continue
		}
		toolName := mcpToolName(commandID)
		resolved, ok := commands.ResolveToolCommandID(toolName)
		if !ok || resolved != commandID {
			missing = append(missing, commandID)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("invokable commands missing compatibility resolution: %v", missing)
	}
}
