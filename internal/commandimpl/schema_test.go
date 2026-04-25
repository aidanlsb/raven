package commandimpl

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestHandleSchemaUpdateFieldAcceptsArrayValues(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()

	result := HandleSchemaUpdateField(context.Background(), commandexec.Request{
		VaultPath: vault.Path,
		Args: map[string]any{
			"type_name":  "project",
			"field_name": "status",
			"values":     []any{"active", "paused", "done", "archived"},
		},
	})
	if !result.OK {
		t.Fatalf("expected success, got error: %#v", result.Error)
	}

	loaded, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	want := []string{"active", "paused", "done", "archived"}
	got := loaded.Types["project"].Fields["status"].Values
	if len(got) != len(want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values = %v, want %v", got, want)
		}
	}
}

func TestHandleSchemaUpdateTraitAcceptsArrayValues(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()

	result := HandleSchemaUpdateTrait(context.Background(), commandexec.Request{
		VaultPath: vault.Path,
		Args: map[string]any{
			"name":   "priority",
			"values": []string{"low", "medium", "high", "critical"},
		},
	})
	if !result.OK {
		t.Fatalf("expected success, got error: %#v", result.Error)
	}

	loaded, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	want := []string{"low", "medium", "high", "critical"}
	got := loaded.Traits["priority"].Values
	if len(got) != len(want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values = %v, want %v", got, want)
		}
	}
}
