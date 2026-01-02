package resolver

import (
	"testing"
)

func TestResolver(t *testing.T) {
	objectIDs := []string{
		"people/freya",
		"people/thor",
		"projects/bifrost",
		"daily/2025-02-01",
		"daily/2025-02-01#standup",
	}

	r := New(objectIDs)

	t.Run("resolve full path", func(t *testing.T) {
		result := r.Resolve("people/freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
		if result.Ambiguous {
			t.Error("expected not ambiguous")
		}
	})

	t.Run("resolve short name", func(t *testing.T) {
		result := r.Resolve("bifrost")
		if result.TargetID != "projects/bifrost" {
			t.Errorf("got %q, want %q", result.TargetID, "projects/bifrost")
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
		"people/freya",
		"clients/freya",
	}

	r := New(objectIDs)

	result := r.Resolve("freya")
	if !result.Ambiguous {
		t.Error("expected ambiguous")
	}
	if len(result.Matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(result.Matches))
	}
}

func TestResolverDateShorthand(t *testing.T) {
	objectIDs := []string{
		"daily/2025-02-01",
		"people/freya",
	}

	r := NewWithDailyDir(objectIDs, "daily")

	t.Run("date reference to existing daily note", func(t *testing.T) {
		result := r.Resolve("2025-02-01")
		if result.TargetID != "daily/2025-02-01" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-02-01")
		}
	})

	t.Run("date reference to non-existent daily note", func(t *testing.T) {
		// Date references should resolve even if the daily note doesn't exist
		result := r.Resolve("2025-03-15")
		if result.TargetID != "daily/2025-03-15" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-03-15")
		}
	})

	t.Run("custom daily directory", func(t *testing.T) {
		r2 := NewWithDailyDir([]string{"journal/2025-02-01"}, "journal")
		result := r2.Resolve("2025-02-01")
		if result.TargetID != "journal/2025-02-01" {
			t.Errorf("got %q, want %q", result.TargetID, "journal/2025-02-01")
		}
	})

	t.Run("non-date string not treated as date", func(t *testing.T) {
		result := r.Resolve("freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}

func TestResolverSlugifiedMatching(t *testing.T) {
	// Files are stored with slugified names
	objectIDs := []string{
		"people/sif",
		"people/freya",
		"projects/my-awesome-project",
	}

	r := New(objectIDs)

	t.Run("proper noun resolves to slugified file", func(t *testing.T) {
		// User writes [[people/Sif]] but file is sif.md
		result := r.Resolve("people/Sif")
		if result.TargetID != "people/sif" {
			t.Errorf("got %q, want %q", result.TargetID, "people/sif")
		}
	})

	t.Run("mixed case resolves to slugified file", func(t *testing.T) {
		result := r.Resolve("people/SIF")
		if result.TargetID != "people/sif" {
			t.Errorf("got %q, want %q", result.TargetID, "people/sif")
		}
	})

	t.Run("spaces and caps in project name", func(t *testing.T) {
		result := r.Resolve("projects/My Awesome Project")
		if result.TargetID != "projects/my-awesome-project" {
			t.Errorf("got %q, want %q", result.TargetID, "projects/my-awesome-project")
		}
	})

	t.Run("short name with spaces resolves", func(t *testing.T) {
		result := r.Resolve("Sif")
		if result.TargetID != "people/sif" {
			t.Errorf("got %q, want %q", result.TargetID, "people/sif")
		}
	})

	t.Run("exact match still works", func(t *testing.T) {
		result := r.Resolve("people/freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}
