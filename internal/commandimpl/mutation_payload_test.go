package commandimpl

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
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
	requirePayloadType[commandpayload.NewResult](t, result)

	result = HandleUpsert(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"type":  "project",
			"title": "Typed Payload",
		},
	})
	requirePayloadType[commandpayload.UpsertResult](t, result)

	result = HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"to":   "projects/typed-payload",
			"text": "typed body",
		},
	})
	requirePayloadType[commandpayload.AddResult](t, result)

	result = HandleSet(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"reference": "projects/typed-payload",
			"fields":    []string{"status=active"},
		},
	})
	requirePayloadType[commandpayload.SetResult](t, result)

	result = HandleUnset(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"reference": "projects/typed-payload",
			"fields":    []string{"status"},
		},
	})
	requirePayloadType[commandpayload.UnsetResult](t, result)

	result = HandleEdit(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"reference": "projects/typed-payload",
			"old_str":   "typed body",
			"new_str":   "updated body",
		},
	})
	requirePayloadType[commandpayload.EditSingleResult](t, result)

	result = HandleMove(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Preview:   true,
		Args: map[string]any{
			"source":      "projects/typed-payload",
			"destination": "archive/typed-payload",
		},
	})
	requirePayloadType[commandpayload.MoveResult](t, result)

	result = HandleDelete(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Preview:   true,
		Args: map[string]any{
			"reference": "projects/typed-payload",
		},
	})
	requirePayloadType[commandpayload.DeletePreviewResult](t, result)
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
