package commands

import "testing"

func TestEmitsMutationPhaseCoversPreviewCommands(t *testing.T) {
	t.Parallel()

	// Every command with an explicit preview policy is a mutation and must
	// report a phase, except plain `query`: its read path carries no phase, and
	// its `apply` path delegates to a nested mutating command that does.
	for commandID := range previewModeByCommandID {
		if commandID == "query" {
			if EmitsMutationPhase(commandID) {
				t.Fatalf("plain query should not emit a mutation phase")
			}
			continue
		}
		if !EmitsMutationPhase(commandID) {
			t.Fatalf("%s has a preview policy but does not emit a mutation phase", commandID)
		}
	}
}

func TestEmitsMutationPhaseOnlyForRegisteredWriteCommands(t *testing.T) {
	t.Parallel()

	for commandID := range mutationPhaseCommandIDs {
		meta, ok := EffectiveMeta(commandID)
		if !ok {
			t.Fatalf("%s emits a mutation phase but is not in the registry", commandID)
		}
		if meta.Access != AccessWrite {
			t.Fatalf("%s emits a mutation phase but is classified %q, want write", commandID, meta.Access)
		}
	}
}

func TestEmitsMutationPhaseExcludesReadsAndMaintenance(t *testing.T) {
	t.Parallel()

	for _, commandID := range []string{
		"query", "search", "read", "resolve", "backlinks", "outlinks",
		"schema", "schema_validate", "schema_template_list", "schema_template_get",
		"check", "reindex", "version", "vault_list", "config_show",
	} {
		if EmitsMutationPhase(commandID) {
			t.Fatalf("%s should not emit a mutation phase", commandID)
		}
	}
}
