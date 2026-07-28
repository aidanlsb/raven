// Package parser provides AST-based parsing for Raven markdown files.
//
// This file implements goldmark-first parsing where the markdown AST is used
// to identify code blocks (which are skipped) and text content (where Raven
// syntax like traits and references are extracted).
package parser

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Goldmark's parser configuration is immutable after its first Parse call;
// each Parse creates its own context and AST, so the parser can be reused.
var markdownParser = goldmark.New().Parser()

// ASTContent holds all Raven syntax extracted from a markdown AST.
type ASTContent struct {
	Headings []Heading
	Traits   []TraitAnnotation
	Refs     []Reference
	Links    []MarkdownLink
}

// MarkdownLink is a direct [label](target) or ![alt](target) extracted from
// Markdown body content before source scope and target normalization are added.
type MarkdownLink struct {
	RawTarget     string
	Target        string
	Display       string
	IsImage       bool
	Line          int
	PositionStart int
	PositionEnd   int
}

// ExtractFromAST parses markdown content with goldmark and extracts all
// Raven-specific syntax (headings, traits, references).
//
// Code blocks (fenced, indented, inline) are automatically skipped - any
// @traits or [[references]] inside code will not be extracted.
func ExtractFromAST(content []byte, startLine int) (*ASTContent, error) {
	reader := text.NewReader(content)
	doc := markdownParser.Parse(reader)

	lineStarts := computeLineStarts(string(content))

	result := &ASTContent{}

	// First pass: extract headings.
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := n.(*ast.Heading); ok {
			headingInfo := extractHeadingFromNode(heading, content, lineStarts, startLine)
			if headingInfo != nil {
				result.Headings = append(result.Headings, *headingInfo)
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
			// Collect all text from this node, skipping inline code
			segments := collectTextSegments(processNode, content, lineStarts)
			for _, seg := range segments {
				line := startLine + offsetToLine(lineStarts, seg.start)

				// Parse traits
				traits := ParseTraitAnnotations(seg.text, line)
				result.Traits = append(result.Traits, traits...)

				// Parse refs
				refs := extractRefsFromText(seg.text, line)
				result.Refs = append(result.Refs, refs...)
			}
			result.Refs = append(result.Refs, extractMarkdownAssetRefs(processNode, content, lineStarts, startLine)...)

			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	}); err != nil {
		return nil, err
	}

	// Direct Markdown links are native AST nodes, so extract them once from the
	// whole document. This includes links in headings without duplicating links
	// nested under list items.
	result.Links = extractMarkdownLinks(doc, content, lineStarts, startLine)

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

// extractRefsFromText extracts wikilink references from a text segment.
func extractRefsFromText(textStr string, line int) []Reference {
	return extractRefsFromLine(textStr, line)
}

func extractMarkdownAssetRefs(node ast.Node, content []byte, lineStarts []int, startLine int) []Reference {
	var refs []Reference
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		switch typed := n.(type) {
		case *ast.Link:
			if ref, ok := markdownLinkRef(typed, content, lineStarts, startLine); ok {
				refs = append(refs, ref)
			}
		case *ast.Image:
			if ref, ok := markdownImageRef(typed, content, lineStarts, startLine); ok {
				refs = append(refs, ref)
			}
		case *ast.CodeSpan:
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			walk(child)
		}
	}
	walk(node)
	return refs
}

func extractMarkdownLinks(node ast.Node, content []byte, lineStarts []int, startLine int) []MarkdownLink {
	var links []MarkdownLink
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		switch typed := n.(type) {
		case *ast.Link:
			if link, ok := directMarkdownLink(typed, typed.Destination, false, content, lineStarts, startLine); ok {
				links = append(links, link)
			}
		case *ast.Image:
			if link, ok := directMarkdownLink(typed, typed.Destination, true, content, lineStarts, startLine); ok {
				links = append(links, link)
			}
		case *ast.CodeSpan:
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			walk(child)
		}
	}
	walk(node)
	return links
}

