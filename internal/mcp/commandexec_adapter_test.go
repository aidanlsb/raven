package mcp

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

func TestNormalizeCanonicalArgsUpdateTraitIDs(t *testing.T) {
	args := map[string]interface{}{
		"stdin":     true,
		"trait_ids": []interface{}{"tasks/task1.md:trait:0"},
		"value":     "done",
	}

	got := normalizeCanonicalArgs("update", args)

	rawIDs, ok := got["object_ids"].([]interface{})
	if !ok {
		t.Fatalf("object_ids missing or wrong type: %#v", got["object_ids"])
	}
	if len(rawIDs) != 1 || rawIDs[0] != "tasks/task1.md:trait:0" {
		t.Fatalf("object_ids=%#v, want trait IDs copied through", rawIDs)
	}
}

func TestNormalizeCanonicalArgsLeavesOtherCommandsUntouched(t *testing.T) {
	args := map[string]interface{}{"object_ids": []interface{}{"people/freya"}}
	got := normalizeCanonicalArgs("set", args)
	if got["object_ids"] == nil {
		t.Fatalf("expected object_ids to remain present: %#v", got)
	}
}

func TestAdaptCanonicalResultForMCPRewritesValidationSuggestion(t *testing.T) {
	result := adaptCanonicalResultForMCP("add", commandexec.Failure("INVALID_ARGS", "argument validation failed", nil, "Check command arguments and retry"))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Call raven_describe with command 'add' for the strict contract and retry"
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestAdaptCanonicalResultForMCPAddsQuerySuggestionWhenMissing(t *testing.T) {
	result := adaptCanonicalResultForMCP("query", commandexec.Failure("QUERY_INVALID", "parse error: expected 2, got 1 at pos 5", nil, ""))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Check the query syntax, quote string literals, and retry."
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestCommandVaultRequirementsFollowRegistryMetadata(t *testing.T) {
	t.Parallel()

	if commands.RequiresVault("version") {
		t.Fatal("version should not require vault resolution")
	}
	if commands.RequiresVault("config_show") {
		t.Fatal("config_show should not require vault resolution")
	}
	if !commands.RequiresVault("query") {
		t.Fatal("query should require vault resolution")
	}
}
