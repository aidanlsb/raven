// Package parser provides AST-based parsing for Raven markdown files.
//
// This file implements goldmark-first parsing where the markdown AST is used
// to identify code blocks (which are skipped) and text content (where Raven
// syntax like traits and references are extracted).
package parser

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/aidanlsb/raven/internal/wikilink"
)

// ASTContent holds all Raven syntax extracted from a markdown AST.
type ASTContent struct {
	Headings  []Heading
	Traits    []TraitAnnotation
	Refs      []Reference
	TypeDecls map[int]*EmbeddedTypeInfo // heading line -> type decl
}

// ExtractFromAST parses markdown content with goldmark and extracts all
// Raven-specific syntax (headings, traits, references, type declarations).
//
// Code blocks (fenced, indented, inline) are automatically skipped - any
// @traits or [[references]] inside code will not be extracted.
func ExtractFromAST(content []byte, startLine int) (*ASTContent, error) {
	md := goldmark.New()
	reader := text.NewReader(content)
	doc := md.Parser().Parse(reader)

	lineStarts := computeLineStarts(string(content))

	result := &ASTContent{
		TypeDecls: make(map[int]*EmbeddedTypeInfo),
	}

	// Track paragraphs that contain type declarations (to skip them for trait/ref extraction)
	consumedNodes := make(map[ast.Node]bool)

	// First pass: extract headings and detect type declarations
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := n.(*ast.Heading); ok {
			headingInfo := extractHeadingFromNode(heading, content, lineStarts, startLine)
			if headingInfo != nil {
				result.Headings = append(result.Headings, *headingInfo)

				// Check next sibling for ::type() declaration
				if next := heading.NextSibling(); next != nil {
					if para, ok := next.(*ast.Paragraph); ok {
						if decl := extractTypeDeclFromParagraph(para, content, lineStarts, startLine); decl != nil {
							result.TypeDecls[headingInfo.Line] = decl
							consumedNodes[para] = true
						}
					}
				}
			}
		}

		return ast.WalkContinue, nil
	}); err != nil {
		return nil, err
	}

	// Second pass: extract traits and refs from non-code, non-consumed content.
	// We process at the paragraph/list-item level because goldmark splits wikilinks
	// like [[target]] across multiple Text nodes (due to [ being link syntax).
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Skip code constructs entirely
		switch n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		}

		// Skip consumed nodes (type declarations)
		if consumedNodes[n] {
			return ast.WalkSkipChildren, nil
		}

		// Process block-level nodes that contain text content.
		// We handle Paragraph and ListItem because they contain the actual text.
		// Goldmark splits wikilinks like [[target]] across multiple Text nodes,
		// so we need to collect text at the block level.
		var processNode ast.Node
		switch node := n.(type) {
		case *ast.Paragraph:
			processNode = node
		case *ast.ListItem:
			processNode = node
		}

		if processNode != nil {
			// Check parent chain for consumed nodes
			for parent := processNode.Parent(); parent != nil; parent = parent.Parent() {
				if consumedNodes[parent] {
					return ast.WalkSkipChildren, nil
				}
			}

			// Collect all text from this node, skipping inline code
			segments := collectTextSegments(processNode, content)
			for _, seg := range segments {
				line := startLine + offsetToLine(lineStarts, seg.start)

				// Parse traits
				traits := ParseTraitAnnotations(seg.text, line)
				result.Traits = append(result.Traits, traits...)

				// Parse refs
				refs := extractRefsFromText(seg.text, line)
				result.Refs = append(result.Refs, refs...)
			}

			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// extractHeadingFromNode extracts heading information from a goldmark Heading node.
func extractHeadingFromNode(heading *ast.Heading, content []byte, lineStarts []int, startLine int) *Heading {
	// Get heading text by concatenating all text children
	var textBuilder strings.Builder
	for child := heading.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok {
			textBuilder.Write(textNode.Segment.Value(content))
		}
	}

	headingText := strings.TrimSpace(textBuilder.String())
	if headingText == "" {
		return nil
	}

	// Calculate line number
	line := startLine
	if heading.Lines().Len() > 0 {
		offset := heading.Lines().At(0).Start
		line = startLine + offsetToLine(lineStarts, offset)
	}

	return &Heading{
		Level: heading.Level,
		Text:  headingText,
		Line:  line,
	}
}

