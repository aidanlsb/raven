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
