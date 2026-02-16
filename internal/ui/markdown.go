package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
)

// MarkdownRenderMargin is the left margin used for terminal markdown rendering.
const MarkdownRenderMargin = 2

// RenderMarkdown renders markdown content for terminal display using the shared
// Raven style configuration.
func RenderMarkdown(content string, width int) (string, error) {
	if width <= 0 {
		width = DefaultTermWidth
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(ravenMarkdownStyle()),
		glamour.WithWordWrap(width),
	)
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

func ravenMarkdownStyle() ansi.StyleConfig {
	muted := mdStringPtr("8")
	var accent *string
	if color, ok := AccentColor(); ok {
		accent = mdStringPtr(color)
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
				BlockSuffix: "\n",
				Color:       accent,
				Bold:        mdBoolPtr(true),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "# ",
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
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
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{},
			},
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

func mdBoolPtr(v bool) *bool { return &v }

func mdStringPtr(v string) *string { return &v }

func mdUintPtr(v uint) *uint { return &v }
