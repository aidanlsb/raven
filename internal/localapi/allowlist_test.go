package localapi

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestCommandAllowedMatchesInvokableRegistryPolicy(t *testing.T) {
	t.Parallel()

	for commandID := range commands.Registry {
		got := CommandAllowed(commandID)
		want := commands.IsInvokableCommandID(commandID)
		if got != want {
			t.Errorf("CommandAllowed(%q) = %v, want %v", commandID, got, want)
		}
	}

	allowed := []string{"query", "read", "objects", "set", "unset", "new", "move", "delete", "write", "add", "edit", "reindex", "open", "init", "vault_list"}
	for _, id := range allowed {
		if !CommandAllowed(id) {
			t.Errorf("CommandAllowed(%q) = false, want true", id)
		}
	}

	blocked := []string{"mcp_serve", "api", "lsp", "config", "vault", "template", "mcp_install"}
	for _, id := range blocked {
		if CommandAllowed(id) {
			t.Errorf("CommandAllowed(%q) = true, want false", id)
		}
	}
}
