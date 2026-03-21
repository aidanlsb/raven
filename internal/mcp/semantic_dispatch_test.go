package mcp

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestAllInvokableCommandsHaveSemanticHandlers(t *testing.T) {
	for commandID, meta := range commands.Registry {
		if meta.HideFromMCP || !commands.IsInvokableCommandID(commandID) {
			continue
		}

		op, ok := semanticOpForCommandID(commandID)
		if !ok {
			t.Fatalf("command %q has no semantic handler", commandID)
		}
		if !semanticHandlerExists(op) {
			t.Fatalf("command %q maps to semantic op %q without a handler", commandID, op)
		}
	}
}

func TestCompatibilityAliasesResolveToHandledCommands(t *testing.T) {
	for toolName, commandID := range compatibilityToolCommandAliases {
		op, ok := semanticOpForCommandID(commandID)
		if !ok {
			t.Fatalf("compatibility alias %q resolves to unhandled command %q", toolName, commandID)
		}
		if !semanticHandlerExists(op) {
			t.Fatalf("compatibility alias %q maps to semantic op %q without a handler", toolName, op)
		}
	}
}

func TestCallToolDirectUnknownTool(t *testing.T) {
	server := NewServer("")

	out, isErr, handled := server.callToolDirect("raven_not_real", nil)
	if handled {
		t.Fatalf("expected unknown tool to be unhandled")
	}
	if isErr {
		t.Fatalf("expected unknown tool to be non-error when unhandled")
	}
	if out != "" {
		t.Fatalf("expected empty output for unknown tool, got %q", out)
	}
}
