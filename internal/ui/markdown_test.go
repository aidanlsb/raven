package ui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownNormalizesTrailingNewline(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
