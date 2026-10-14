package parser

import (
	"strings"

	"github.com/aidanlsb/raven/internal/wikilink"
)

// WikilinkSpan is a [[wikilink]] located in markdown content. Offsets are
// 0-based and end-exclusive UTF-8 byte indexes into the scanned string.
type WikilinkSpan struct {
	Target      string
	DisplayText *string
	Literal     string
	ByteStart   int
	ByteEnd     int
}

// ExtractWikilinkSpans finds wikilinks in markdown using the same extractors
// as document parsing: YAML frontmatter via ExtractRefs, body via
// ExtractFromAST (which skips fenced, indented, and inline code). Offsets
// refer to content, not to a reconstructed AST string.
func ExtractWikilinkSpans(content string) ([]WikilinkSpan, error) {
	if content == "" {
		return nil, nil
	}

	lineStarts := computeLineStarts(content)
	lines := strings.Split(content, "\n")
	fmEndIdx, body, bodyOffset, hasFM := splitMarkdownRegions(lines)

	var spans []WikilinkSpan
	usedOnLine := map[int]map[int]struct{}{}
	if hasFM {
		raw := strings.Join(lines[1:fmEndIdx], "\n")
		for _, ref := range ExtractRefs(raw, 2) {
			span, ok := spanFromLineRef(content, lines, lineStarts, ref, usedOnLine)
			if ok {
				spans = append(spans, span)
			}
		}
	}

	astContent, err := ExtractFromAST([]byte(body), 1)
	if err != nil {
		return nil, err
	}
	bodyLineStarts := computeLineStarts(body)
	bodyLines := strings.Split(body, "\n")
	bodyUsed := map[int]map[int]struct{}{}
	for _, ref := range astContent.Refs {
		span, ok := spanFromBodyRef(body, bodyLines, bodyLineStarts, bodyOffset, ref, bodyUsed)
		if ok {
			spans = append(spans, span)
		}
	}
	return spans, nil
}

func splitMarkdownRegions(lines []string) (fmEndIdx int, body string, bodyOffset int, hasFM bool) {
	_, endLine, ok := FrontmatterBounds(lines)
	if !ok || endLine == -1 {
		return 0, strings.Join(lines, "\n"), 0, false
	}
	frontmatter := strings.Join(lines[:endLine+1], "\n") + "\n"
	if endLine+1 < len(lines) {
		body = strings.Join(lines[endLine+1:], "\n")
	}
	return endLine, body, len(frontmatter), true
}

func spanFromLineRef(content string, lines []string, lineStarts []int, ref Reference, usedOnLine map[int]map[int]struct{}) (WikilinkSpan, bool) {
	lineIdx := ref.Line - 1
	if lineIdx < 0 || lineIdx >= len(lines) || lineIdx >= len(lineStarts) {
		return WikilinkSpan{}, false
	}
	used := usedOnLine[ref.Line]
	if used == nil {
		used = make(map[int]struct{})
		usedOnLine[ref.Line] = used
	}
	return locateWikilink(content, lines[lineIdx], lineStarts[lineIdx], ref, used)
}

func spanFromBodyRef(
	body string,
	bodyLines []string,
	bodyLineStarts []int,
	bodyOffset int,
	ref Reference,
	usedOnLine map[int]map[int]struct{},
) (WikilinkSpan, bool) {
	lineIdx := ref.Line - 1
	if lineIdx < 0 || lineIdx >= len(bodyLines) || lineIdx >= len(bodyLineStarts) {
		return WikilinkSpan{}, false
	}
	used := usedOnLine[ref.Line]
	if used == nil {
		used = make(map[int]struct{})
		usedOnLine[ref.Line] = used
	}
	span, ok := locateWikilink(body, bodyLines[lineIdx], bodyLineStarts[lineIdx], ref, used)
	if !ok {
		return WikilinkSpan{}, false
	}
	span.ByteStart += bodyOffset
	span.ByteEnd += bodyOffset
	return span, true
}

func locateWikilink(region, line string, lineStart int, ref Reference, used map[int]struct{}) (WikilinkSpan, bool) {
	if span, ok := spanAt(region, line, lineStart, ref.Start, ref.End, ref.TargetRaw, ref.DisplayText); ok {
		if used != nil {
			if _, taken := used[ref.Start]; !taken {
				used[ref.Start] = struct{}{}
				return span, true
			}
		} else {
			return span, true
		}
	}

	codeSpans := inlineCodeSpans(line)
	for _, match := range wikilink.FindAllInLine(line, false) {
		if match.Target != ref.TargetRaw {
			continue
		}
		if matchInsideInlineCode(match.Start, match.End, codeSpans) {
			continue
		}
		if used != nil {
			if _, taken := used[match.Start]; taken {
				continue
			}
			used[match.Start] = struct{}{}
		}
		return WikilinkSpan{
			Target:      match.Target,
			DisplayText: match.DisplayText,
			Literal:     match.Literal,
			ByteStart:   lineStart + match.Start,
			ByteEnd:     lineStart + match.End,
		}, true
	}
	return WikilinkSpan{}, false
}

func spanAt(region, line string, lineStart, start, end int, target string, display *string) (WikilinkSpan, bool) {
	if start < 0 || end <= start || end > len(line) {
		return WikilinkSpan{}, false
	}
	literal := line[start:end]
	parsedTarget, parsedDisplay, ok := wikilink.ParseExact(literal)
	if !ok || parsedTarget != target {
		return WikilinkSpan{}, false
	}
	absStart := lineStart + start
	absEnd := lineStart + end
	if absStart < 0 || absEnd > len(region) || region[absStart:absEnd] != literal {
		return WikilinkSpan{}, false
	}
	if display == nil {
		display = parsedDisplay
	}
	return WikilinkSpan{
		Target:      target,
		DisplayText: display,
		Literal:     literal,
		ByteStart:   absStart,
		ByteEnd:     absEnd,
	}, true
}
