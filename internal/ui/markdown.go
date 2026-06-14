package ui

import (
	"strings"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
)

// MarkdownRenderMargin is the left margin used for terminal markdown rendering.
const MarkdownRenderMargin = 2

const defaultCodeTheme = "monokai"

var (
	markdownCodeTheme = defaultCodeTheme
	markdownStyle     = "auto"
)

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
	switch style {
	case "", "auto":
		return append(options, glamour.WithAutoStyle())
	case "raven":
		return append(options, glamour.WithStyles(ravenMarkdownStyle()))
	default:
		return append(options, glamour.WithStylePath(style))
	}
}

func ravenMarkdownStyle() ansi.StyleConfig {
	muted := mdStringPtr("8")
	heading := mdStringPtr("12")
	syntax := mdStringPtr("6")
	if color, ok := AccentColor(); ok {
		heading = mdStringPtr(color)
		syntax = mdStringPtr(color)
	}

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "\n",
				BlockSuffix: "\n",
			},
			Margin: mdUintPtr(MarkdownRenderMargin),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: muted,
			},
			Indent:      mdUintPtr(1),
			IndentToken: mdStringPtr("│ "),
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{},
		},
		List: ansi.StyleList{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{},
			},
			LevelIndent: 2,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "\n",
				BlockSuffix: "\n",
				Color:       heading,
				Bold:        mdBoolPtr(true),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:    "# ",
				Color:     heading,
				Bold:      mdBoolPtr(true),
				Underline: mdBoolPtr(true),
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:    "## ",
				Color:     heading,
				Bold:      mdBoolPtr(true),
				Underline: mdBoolPtr(true),
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  heading,
				Bold:   mdBoolPtr(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  heading,
				Bold:   mdBoolPtr(true),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  heading,
				Bold:   mdBoolPtr(true),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  heading,
				Bold:   mdBoolPtr(false),
			},
		},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: mdBoolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Italic: mdBoolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: mdBoolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  muted,
			Format: "\n--------\n",
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		Task: ansi.StyleTask{
			Ticked:   "[x] ",
			Unticked: "[ ] ",
		},
		Link: ansi.StylePrimitive{
			Color:     muted,
			Underline: mdBoolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: muted,
			Bold:  mdBoolPtr(true),
		},
		Image: ansi.StylePrimitive{
			Underline: mdBoolPtr(true),
		},
		ImageText: ansi.StylePrimitive{
			Color:  muted,
			Format: "Image: {{.text}} ->",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "`",
				Suffix: "`",
				Color:  syntax,
				Bold:   mdBoolPtr(true),
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: syntax,
				},
			},
			Theme: markdownCodeTheme,
		},
		Table: ansi.StyleTable{
			CenterSeparator: mdStringPtr("│"),
			ColumnSeparator: mdStringPtr("│"),
			RowSeparator:    mdStringPtr("─"),
		},
		DefinitionDescription: ansi.StylePrimitive{
			BlockPrefix: "\n- ",
		},
	}
}

// ConfigureMarkdownCodeTheme sets the code block theme used by Glamour.
// Invalid or empty values fall back to the default theme.
func ConfigureMarkdownCodeTheme(theme string) {
	markdownCodeTheme = normalizeCodeTheme(theme)
}

// ConfigureMarkdownStyle sets the Glamour style used for rendered markdown.
// Empty or "auto" uses Glamour's automatic light/dark style. "raven" uses
// Raven's built-in legacy style. Other values are passed to Glamour as either a
// style JSON path or standard style name, falling back to auto if invalid.
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

func normalizeCodeTheme(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultCodeTheme
	}
	for _, name := range styles.Names() {
		if name == value {
			return name
		}
		if strings.EqualFold(name, value) {
			return name
		}
	}
	return defaultCodeTheme
}

func mdBoolPtr(v bool) *bool { return &v }

func mdStringPtr(v string) *string { return &v }

func mdUintPtr(v uint) *uint { return &v }