// extractTypeDeclFromParagraph checks if a paragraph contains a type declaration.
// Returns nil if the paragraph doesn't start with "::".
//
// Note: Goldmark splits text at special characters like '[', so we need to
// collect all text nodes in the paragraph to get the full type declaration.
func extractTypeDeclFromParagraph(para *ast.Paragraph, content []byte, lineStarts []int, startLine int) *EmbeddedTypeInfo {
	// Get the first text node to check for "::" prefix and get the line number
	firstChild := para.FirstChild()
	if firstChild == nil {
		return nil
	}

	textNode, ok := firstChild.(*ast.Text)
	if !ok {
		return nil
	}

	// Quick check: first text node must start with "::"
	firstSegment := textNode.Segment
	firstText := strings.TrimSpace(string(firstSegment.Value(content)))
	if !strings.HasPrefix(firstText, "::") {
		return nil
	}

	// Collect all text from the paragraph (goldmark may split at '[' etc.)
	var builder strings.Builder
	for child := para.FirstChild(); child != nil; child = child.NextSibling() {
		if tn, ok := child.(*ast.Text); ok {
			builder.Write(tn.Segment.Value(content))
		}
	}

	fullText := strings.TrimSpace(builder.String())
	line := startLine + offsetToLine(lineStarts, firstSegment.Start)
	return ParseEmbeddedType(fullText, line)
}

// extractRefsFromText extracts wikilink references from a text segment.
func extractRefsFromText(textStr string, line int) []Reference {
	var refs []Reference

	sanitized := RemoveInlineCode(textStr)
	matches := wikilink.FindAllInLine(sanitized, false)
	for _, match := range matches {
		refs = append(refs, Reference{
			TargetRaw:   match.Target,
			DisplayText: match.DisplayText,
			Line:        line,
			Start:       match.Start,
			End:         match.End,
		})
	}

	return refs
}

// textSegment represents a contiguous piece of text with its byte offset.
type textSegment struct {
	text  string
	start int
}

// collectTextSegments collects all text from a node, grouping by line.
// This is needed because goldmark splits text at special characters like '['.
func collectTextSegments(node ast.Node, content []byte) []textSegment {
	var segments []textSegment

	// We'll collect text by line to preserve line number accuracy
	lineTexts := make(map[int]*strings.Builder)
	lineStarts := make(map[int]int) // line -> first byte offset

	localLineStarts := computeLineStarts(string(content))

	ensureLineBuilder := func(line int, startOffset int) *strings.Builder {
		if _, ok := lineTexts[line]; !ok {
			lineTexts[line] = &strings.Builder{}
			lineStarts[line] = startOffset
		}
		return lineTexts[line]
	}

	var walkNode func(n ast.Node)
	walkNode = func(n ast.Node) {
		// Preserve inline code spans in the collected text.
		if codeSpan, ok := n.(*ast.CodeSpan); ok {
			appendInlineCodeSpan(codeSpan, content, localLineStarts, lineTexts, lineStarts)
			return
		}

		// Process text nodes
		if textNode, ok := n.(*ast.Text); ok {
			segment := textNode.Segment
			text := string(segment.Value(content))
			line := offsetToLine(localLineStarts, segment.Start)

			ensureLineBuilder(line, segment.Start).WriteString(text)
		}

		// Recurse into children
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			walkNode(child)
		}
	}

	walkNode(node)

	// Convert to segments, sorted by line
	for line, builder := range lineTexts {
		segments = append(segments, textSegment{
			text:  builder.String(),
			start: lineStarts[line],
		})
	}

	return segments
}

func appendInlineCodeSpan(
	node *ast.CodeSpan,
	content []byte,
	lineStarts []int,
	lineTexts map[int]*strings.Builder,
	lineOffsets map[int]int,
) {
	code, startLine, ok := extractCodeSpanText(node, content, lineStarts)
	if !ok {
		return
	}

	wrapped := wrapInlineCode(code)
	lines := strings.Split(wrapped, "\n")
	for i, lineText := range lines {
		lineNum := startLine + i
		startOffset := 0
		if lineNum >= 0 && lineNum < len(lineStarts) {
			startOffset = lineStarts[lineNum]
		}
		if _, ok := lineTexts[lineNum]; !ok {
			lineTexts[lineNum] = &strings.Builder{}
			lineOffsets[lineNum] = startOffset
		}
		lineTexts[lineNum].WriteString(lineText)
	}
}

func extractCodeSpanText(node *ast.CodeSpan, content []byte, lineStarts []int) (string, int, bool) {
	var b strings.Builder
	startLine := -1

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		segment := textNode.Segment
		if startLine == -1 {
			startLine = offsetToLine(lineStarts, segment.Start)
		}
		b.Write(segment.Value(content))
	}

	if startLine == -1 {
		return "", 0, false
	}

	return b.String(), startLine, true
}

func wrapInlineCode(code string) string {
	if code == "" {
		return "``"
	}

	maxRun := 0
	current := 0
	for i := 0; i < len(code); i++ {
		if code[i] == '`' {
			current++
			if current > maxRun {
				maxRun = current
			}
		} else {
			current = 0
		}
	}

	delimiterLen := maxRun + 1
	if delimiterLen < 1 {
		delimiterLen = 1
	}

	delim := strings.Repeat("`", delimiterLen)
	return delim + code + delim
}
