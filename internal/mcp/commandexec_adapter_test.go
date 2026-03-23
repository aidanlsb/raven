package mcp

import "testing"

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
