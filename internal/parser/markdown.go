package parser

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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

// ExtractInlineTags extracts #tag patterns from content.
// Only extracts tags from regular text, not from code blocks or inline code.
func ExtractInlineTags(content string) []string {
	tags := make(map[string]struct{})

	md := goldmark.New()
	reader := text.NewReader([]byte(content))
	doc := md.Parser().Parse(reader)

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Skip code blocks and code spans
		switch n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.CodeSpan:
			return ast.WalkSkipChildren, nil
		}

		// Extract tags from text nodes
		if textNode, ok := n.(*ast.Text); ok {
			text := string(textNode.Segment.Value([]byte(content)))
			for _, tag := range extractTagsFromText(text) {
				tags[tag] = struct{}{}
			}
		}

		return ast.WalkContinue, nil
	})

	// Convert map to sorted slice
	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}

	return result
}

// tagRegex matches #tag patterns.
// Tags must start with a letter or underscore (not a digit to avoid #123 issue refs).
var tagRegex = regexp.MustCompile(`(?:^|[\s\(\[])#([a-zA-Z_][a-zA-Z0-9_-]*)`)

// extractTagsFromText extracts #tag patterns from a text string.
func extractTagsFromText(text string) []string {
	var tags []string

	matches := tagRegex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			tags = append(tags, match[1])
		}
	}

	return tags
}

// Slugify converts a heading text to a URL-friendly slug.
func Slugify(text string) string {
	var result strings.Builder
	prevDash := false

	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			result.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == ':':
			// Convert separators (including colon) to dashes
			if !prevDash && result.Len() > 0 {
				result.WriteRune('-')
				prevDash = true
			}
		}
	}

	s := result.String()
	// Trim trailing dash
	return strings.TrimSuffix(s, "-")
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
