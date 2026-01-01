package resolver

import (
	"testing"
)

func TestResolver(t *testing.T) {
	objectIDs := []string{
		"people/alice",
		"people/bob",
		"projects/website",
		"daily/2025-02-01",
		"daily/2025-02-01#standup",
	}

	r := New(objectIDs)

	t.Run("resolve full path", func(t *testing.T) {
		result := r.Resolve("people/alice")
		if result.TargetID != "people/alice" {
			t.Errorf("got %q, want %q", result.TargetID, "people/alice")
		}
		if result.Ambiguous {
			t.Error("expected not ambiguous")
		}
	})

	t.Run("resolve short name", func(t *testing.T) {
		result := r.Resolve("website")
		if result.TargetID != "projects/website" {
			t.Errorf("got %q, want %q", result.TargetID, "projects/website")
		}
	})

	t.Run("resolve embedded object", func(t *testing.T) {
		result := r.Resolve("daily/2025-02-01#standup")
		if result.TargetID != "daily/2025-02-01#standup" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-02-01#standup")
		}
	})

	t.Run("short name for embedded object", func(t *testing.T) {
		result := r.Resolve("standup")
		if result.TargetID != "daily/2025-02-01#standup" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-02-01#standup")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result := r.Resolve("nonexistent")
		if result.TargetID != "" {
			t.Errorf("expected empty target, got %q", result.TargetID)
		}
		if result.Error == "" {
			t.Error("expected error message")
		}
	})
}

func TestResolverAmbiguous(t *testing.T) {
	// Two objects with the same short name
	objectIDs := []string{
		"people/alice",
		"clients/alice",
	}

	r := New(objectIDs)

	result := r.Resolve("alice")
	if !result.Ambiguous {
		t.Error("expected ambiguous")
	}
	if len(result.Matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(result.Matches))
	}
}
