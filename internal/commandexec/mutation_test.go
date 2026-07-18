package commandexec

import (
	"context"
	"testing"
)

func TestWithMutationPhaseSetsPhaseOnSuccess(t *testing.T) {
	t.Parallel()

	result := Success(map[string]any{"ok": true}, nil).WithMutationPhase(MutationPhaseApplied)
	if result.Meta == nil || result.Meta.Mutation == nil {
		t.Fatalf("meta.mutation missing: %#v", result.Meta)
	}
	if got := result.Meta.Mutation.Phase; got != MutationPhaseApplied {
		t.Fatalf("phase = %q, want %q", got, MutationPhaseApplied)
	}
}

func TestWithMutationPhasePreservesExistingMeta(t *testing.T) {
	t.Parallel()

	result := Success("data", &Meta{Count: 3}).WithMutationPhase(MutationPhasePreview)
	if result.Meta == nil {
		t.Fatal("meta unexpectedly nil")
	}
	if result.Meta.Count != 3 {
		t.Fatalf("count = %d, want 3", result.Meta.Count)
	}
	if result.Meta.Mutation == nil || result.Meta.Mutation.Phase != MutationPhasePreview {
		t.Fatalf("mutation = %#v, want preview", result.Meta.Mutation)
	}
}

func TestWithMutationPhaseNoOpOnFailure(t *testing.T) {
	t.Parallel()

	result := Failure("BOOM", "nope", nil, "").WithMutationPhase(MutationPhaseApplied)
	if result.Meta != nil {
		t.Fatalf("failure envelope gained meta: %#v", result.Meta)
	}
}

func TestInvokerRunsResultAnnotator(t *testing.T) {
	t.Parallel()

	registry := NewHandlerRegistry()
	registry.Register("mutating", func(_ context.Context, _ Request) Result {
		return Success(map[string]any{"ok": true}, nil)
	})
	registry.Register("explicit", func(_ context.Context, _ Request) Result {
		return Success(map[string]any{"ok": true}, nil).WithMutationPhase(MutationPhasePreview)
	})

	invoker := NewInvoker(registry, nil).WithResultAnnotator(func(_ context.Context, _ Request, result Result) Result {
		if !result.OK {
			return result
		}
		if result.Meta != nil && result.Meta.Mutation != nil {
			return result
		}
		return result.WithMutationPhase(MutationPhaseApplied)
	})

	annotated := invoker.Execute(context.Background(), Request{CommandID: "mutating"})
	if annotated.Meta == nil || annotated.Meta.Mutation == nil || annotated.Meta.Mutation.Phase != MutationPhaseApplied {
		t.Fatalf("annotator did not attach phase: %#v", annotated.Meta)
	}

	// An explicit handler phase must not be clobbered by the annotator.
	explicit := invoker.Execute(context.Background(), Request{CommandID: "explicit"})
	if explicit.Meta == nil || explicit.Meta.Mutation == nil || explicit.Meta.Mutation.Phase != MutationPhasePreview {
		t.Fatalf("explicit handler phase changed: %#v", explicit.Meta)
	}
}
