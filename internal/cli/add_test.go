package cli

import (
	"testing"
)

func TestFormatCaptureLine(t *testing.T) {
	t.Run("preserves plain text exactly", func(t *testing.T) {
		got := formatCaptureLine("Call Odin")
		if got != "Call Odin" {
			t.Fatalf("formatCaptureLine() = %q, want %q", got, "Call Odin")
		}
	})

	t.Run("preserves leading bullet exactly", func(t *testing.T) {
		got := formatCaptureLine("- [[people/freya]]")
		if got != "- [[people/freya]]" {
			t.Fatalf("formatCaptureLine() = %q, want %q", got, "- [[people/freya]]")
		}
	})
}
