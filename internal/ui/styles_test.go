package ui

import "testing"

func TestConfigureStylesUsesFixedSemanticStyles(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	origAccent := Accent
	origBold := Bold
	origMuted := Muted
	origSyntax := Syntax
	origSyntaxSubtle := SyntaxSubtle
	t.Cleanup(func() {
		Accent = origAccent
		Bold = origBold
		Muted = origMuted
		Syntax = origSyntax
		SyntaxSubtle = origSyntaxSubtle
	})

	ConfigureStyles()
	if Accent.Render("value") != Bold.Render("value") {
		t.Fatalf("expected fixed accent semantics to match bold style")
	}
}

func TestConfigureStylesHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	origAccent := Accent
	origBold := Bold
	origMuted := Muted
	origSyntax := Syntax
	origSyntaxSubtle := SyntaxSubtle
	t.Cleanup(func() {
		Accent = origAccent
		Bold = origBold
		Muted = origMuted
		Syntax = origSyntax
		SyntaxSubtle = origSyntaxSubtle
	})

	ConfigureStyles()

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
}
