package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
)

// MarkdownRenderMargin is the left margin used for terminal markdown rendering.
const MarkdownRenderMargin = 2

var markdownStyle = "auto"

// RenderMarkdown renders markdown content for terminal display using the shared
// Raven style configuration.
func RenderMarkdown(content string, width int) (string, error) {
	if width <= 0 {
		width = DefaultTermWidth
	}

	r, err := newMarkdownRenderer(width)
	if err != nil {
		return "", err
	}

	rendered, err := r.Render(content)
	if err != nil {
		return "", err
	}

	// glamour adds trailing newlines; normalize to a single trailing newline.
	rendered = strings.TrimRight(rendered, "\n") + "\n"
	return rendered, nil
}

func newMarkdownRenderer(width int) (*glamour.TermRenderer, error) {
	options := markdownRendererOptions(width)
	r, err := glamour.NewTermRenderer(options...)
	if err == nil {
		return r, nil
	}
	if NoColorEnabled() || normalizeMarkdownStyle(markdownStyle) == "auto" {
		return nil, err
	}

	// Invalid custom style names/paths should not break docs/read rendering.
	return glamour.NewTermRenderer(glamour.WithWordWrap(width), glamour.WithAutoStyle())
}

func markdownRendererOptions(width int) []glamour.TermRendererOption {
	options := []glamour.TermRendererOption{glamour.WithWordWrap(width)}
	if NoColorEnabled() {
		style := glamourstyles.ASCIIStyleConfig
		style.Document.Margin = mdUintPtr(MarkdownRenderMargin)
		return append(options, glamour.WithStyles(style))
	}

	style := normalizeMarkdownStyle(markdownStyle)
	if style == "auto" {
		return append(options, glamour.WithAutoStyle())
	}
	return append(options, glamour.WithStylePath(style))
}

// ConfigureMarkdownStyle sets the Glamour style used for rendered markdown.
// Empty or "auto" uses Glamour's automatic light/dark style. Other values are
// passed to Glamour as either a style JSON path or stock built-in style name,
// falling back to auto if invalid.
func ConfigureMarkdownStyle(style string) {
	markdownStyle = normalizeMarkdownStyle(style)
}

func normalizeMarkdownStyle(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "auto"
	}
	return value
}

func mdUintPtr(v uint) *uint { return &v }
