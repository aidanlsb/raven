package mcp

import (
	"testing"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commands"
)

func TestAllInvokableCommandsHaveCanonicalHandlers(t *testing.T) {
	invoker := app.CommandInvoker()

	for commandID, meta := range commands.Registry {
		if meta.HideFromMCP || !commands.IsInvokableCommandID(commandID) {
			continue
		}

		if _, ok := invoker.Handlers().Lookup(commandID); !ok {
			t.Fatalf("command %q has no canonical handler", commandID)
		}
	}
}

func TestCompatibilityAliasesResolveToCanonicalHandlers(t *testing.T) {
	invoker := app.CommandInvoker()

	for toolName, commandID := range commands.CompatibilityToolCommandAliases() {
		resolved, ok := commands.ResolveToolCommandID(toolName)
		if !ok || resolved != commandID {
			t.Fatalf("compatibility alias %q resolved to %q, want %q", toolName, resolved, commandID)
		}
		if _, ok := invoker.Handlers().Lookup(commandID); !ok {
			t.Fatalf("compatibility alias %q resolves to command %q without a canonical handler", toolName, commandID)
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
