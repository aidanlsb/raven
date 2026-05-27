package cli

import (
	"os/exec"
	"strings"
	"testing"
)

func TestInteractivePickerMissingArgSuggestion(t *testing.T) {
	prevLookPath := fzfLookPath
	t.Cleanup(func() {
		fzfLookPath = prevLookPath
	})

	t.Run("includes install hint when fzf missing", func(t *testing.T) {
		fzfLookPath = func(string) (string, error) {
			return "", exec.ErrNotFound
		}

		suggestion := interactivePickerMissingArgSuggestion("read", "rvn read <reference>")
		if !strings.Contains(suggestion, "Install fzf") {
			t.Fatalf("expected install hint, got %q", suggestion)
		}
		if !strings.Contains(suggestion, "rvn read <reference>") {
			t.Fatalf("expected fallback usage, got %q", suggestion)
		}
	})

	t.Run("uses direct usage hint when fzf installed", func(t *testing.T) {
		fzfLookPath = func(string) (string, error) {
			return "/usr/local/bin/fzf", nil
		}

		suggestion := interactivePickerMissingArgSuggestion("open", "rvn open <reference>")
		if strings.Contains(suggestion, "Install fzf") {
			t.Fatalf("did not expect install hint when fzf is available, got %q", suggestion)
		}
		if !strings.Contains(suggestion, "rvn open <reference>") {
			t.Fatalf("expected fallback usage, got %q", suggestion)
		}
	})
}

func TestPickAmbiguousReferenceWithFZF(t *testing.T) {
	prevRun := fzfRunPicker
	t.Cleanup(func() {
		fzfRunPicker = prevRun
	})

	fzfRunPicker = func(lines []string, opts fzfPickerOptions) (string, bool, error) {
		if opts.Prompt != "open/ref> " {
			t.Fatalf("prompt = %q, want open/ref> ", opts.Prompt)
		}
		if opts.Delimiter != "\t" {
			t.Fatalf("delimiter = %q, want tab", opts.Delimiter)
		}
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}
		if !strings.Contains(lines[0], "short_name") {
			t.Fatalf("expected match source in first line, got %q", lines[0])
		}
		return lines[1], true, nil
	}

	selected, ok, err := pickAmbiguousReferenceWithFZF(
		"freya",
		[]string{"person/freya", "animal/freya"},
		map[string]string{"person/freya": "short_name", "animal/freya": "name_field"},
		"open/ref> ",
	)
	if err != nil {
		t.Fatalf("pickAmbiguousReferenceWithFZF() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected selection")
	}
	if selected != "animal/freya" {
		t.Fatalf("selected = %q, want animal/freya", selected)
	}
}

func TestPickAmbiguousReferenceWithFZFCancelled(t *testing.T) {
	prevRun := fzfRunPicker
	t.Cleanup(func() {
		fzfRunPicker = prevRun
	})

	fzfRunPicker = func(_ []string, _ fzfPickerOptions) (string, bool, error) {
		return "", false, nil
	}

	selected, ok, err := pickAmbiguousReferenceWithFZF("freya", []string{"person/freya"}, nil, "open/ref> ")
	if err != nil {
		t.Fatalf("pickAmbiguousReferenceWithFZF() error = %v", err)
	}
	if ok || selected != "" {
		t.Fatalf("expected cancelled selection, got selected=%q ok=%v", selected, ok)
	}
}
