package ui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownNormalizesTrailingNewline(t *testing.T) {
	out, err := RenderMarkdown("# Heading", 80)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("expected rendered markdown to end with newline, got %q", out)
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Fatalf("expected single trailing newline, got %q", out)
	}
}

func TestRenderMarkdownDefaultsWidthWhenNonPositive(t *testing.T) {
	out, err := RenderMarkdown("hello", 0)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected non-empty rendered output")
	}
}

func TestConfigureMarkdownStyle(t *testing.T) {
	orig := markdownStyle
	t.Cleanup(func() {
		markdownStyle = orig
	})

	ConfigureMarkdownStyle("dark")
	if markdownStyle != "dark" {
		t.Fatalf("expected markdown style dark, got %q", markdownStyle)
	}

	ConfigureMarkdownStyle("")
	if markdownStyle != "auto" {
		t.Fatalf("expected empty style to normalize to auto, got %q", markdownStyle)
	}
}

func TestRenderMarkdownFallsBackToAutoForInvalidConfiguredStyle(t *testing.T) {
	orig := markdownStyle
	t.Cleanup(func() {
		markdownStyle = orig
	})

	ConfigureMarkdownStyle("definitely-not-a-real-glamour-style")
	out, err := RenderMarkdown("# Heading", 80)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected rendered output")
	}
}

func TestRenderMarkdownHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	origMarkdownStyle := markdownStyle
	origAccent := Accent
	origBold := Bold
	origMuted := Muted
	origSyntax := Syntax
	origSyntaxSubtle := SyntaxSubtle
	t.Cleanup(func() {
		markdownStyle = origMarkdownStyle
		Accent = origAccent
		Bold = origBold
		Muted = origMuted
		Syntax = origSyntax
		SyntaxSubtle = origSyntaxSubtle
	})

	ConfigureStyles()
	ConfigureMarkdownStyle("dark")

	out, err := RenderMarkdown("# Heading\n\n`value`\n\n```go\nfmt.Println(\"hi\")\n```", 80)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected markdown output without ANSI escapes when NO_COLOR is set, got %q", out)
	}
}
