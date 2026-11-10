package commandimpl

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/editsvc"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestObjectMutationHandlersReturnTypedPayloads(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := HandleNew(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"type":  "project",
			"title": "Typed Payload",
		},
	})
	requirePayloadType[objectsvc.CreateResult](t, result)

	result = HandleWrite(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"type":  "project",
			"title": "Typed Payload",
		},
	})
	requirePayloadType[objectsvc.WriteResult](t, result)

	result = HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"to":   "projects/typed-payload",
			"text": "typed body",
		},
	})
	requirePayloadType[objectsvc.AddResult](t, result)

	result = HandleSet(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"reference": "projects/typed-payload",
			"field": map[string]interface{}{
				"status": "active",
			},
		},
	})
	requirePayloadType[objectsvc.SetByReferenceResult](t, result)

	result = HandleUnset(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"reference": "projects/typed-payload",
			"fields":    []string{"status"},
		},
	})
	requirePayloadType[objectsvc.UnsetByReferenceResult](t, result)

	result = HandleEdit(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"reference": "projects/typed-payload",
			"old_str":   "typed body",
			"new_str":   "updated body",
		},
	})
	requirePayloadType[editsvc.EditSingleResult](t, result)

	result = HandleMove(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Preview:   true,
		Args: map[string]any{
			"source":      "projects/typed-payload",
			"destination": "archive/typed-payload",
		},
	})
	requirePayloadType[objectsvc.MoveResult](t, result)

	result = HandleDelete(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Preview:   true,
		Args: map[string]any{
			"reference": "projects/typed-payload",
		},
	})
	requirePayloadType[objectsvc.DeletePreviewResult](t, result)
}

func requirePayloadType[T any](t *testing.T, result commandexec.Result) T {
	t.Helper()
	if !result.OK {
		t.Fatalf("command failed: %#v", result.Error)
	}
	payload, ok := result.Data.(T)
	if !ok {
		t.Fatalf("Data type = %T, want typed payload", result.Data)
	}
	return payload
}
