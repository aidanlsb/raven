package parser

import (
	"strings"

	"github.com/aidanlsb/raven/internal/wikilink"
)

// Reference represents a parsed [[wikilink]] reference.
type Reference struct {
	TargetRaw   string  // The raw target (as written)
	DisplayText *string // Display text (if different from target)
	Line        int     // Line number where found (1-indexed)
	Start       int     // Start position in line
	End         int     // End position in line
}

// ExtractRefs extracts references from plain text content line by line.
// This is used for frontmatter/raw text scanning; markdown-aware code skipping
// is handled by the AST parser path instead.
func ExtractRefs(content string, startLine int) []Reference {
	var refs []Reference

	lines := strings.Split(content, "\n")
	for lineOffset, line := range lines {
		lineNum := startLine + lineOffset
		refs = append(refs, extractRefsFromLine(line, lineNum)...)
	}

	return refs
}

func extractRefsFromLine(line string, lineNum int) []Reference {
	var refs []Reference
	matches := wikilink.FindAllInLine(line, false)
	codeSpans := inlineCodeSpans(line)
	for _, match := range matches {
		if matchInsideInlineCode(match.Start, match.End, codeSpans) {
			continue
		}
		refs = append(refs, Reference{
			TargetRaw:   match.Target,
			DisplayText: match.DisplayText,
			Line:        lineNum,
			Start:       match.Start,
			End:         match.End,
		})
	}
	return refs
}

func matchInsideInlineCode(start, end int, codeSpans []inlineCodeSpan) bool {
	for _, span := range codeSpans {
		if start >= span.start && end <= span.end {
			return true
		}
	}
	return false
}

// ExtractValueRefs parses refs in field and trait values like [[[path/to/file]], [[other]]].
// Handles array syntax where refs are wrapped in extra brackets.
func ExtractValueRefs(value string) []string {
	var refs []string

	matches := wikilink.FindAllInLine(value, true)
	for _, match := range matches {
		refs = append(refs, match.Target)
	}

	return refs
}
