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

func TestParseHeadingTextFromSpec(t *testing.T) {
	t.Run("accepts markdown heading line", func(t *testing.T) {
		got, ok := parseHeadingTextFromSpec("### Bugs / Fixes")
		if !ok {
			t.Fatal("expected heading to parse")
		}
		if got != "Bugs / Fixes" {
			t.Fatalf("parseHeadingTextFromSpec() = %q, want %q", got, "Bugs / Fixes")
		}
	})

	t.Run("does not treat fragment as heading", func(t *testing.T) {
		if _, ok := parseHeadingTextFromSpec("#bugs-fixes"); ok {
			t.Fatal("expected fragment to not parse as markdown heading")
		}
	})
}
