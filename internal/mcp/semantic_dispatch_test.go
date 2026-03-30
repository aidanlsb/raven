package mcp

import (
	"testing"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commands"
)

func TestAllInvokableCommandsHaveCanonicalHandlers(t *testing.T) {
	t.Parallel()
	invoker := app.CommandInvoker()

	for commandID := range commands.Registry {
		if !commands.IsInvokableCommandID(commandID) {
			continue
		}

		if _, ok := invoker.Handlers().Lookup(commandID); !ok {
			t.Fatalf("command %q has no canonical handler", commandID)
		}
	}
}
