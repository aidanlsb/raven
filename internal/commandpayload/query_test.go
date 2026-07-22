package commandpayload

import (
	"encoding/json"
	"testing"
)

func TestTraitItemMarshalJSONPreservesObjectID(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(TraitItem{ScopeID: "project/raven#tasks"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := result["object_id"]; got != "project/raven#tasks" {
		t.Errorf("object_id = %#v, want project/raven#tasks", got)
	}
	if _, ok := result["scope_id"]; ok {
		t.Error("scope_id must not be added to the compatibility JSON")
	}
}
