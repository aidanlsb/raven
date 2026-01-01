package parser

import (
	"regexp"
	"strings"
)

// Reference represents a parsed [[wikilink]] reference.
type Reference struct {
	TargetRaw   string  // The raw target (as written)
	DisplayText *string // Display text (if different from target)
	Line        int     // Line number where found (1-indexed)
	Start       int     // Start position in line
	End         int     // End position in line
}

// wikilinkRegex matches [[target]] or [[target|display]]
// The target cannot contain [ or ] to avoid matching array syntax like [[[ref]]]
var wikilinkRegex = regexp.MustCompile(`\[\[([^\]\[|]+)(?:\|([^\]]+))?\]\]`)

// ExtractRefs extracts references from content.
func ExtractRefs(content string, startLine int) []Reference {
	var refs []Reference

	lines := strings.Split(content, "\n")
	for lineOffset, line := range lines {
		lineNum := startLine + lineOffset

		matches := wikilinkRegex.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			if len(match) < 4 {
				continue
			}

			// Skip if preceded by [ (array syntax like [[[ref]]])
			if match[0] > 0 && line[match[0]-1] == '[' {
				continue
			}

			target := strings.TrimSpace(line[match[2]:match[3]])

			var displayText *string
			if match[4] >= 0 && match[5] >= 0 {
				dt := strings.TrimSpace(line[match[4]:match[5]])
				displayText = &dt
			}

			refs = append(refs, Reference{
				TargetRaw:   target,
				DisplayText: displayText,
				Line:        lineNum,
				Start:       match[0],
				End:         match[1],
			})
		}
	}

	return refs
}

// ExtractEmbeddedRefs parses embedded refs in trait values like [[[path/to/file]], [[other]]].
// Handles array syntax where refs are wrapped in extra brackets.
func ExtractEmbeddedRefs(value string) []string {
	var refs []string

	matches := wikilinkRegex.FindAllStringSubmatch(value, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			target := strings.TrimSpace(match[1])
			// For embedded refs, we DO want to match inside arrays
			// Just make sure the target doesn't start with [
			if !strings.HasPrefix(target, "[") {
				refs = append(refs, target)
			}
		}
	}

	return refs
}
