package cli

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

func TestBuildSchemaCommandsIncludesOnlySchemaCommands(t *testing.T) {
	cmds := schemasvc.SchemaCommands().Commands

	if _, ok := cmds["schema"]; !ok {
		t.Fatalf("expected schema command to be present")
	}
	if _, ok := cmds["schema_add_type"]; !ok {
		t.Fatalf("expected schema_add_type command to be present")
	}
	if _, ok := cmds["search"]; ok {
		t.Fatalf("expected non-schema command %q to be absent", "search")
	}
	if len(cmds) >= len(commands.Registry) {
		t.Fatalf("expected schema command list to be a filtered subset: got %d of %d", len(cmds), len(commands.Registry))
	}
}
