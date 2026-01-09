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

type fenceState struct {
	inFence  bool
	fenceCh  byte
	fenceLen int
}

func normalizeFenceLine(line string) string {
	// Allow up to 3 leading spaces and handle blockquote prefixes (`>`),
	// so we can detect fenced code blocks in common markdown contexts.
	s := strings.TrimLeft(line, " \t")
	for strings.HasPrefix(s, ">") {
		s = strings.TrimPrefix(s, ">")
		s = strings.TrimLeft(s, " \t")
	}
	return s
}

func parseFenceMarker(line string) (ch byte, n int, ok bool) {
	if len(line) < 3 {
		return 0, 0, false
	}
	ch = line[0]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	i := 0
	for i < len(line) && line[i] == ch {
		i++
	}
	if i < 3 {
		return 0, 0, false
	}
	return ch, i, true
}

// ExtractRefs extracts references from content.
func ExtractRefs(content string, startLine int) []Reference {
	var refs []Reference

	lines := strings.Split(content, "\n")
	state := fenceState{}
	for lineOffset, line := range lines {
		lineNum := startLine + lineOffset

		// Skip wiki refs inside fenced code blocks.
		fenceLine := normalizeFenceLine(line)
		if ch, n, ok := parseFenceMarker(fenceLine); ok {
			if !state.inFence {
				state.inFence = true
				state.fenceCh = ch
				state.fenceLen = n
			} else if state.fenceCh == ch && n >= state.fenceLen {
				state.inFence = false
				state.fenceCh = 0
				state.fenceLen = 0
			}
			continue
		}
		if state.inFence {
			continue
		}

		matches := wikilink.FindAllInLine(line, false)
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
