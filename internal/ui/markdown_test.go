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

func TestRavenMarkdownStyleEmphasizesHeadingsAndSyntax(t *testing.T) {
	style := ravenMarkdownStyle()

	if style.H1.Underline == nil || !*style.H1.Underline {
		t.Fatalf("expected H1 headings to be underlined")
	}
	if style.H2.Underline == nil || !*style.H2.Underline {
		t.Fatalf("expected H2 headings to be underlined")
	}
	if style.Code.Color == nil {
		t.Fatalf("expected inline code to have a color")
	}
	if style.CodeBlock.StylePrimitive.Color == nil {
		t.Fatalf("expected code blocks to have a color")
	}
	if style.CodeBlock.Theme == "" {
		t.Fatalf("expected code blocks to use a syntax theme")
	}
}

func TestConfigureMarkdownCodeTheme(t *testing.T) {
	orig := markdownCodeTheme
	t.Cleanup(func() {
		markdownCodeTheme = orig
	})

	ConfigureMarkdownCodeTheme("dracula")
	if markdownCodeTheme != "dracula" {
		t.Fatalf("expected code theme dracula, got %q", markdownCodeTheme)
	}

	style := ravenMarkdownStyle()
	if style.CodeBlock.Theme != "dracula" {
		t.Fatalf("expected rendered style theme dracula, got %q", style.CodeBlock.Theme)
	}
}

func TestConfigureMarkdownCodeThemeFallsBackToDefault(t *testing.T) {
	orig := markdownCodeTheme
	t.Cleanup(func() {
		markdownCodeTheme = orig
	})

	ConfigureMarkdownCodeTheme("not-a-real-theme")
	if markdownCodeTheme != defaultCodeTheme {
		t.Fatalf("expected default code theme %q, got %q", defaultCodeTheme, markdownCodeTheme)
	}
}

func TestConfigureMarkdownCodeThemeIsCaseInsensitive(t *testing.T) {
	orig := markdownCodeTheme
	t.Cleanup(func() {
		markdownCodeTheme = orig
	})

	ConfigureMarkdownCodeTheme("DrAcUlA")
	if markdownCodeTheme != "dracula" {
		t.Fatalf("expected normalized code theme dracula, got %q", markdownCodeTheme)
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

	origTheme := markdownCodeTheme
	origMarkdownStyle := markdownStyle
	origAccent := Accent
	origBold := Bold
	origMuted := Muted
	origSyntax := Syntax
	origSyntaxSubtle := SyntaxSubtle
	origAccentColor := accentColor
	t.Cleanup(func() {
		markdownCodeTheme = origTheme
		markdownStyle = origMarkdownStyle
		Accent = origAccent
		Bold = origBold
		Muted = origMuted
		Syntax = origSyntax
		SyntaxSubtle = origSyntaxSubtle
		accentColor = origAccentColor
	})

	ConfigureTheme("39")
	ConfigureMarkdownCodeTheme("dracula")
	ConfigureMarkdownStyle("dark")

	out, err := RenderMarkdown("# Heading\n\n`value`\n\n```go\nfmt.Println(\"hi\")\n```", 80)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected markdown output without ANSI escapes when NO_COLOR is set, got %q", out)
	}
}
