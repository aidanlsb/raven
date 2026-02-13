package ui

import "testing"

func TestNormalizeAccentColor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		ok       bool
	}{
		{name: "empty", input: "", expected: "", ok: false},
		{name: "none", input: "none", expected: "", ok: false},
		{name: "off", input: "off", expected: "", ok: false},
		{name: "default", input: "default", expected: "", ok: false},
		{name: "ansi code", input: "39", expected: "39", ok: true},
		{name: "ansi with whitespace", input: "  244 ", expected: "244", ok: true},
		{name: "ansi out of range", input: "256", expected: "", ok: false},
		{name: "negative ansi", input: "-1", expected: "", ok: false},
		{name: "hex 6", input: "#7aa2f7", expected: "#7aa2f7", ok: true},
		{name: "hex 3", input: "#abc", expected: "#aabbcc", ok: true},
		{name: "bad hex", input: "#zzzzzz", expected: "", ok: false},
		{name: "bad string", input: "blue", expected: "", ok: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeAccentColor(tt.input)
			if ok != tt.ok {
				t.Fatalf("expected ok=%v, got %v", tt.ok, ok)
			}
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestConfigureThemeAccentColor(t *testing.T) {
	origAccent := Accent
	origAccentColor := accentColor
	t.Cleanup(func() {
		Accent = origAccent
		accentColor = origAccentColor
	})

	ConfigureTheme("39")
	got, ok := AccentColor()
	if !ok {
		t.Fatalf("expected accent color to be configured")
	}
	if got != "39" {
		t.Fatalf("expected accent color '39', got %q", got)
	}

	ConfigureTheme("none")
	_, ok = AccentColor()
	if ok {
		t.Fatalf("expected accent color to be disabled")
	}
}