func directMarkdownLink(
	node ast.Node,
	destination []byte,
	isImage bool,
	content []byte,
	lineStarts []int,
	startLine int,
) (MarkdownLink, bool) {
	switch typed := node.(type) {
	case *ast.Link:
		if typed.Reference != nil {
			return MarkdownLink{}, false
		}
	case *ast.Image:
		if typed.Reference != nil {
			return MarkdownLink{}, false
		}
	}

	start := node.Pos()
	rawTarget, end, ok := markdownLinkSource(content, start, isImage)
	if !ok || rawTarget == "" {
		return MarkdownLink{}, false
	}
	lineIndex := offsetToLine(lineStarts, start)
	lineStartOffset := lineStarts[lineIndex]
	return MarkdownLink{
		RawTarget:     rawTarget,
		Target:        string(destination),
		Display:       collectInlineText(node, content),
		IsImage:       isImage,
		Line:          startLine + lineIndex,
		PositionStart: start - lineStartOffset,
		PositionEnd:   end - lineStartOffset,
	}, true
}

func markdownLinkSource(content []byte, start int, isImage bool) (rawTarget string, end int, ok bool) {
	if start < 0 || start >= len(content) {
		return "", 0, false
	}
	labelOpen := start
	if isImage {
		if start+1 >= len(content) || content[start] != '!' {
			return "", 0, false
		}
		labelOpen++
	}
	if content[labelOpen] != '[' {
		return "", 0, false
	}

	labelClose := matchingBracket(content, labelOpen, '[', ']')
	if labelClose < 0 || labelClose+1 >= len(content) || content[labelClose+1] != '(' {
		return "", 0, false
	}
	openParen := labelClose + 1
	destinationStart := skipMarkdownSpaces(content, openParen+1)
	if destinationStart >= len(content) {
		return "", 0, false
	}

	destinationEnd := destinationStart
	if content[destinationStart] == '<' {
		destinationStart++
		destinationEnd = escapedDelimiter(content, destinationStart, '>')
		if destinationEnd < 0 {
			return "", 0, false
		}
	} else {
		destinationEnd = unwrappedDestinationEnd(content, destinationStart)
	}
	if destinationEnd < destinationStart {
		return "", 0, false
	}

	linkEnd := markdownLinkEnd(content, destinationEnd)
	if linkEnd < 0 {
		return "", 0, false
	}
	return string(content[destinationStart:destinationEnd]), linkEnd, true
}

func matchingBracket(content []byte, start int, opener, closer byte) int {
	depth := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '\\':
			i++
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func escapedDelimiter(content []byte, start int, delimiter byte) int {
	for i := start; i < len(content); i++ {
		if content[i] == '\\' {
			i++
			continue
		}
		if content[i] == delimiter {
			return i
		}
	}
	return -1
}

func unwrappedDestinationEnd(content []byte, start int) int {
	opened := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '\\':
			i++
		case '(':
			opened++
		case ')':
			if opened == 0 {
				return i
			}
			opened--
		case ' ', '\t', '\n', '\r':
			return i
		}
	}
	return len(content)
}

func markdownLinkEnd(content []byte, destinationEnd int) int {
	i := destinationEnd
	if i < len(content) && content[i] == '>' {
		i++
	}
	i = skipMarkdownSpaces(content, i)
	if i >= len(content) {
		return -1
	}
	if content[i] == ')' {
		return i + 1
	}

	opener := content[i]
	closer := opener
	switch opener {
	case '"', '\'':
	case '(':
		closer = ')'
	default:
		return -1
	}
	titleEnd := escapedDelimiter(content, i+1, closer)
	if titleEnd < 0 {
		return -1
	}
	i = skipMarkdownSpaces(content, titleEnd+1)
	if i >= len(content) || content[i] != ')' {
		return -1
	}
	return i + 1
}

func skipMarkdownSpaces(content []byte, start int) int {
	for start < len(content) {
		switch content[start] {
		case ' ', '\t', '\n', '\r':
			start++
		default:
			return start
		}
	}
	return start
}

func markdownLinkRef(link *ast.Link, content []byte, lineStarts []int, startLine int) (Reference, bool) {
	target, ok := normalizeMarkdownAssetDestination(string(link.Destination))
	if !ok {
		return Reference{}, false
	}
	line, start := nodeLineStart(link, lineStarts, startLine)
	display := collectInlineText(link, content)
	return Reference{
		TargetRaw:   target,
		DisplayText: optionalDisplayText(display, target),
		Line:        line,
		Start:       start,
		End:         start,
	}, true
}

func markdownImageRef(image *ast.Image, content []byte, lineStarts []int, startLine int) (Reference, bool) {
	target, ok := normalizeMarkdownAssetDestination(string(image.Destination))
	if !ok {
		return Reference{}, false
	}
	line, start := nodeLineStart(image, lineStarts, startLine)
	display := collectInlineText(image, content)
	return Reference{
		TargetRaw:   target,
		DisplayText: optionalDisplayText(display, target),
		Line:        line,
		Start:       start,
		End:         start,
	}, true
}

