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
	t.Setenv("NO_COLOR", "")

	origAccent := Accent
	origBold := Bold
	origMuted := Muted
	origSyntax := Syntax
	origSyntaxSubtle := SyntaxSubtle
	origAccentColor := accentColor
	t.Cleanup(func() {
		Accent = origAccent
		Bold = origBold
		Muted = origMuted
		Syntax = origSyntax
		SyntaxSubtle = origSyntaxSubtle
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

func TestConfigureThemeHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	origAccent := Accent
	origBold := Bold
	origMuted := Muted
	origSyntax := Syntax
	origSyntaxSubtle := SyntaxSubtle
	origAccentColor := accentColor
	t.Cleanup(func() {
		Accent = origAccent
		Bold = origBold
		Muted = origMuted
		Syntax = origSyntax
		SyntaxSubtle = origSyntaxSubtle
		accentColor = origAccentColor
	})

	ConfigureTheme("39")

	if Accent.Render("value") != "value" {
		t.Fatalf("expected accent style to be a no-op when NO_COLOR is set")
	}
	if Bold.Render("value") != "value" {
		t.Fatalf("expected bold style to be a no-op when NO_COLOR is set")
	}
	if Muted.Render("value") != "value" {
		t.Fatalf("expected muted style to be a no-op when NO_COLOR is set")
	}
	if Syntax.Render("value") != "value" {
		t.Fatalf("expected syntax style to be a no-op when NO_COLOR is set")
	}
	if _, ok := AccentColor(); ok {
		t.Fatalf("expected configured accent color to be ignored when NO_COLOR is set")
	}
}
