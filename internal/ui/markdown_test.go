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
