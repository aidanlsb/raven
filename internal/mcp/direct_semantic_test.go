package mcp

import "testing"

func TestSemanticCompatibilityMapHasHandlers(t *testing.T) {
	for toolName, op := range compatibilityToolSemanticMap {
		if _, ok := semanticToolHandlers[op]; !ok {
			t.Fatalf("tool %q maps to semantic op %q without a handler", toolName, op)
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
