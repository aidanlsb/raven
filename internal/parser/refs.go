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

// ExtractRefs extracts references from content.
// It automatically skips refs inside fenced code blocks and inline code spans.
func ExtractRefs(content string, startLine int) []Reference {
	var refs []Reference

	lines := strings.Split(content, "\n")
	state := FenceState{}
	for lineOffset, line := range lines {
		lineNum := startLine + lineOffset

		// Skip wiki refs inside fenced code blocks.
		if state.UpdateFenceState(line) {
			continue // This line is a fence marker
		}
		if state.InFence {
			continue // Inside a fenced code block
		}

		// Remove inline code spans to avoid matching refs inside them
		sanitizedLine := RemoveInlineCode(line)
		matches := wikilink.FindAllInLine(sanitizedLine, false)
		for _, match := range matches {
			refs = append(refs, Reference{
				TargetRaw:   match.Target,
				DisplayText: match.DisplayText,
				Line:        lineNum,
				Start:       match.Start,
				End:         match.End,
			})
		}
	}

	return refs
}

// ExtractEmbeddedRefs parses embedded refs in trait values like [[[path/to/file]], [[other]]].
// Handles array syntax where refs are wrapped in extra brackets.
func ExtractEmbeddedRefs(value string) []string {
	var refs []string

	matches := wikilink.FindAllInLine(value, true)
	for _, match := range matches {
		refs = append(refs, match.Target)
	}

	return refs
}
