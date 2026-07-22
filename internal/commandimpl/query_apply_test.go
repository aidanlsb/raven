package commandimpl

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/readsvc"
)

func TestHandleQueryApplyPreservesNestedMutationMetadata(t *testing.T) {
	t.Parallel()

	registry := commandexec.NewHandlerRegistry()
	var nestedReq commandexec.Request
	registry.Register("set", func(_ context.Context, req commandexec.Request) commandexec.Result {
		nestedReq = req
		return commandexec.Success(map[string]interface{}{"total": 1}, &commandexec.Meta{Count: 1}).
			WithMutationPhase(commandexec.MutationPhasePreview)
	})
	registry.Register("query-test", func(ctx context.Context, req commandexec.Request) commandexec.Result {
		return handleQueryApply(ctx, req, &readsvc.ExecuteQueryResult{
			QueryKind: "type",
			Objects:   []model.Object{{ID: "projects/raven"}},
		}, []string{"set status=done"}, 42)
	})

	result := commandexec.NewInvoker(registry, nil).Execute(context.Background(), commandexec.Request{
		CommandID:      "query-test",
		VaultPath:      "/vault",
		ConfigPath:     "/config",
		StatePath:      "/state",
		ExecutablePath: "/bin/rvn",
		Caller:         commandexec.CallerMCP,
	})

	if !result.OK {
		t.Fatalf("handleQueryApply() failed: %#v", result.Error)
	}
	if result.Meta == nil || result.Meta.Count != 1 || result.Meta.QueryTimeMs != 42 {
		t.Fatalf("meta = %#v, want nested count and query timing", result.Meta)
	}
	if result.Meta.Mutation == nil || result.Meta.Mutation.Phase != commandexec.MutationPhasePreview {
		t.Fatalf("mutation = %#v, want preview", result.Meta.Mutation)
	}
	if nestedReq.CommandID != "set" ||
		nestedReq.VaultPath != "/vault" ||
		nestedReq.ConfigPath != "/config" ||
		nestedReq.StatePath != "/state" ||
		nestedReq.ExecutablePath != "/bin/rvn" ||
		nestedReq.Caller != commandexec.CallerMCP {
		t.Fatalf("nested request context was not preserved: %#v", nestedReq)
	}
}

func TestHandleQueryApplyEmptyResultSetsMutationPhase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		confirm   bool
		wantPhase commandexec.MutationPhase
	}{
		{name: "preview", wantPhase: commandexec.MutationPhasePreview},
		{name: "applied", confirm: true, wantPhase: commandexec.MutationPhaseApplied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := handleQueryApply(
				context.Background(),
				commandexec.Request{Confirm: tt.confirm},
				&readsvc.ExecuteQueryResult{QueryKind: "type"},
				[]string{"delete"},
				42,
			)

			if !result.OK {
				t.Fatalf("handleQueryApply() failed: %#v", result.Error)
			}
			if result.Meta == nil || result.Meta.QueryTimeMs != 42 {
				t.Fatalf("meta = %#v, want query timing", result.Meta)
			}
			if result.Meta.Mutation == nil || result.Meta.Mutation.Phase != tt.wantPhase {
				t.Fatalf("mutation = %#v, want %q", result.Meta.Mutation, tt.wantPhase)
			}
		})
	}
}
