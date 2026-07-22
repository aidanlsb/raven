package model

import (
	"encoding/json"
	"testing"
)

func TestTraitMarshalJSONPreservesParentObjectID(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Trait{
		TraitType:     "todo",
		Content:       "Ship scope cleanup",
		ParentScopeID: "project/raven#tasks",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := result["parent_object_id"]; got != "project/raven#tasks" {
		t.Errorf("parent_object_id = %#v, want project/raven#tasks", got)
	}
	if _, ok := result["parent_scope_id"]; ok {
		t.Error("parent_scope_id must not be added to the compatibility JSON")
	}
}
