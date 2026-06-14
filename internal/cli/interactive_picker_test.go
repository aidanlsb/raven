package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/picker"
)

func TestInteractivePickerMissingArgSuggestion(t *testing.T) {
	prevStdinTTY := interactiveStdinIsTerminal
	prevStdoutTTY := interactiveStdoutIsTerminal
	t.Cleanup(func() {
		interactiveStdinIsTerminal = prevStdinTTY
		interactiveStdoutIsTerminal = prevStdoutTTY
	})

	t.Run("mentions interactive terminal when unavailable", func(t *testing.T) {
		interactiveStdinIsTerminal = func() bool { return false }
		interactiveStdoutIsTerminal = func() bool { return true }

		suggestion := interactivePickerMissingArgSuggestion("read", "rvn read <reference>")
		if !strings.Contains(suggestion, "interactive terminal") {
			t.Fatalf("expected interactive terminal hint, got %q", suggestion)
		}
		if !strings.Contains(suggestion, "rvn read <reference>") {
			t.Fatalf("expected fallback usage, got %q", suggestion)
		}
	})

	t.Run("uses direct usage hint when interactive", func(t *testing.T) {
		interactiveStdinIsTerminal = func() bool { return true }
		interactiveStdoutIsTerminal = func() bool { return true }

		suggestion := interactivePickerMissingArgSuggestion("open", "rvn open <reference>")
		if strings.Contains(suggestion, "interactive terminal") {
			t.Fatalf("did not expect terminal hint when interactive, got %q", suggestion)
		}
		if !strings.Contains(suggestion, "rvn open <reference>") {
			t.Fatalf("expected fallback usage, got %q", suggestion)
		}
	})
}

func TestPickAmbiguousReference(t *testing.T) {
	prevRun := ravenRunPicker
	t.Cleanup(func() {
		ravenRunPicker = prevRun
	})

	ravenRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if opts.Prompt != "open/ref" {
			t.Fatalf("prompt = %q, want open/ref", opts.Prompt)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		if !strings.Contains(items[0].SearchText, "short_name") {
			t.Fatalf("expected match source in first item search text, got %q", items[0].SearchText)
		}
		return picker.Selection{Item: items[1]}, true, nil
	}

	selected, ok, err := pickAmbiguousReference(
		"freya",
		[]string{"person/freya", "animal/freya"},
		map[string]string{"person/freya": "short_name", "animal/freya": "name_field"},
		"open/ref> ",
	)
	if err != nil {
		t.Fatalf("pickAmbiguousReference() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected selection")
	}
	if selected != "animal/freya" {
		t.Fatalf("selected = %q, want animal/freya", selected)
	}
}

func TestPickAmbiguousReferenceCancelled(t *testing.T) {
	prevRun := ravenRunPicker
	t.Cleanup(func() {
		ravenRunPicker = prevRun
	})

	ravenRunPicker = func(_ []picker.Item, _ picker.Options) (picker.Selection, bool, error) {
		return picker.Selection{}, false, nil
	}

	selected, ok, err := pickAmbiguousReference("freya", []string{"person/freya"}, nil, "open/ref> ")
	if err != nil {
		t.Fatalf("pickAmbiguousReference() error = %v", err)
	}
	if ok || selected != "" {
		t.Fatalf("expected cancelled selection, got selected=%q ok=%v", selected, ok)
	}
}
