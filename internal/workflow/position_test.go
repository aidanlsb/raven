package workflow

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestResolveInsertIndex(t *testing.T) {
	steps := []*config.WorkflowStep{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}

	t.Run("default append", func(t *testing.T) {
		got, err := ResolveInsertIndex(steps, PositionHint{})
		if err != nil {
			t.Fatalf("ResolveInsertIndex returned error: %v", err)
		}
		if got != 3 {
			t.Fatalf("index = %d, want 3", got)
		}
	})

	t.Run("before id", func(t *testing.T) {
		got, err := ResolveInsertIndex(steps, PositionHint{BeforeStepID: "b"})
		if err != nil {
			t.Fatalf("ResolveInsertIndex returned error: %v", err)
		}
		if got != 1 {
			t.Fatalf("index = %d, want 1", got)
		}
	})

	t.Run("after id", func(t *testing.T) {
		got, err := ResolveInsertIndex(steps, PositionHint{AfterStepID: "b"})
		if err != nil {
			t.Fatalf("ResolveInsertIndex returned error: %v", err)
		}
		if got != 2 {
			t.Fatalf("index = %d, want 2", got)
		}
	})

	t.Run("index", func(t *testing.T) {
		idx := 0
		got, err := ResolveInsertIndex(steps, PositionHint{Index: &idx})
		if err != nil {
			t.Fatalf("ResolveInsertIndex returned error: %v", err)
		}
		if got != 0 {
			t.Fatalf("index = %d, want 0", got)
		}
	})

	t.Run("invalid before and after", func(t *testing.T) {
		_, err := ResolveInsertIndex(steps, PositionHint{BeforeStepID: "a", AfterStepID: "b"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("before missing", func(t *testing.T) {
		_, err := ResolveInsertIndex(steps, PositionHint{BeforeStepID: "z"})
		if err == nil {
			t.Fatal("expected error")
		}
		de, ok := AsDomainError(err)
		if !ok || de.Code != CodeRefNotFound {
			t.Fatalf("expected CodeRefNotFound, got %#v", err)
		}
	})
}
