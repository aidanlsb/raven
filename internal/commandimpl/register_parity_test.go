package commandimpl

import (
	"sort"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

func TestRegisterAllMatchesInvokableRegistryCommands(t *testing.T) {
	t.Parallel()

	registry := commandexec.NewHandlerRegistry()
	RegisterAll(registry)

	registered := registry.Handlers()
	expected := make(map[string]struct{})
	for commandID := range commands.Registry {
		if !commands.IsInvokableCommandID(commandID) {
			continue
		}
		expected[commandID] = struct{}{}
	}

	var missing []string
	for commandID := range expected {
		if _, ok := registered[commandID]; !ok {
			missing = append(missing, commandID)
		}
	}

	var extra []string
	for commandID, handler := range registered {
		if handler == nil {
			t.Fatalf("registered handler %q is nil", commandID)
		}
		if _, ok := expected[commandID]; !ok {
			extra = append(extra, commandID)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("handler registration drift: missing=%v extra=%v", missing, extra)
	}
}
