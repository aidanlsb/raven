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

func TestBuildCreateObjectCommand(t *testing.T) {
	t.Run("uses rvn new with quoted title", func(t *testing.T) {
		got := buildCreateObjectCommand("project", "projects/raven")
		want := `rvn new project "raven" --json`
		if got != want {
			t.Fatalf("buildCreateObjectCommand() = %q, want %q", got, want)
		}
	})

	t.Run("falls back when title is empty", func(t *testing.T) {
		got := buildCreateObjectCommand("note", "")
		want := `rvn new note "new-object" --json`
		if got != want {
			t.Fatalf("buildCreateObjectCommand() = %q, want %q", got, want)
		}
	})
}
