package parser

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/aidanlsb/raven/internal/slugs"
)

// Heading represents a parsed heading.
type Heading struct {
	Level int
	Text  string
	Line  int // 1-indexed
}

// ExtractHeadings extracts headings from markdown content using goldmark.
func ExtractHeadings(content string, startLine int) []Heading {
	var headings []Heading

	md := goldmark.New()
	reader := text.NewReader([]byte(content))
	doc := md.Parser().Parse(reader)

	// Pre-compute line numbers for byte offsets
	lineStarts := computeLineStarts(content)

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := n.(*ast.Heading); ok {
			// Get heading text
			var textBuilder strings.Builder
			for child := heading.FirstChild(); child != nil; child = child.NextSibling() {
				if textNode, ok := child.(*ast.Text); ok {
					textBuilder.Write(textNode.Segment.Value([]byte(content)))
				}
			}

			headingText := strings.TrimSpace(textBuilder.String())
			if headingText == "" {
				return ast.WalkContinue, nil
			}

			// Calculate line number
			line := startLine
			if heading.Lines().Len() > 0 {
				offset := heading.Lines().At(0).Start
				line = startLine + offsetToLine(lineStarts, offset)
			}

			headings = append(headings, Heading{
				Level: heading.Level,
				Text:  headingText,
				Line:  line,
			})
		}

		return ast.WalkContinue, nil
	})

	return headings
}

// Slugify converts a heading text to a URL-friendly slug.
func Slugify(text string) string {
	return slugs.HeadingSlug(text)
}

// computeLineStarts computes the byte offset of each line start.
func computeLineStarts(content string) []int {
	starts := []int{0}
	for i, c := range content {
		if c == '\n' && i+1 < len(content) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offsetToLine converts a byte offset to a 0-indexed line number.
func offsetToLine(lineStarts []int, offset int) int {
	for i := len(lineStarts) - 1; i >= 0; i-- {
		if lineStarts[i] <= offset {
			return i
		}
	}
	return 0
}
