package cli

import (
	"strings"
	"testing"
)

func TestIsEmbeddedID(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"people/freya", false},
		{"projects/website", false},
		{"daily/2026-01-07", false},
		{"daily/2026-01-07#standup", true},
		{"projects/website#tasks", true},
		{"meetings/team-sync#action-items", true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := IsEmbeddedID(tt.id)
			if got != tt.expected {
				t.Errorf("IsEmbeddedID(%q) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}
}

func TestBuildEmbeddedSkipWarning(t *testing.T) {
	t.Run("no embedded IDs", func(t *testing.T) {
		w := BuildEmbeddedSkipWarning(nil)
		if w != nil {
			t.Error("expected nil warning for empty list")
		}
	})

	t.Run("with embedded IDs", func(t *testing.T) {
		embedded := []string{"daily/2026-01-07#standup", "projects/website#tasks"}
		w := BuildEmbeddedSkipWarning(embedded)
		if w == nil {
			t.Fatal("expected warning, got nil")
		}
		if w.Code != WarnEmbeddedSkipped {
			t.Errorf("code = %q, want %q", w.Code, WarnEmbeddedSkipped)
		}
		if !strings.Contains(w.Message, "2 embedded object") {
			t.Errorf("message should mention count: %q", w.Message)
		}
		if !strings.Contains(w.Ref, "daily/2026-01-07#standup") {
			t.Errorf("ref should contain IDs: %q", w.Ref)
		}
	})
}

func TestGetActionVerb(t *testing.T) {
	tests := []struct {
		action   string
		expected string
	}{
		{"set", "modified"},
		{"delete", "deleted"},
		{"add", "updated"},
		{"move", "moved"},
		{"unknown", "processed"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got := getActionVerb(tt.action)
			if got != tt.expected {
				t.Errorf("getActionVerb(%q) = %q, want %q", tt.action, got, tt.expected)
			}
		})
	}
}
