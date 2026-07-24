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

func TestWithAttemptedIDsAddsOrderedInputsAndPreservesDetails(t *testing.T) {
	t.Parallel()

	originalDetails := map[string]interface{}{"field": "done"}
	ids := []string{"tasks/b.md:trait:1", "tasks/a.md:trait:0"}
	result := Failure("VALIDATION_FAILED", "invalid value", originalDetails, "").
		WithAttemptedIDs("trait_ids", ids)

	details, ok := result.Error.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("details = %#v, want map", result.Error.Details)
	}
	if details["field"] != "done" {
		t.Fatalf("existing details were not preserved: %#v", details)
	}
	gotIDs, ok := details["trait_ids"].([]string)
	if !ok {
		t.Fatalf("trait_ids = %#v, want []string", details["trait_ids"])
	}
	if len(gotIDs) != 2 || gotIDs[0] != ids[0] || gotIDs[1] != ids[1] {
		t.Fatalf("trait_ids = %#v, want %#v", gotIDs, ids)
	}
	if details["total"] != 2 {
		t.Fatalf("total = %#v, want 2", details["total"])
	}

	ids[0] = "changed"
	originalDetails["field"] = "changed"
	if gotIDs[0] != "tasks/b.md:trait:1" || details["field"] != "done" {
		t.Fatalf("result details alias caller data: %#v", details)
	}
}

func TestWithAttemptedIDsNoOpOnSuccess(t *testing.T) {
	t.Parallel()

	result := Success("data", nil).WithAttemptedIDs("references", []string{"notes/a"})
	if result.Error != nil {
		t.Fatalf("success envelope gained error: %#v", result.Error)
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