// NormalizeMarkdownDestination normalizes a markdown link/image destination to a
// vault-relative asset path. It returns ok=false for destinations that are not
// asset references (external URLs, mailto:, pure fragments, protocol-relative
// URLs, markdown targets, or extensionless paths).
//
// This is the canonical normalization shared by ref extraction and ref
// rewriting so both agree on which markdown destinations are asset references.
func NormalizeMarkdownDestination(raw string) (string, bool) {
	return normalizeMarkdownAssetDestination(raw)
}

func normalizeMarkdownAssetDestination(raw string) (string, bool) {
	target := strings.TrimSpace(raw)
	target = strings.TrimPrefix(target, "<")
	target = strings.TrimSuffix(target, ">")
	if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "//") {
		return "", false
	}
	lower := strings.ToLower(target)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "mailto:") {
		return "", false
	}
	if idx := strings.IndexAny(target, "?#"); idx >= 0 {
		target = target[:idx]
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	target = strings.TrimPrefix(target, "/")
	target = strings.TrimPrefix(target, "./")
	normalized := filepath.ToSlash(filepath.Clean(target))
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "." || normalized == "" || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", false
	}
	if strings.HasSuffix(strings.ToLower(normalized), ".md") {
		return "", false
	}
	if filepath.Ext(normalized) == "" {
		return "", false
	}
	return normalized, true
}

func optionalDisplayText(display, target string) *string {
	display = strings.TrimSpace(display)
	if display == "" || display == target {
		return nil
	}
	return &display
}

func nodeLineStart(node ast.Node, lineStarts []int, startLine int) (int, int) {
	if offset, ok := firstTextOffset(node); ok {
		return startLine + offsetToLine(lineStarts, offset), 0
	}
	if node.Lines().Len() > 0 {
		offset := node.Lines().At(0).Start
		return startLine + offsetToLine(lineStarts, offset), 0
	}
	return startLine, 0
}

func firstTextOffset(node ast.Node) (int, bool) {
	if textNode, ok := node.(*ast.Text); ok {
		return textNode.Segment.Start, true
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if offset, ok := firstTextOffset(child); ok {
			return offset, true
		}
	}
	return 0, false
}

func collectInlineText(node ast.Node, content []byte) string {
	var b strings.Builder
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		if textNode, ok := n.(*ast.Text); ok {
			b.Write(textNode.Segment.Value(content))
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			walk(child)
		}
	}
	walk(node)
	return strings.TrimSpace(b.String())
}

// textSegment represents a contiguous piece of text with its byte offset.
type textSegment struct {
	text  string
	start int
}

// collectTextSegments collects all text from a node, grouping by line.
// This is needed because goldmark splits text at special characters like '['.
func collectTextSegments(node ast.Node, content []byte, lineStarts []int) []textSegment {
	var segments []textSegment

	// We'll collect text by line to preserve line number accuracy
	lineTexts := make(map[int]*strings.Builder)
	lineOffsets := make(map[int]int) // line -> first byte offset

	ensureLineBuilder := func(line int, startOffset int) *strings.Builder {
		if _, ok := lineTexts[line]; !ok {
			lineTexts[line] = &strings.Builder{}
			lineOffsets[line] = startOffset
		}
		return lineTexts[line]
	}

	var walkNode func(n ast.Node)
	walkNode = func(n ast.Node) {
		// Preserve inline code spans in the collected text.
		if codeSpan, ok := n.(*ast.CodeSpan); ok {
			appendInlineCodeSpan(codeSpan, content, lineStarts, lineTexts, lineOffsets)
			return
		}

		// Process text nodes
		if textNode, ok := n.(*ast.Text); ok {
			segment := textNode.Segment
			text := string(segment.Value(content))
			line := offsetToLine(lineStarts, segment.Start)

			ensureLineBuilder(line, segment.Start).WriteString(text)
		}

		// Recurse into children
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			walkNode(child)
		}
	}

	walkNode(node)

	// Convert to segments, sorted by line for deterministic extraction order.
	lines := make([]int, 0, len(lineTexts))
	for line := range lineTexts {
		lines = append(lines, line)
	}
	sort.Ints(lines)

	for _, line := range lines {
		builder := lineTexts[line]
		segments = append(segments, textSegment{
			text:  builder.String(),
			start: lineOffsets[line],
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
