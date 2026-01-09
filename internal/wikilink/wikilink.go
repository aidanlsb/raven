// Package wikilink provides canonical parsing/scanning of Raven wikilinks.
//
// Wikilink grammar:
//   [[target]]
//   [[target|display text]]
//
// Notes:
// - The target is trimmed of surrounding whitespace.
// - The display text (if present) is also trimmed.
// - This package intentionally does NOT understand markdown code fences; higher-level
//   parsers decide whether scanning is enabled for a given region.
package wikilink

import (
	"regexp"
	"strings"
)

// Match represents a wikilink found in a string (typically a single line).
type Match struct {
	Target      string
	DisplayText *string
	Start       int
	End         int
	Literal     string
}

// re matches [[target]] or [[target|display]].
// The target cannot contain [ or ] to avoid matching array syntax like [[[ref]]].
var re = regexp.MustCompile(`\[\[([^\]\[|]+)(?:\|([^\]]+))?\]\]`)

// ParseExact parses a string that is exactly a wikilink literal, returning its target and optional display text.
func ParseExact(s string) (target string, display *string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[[") || !strings.HasSuffix(s, "]]") {
		return "", nil, false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(s, "[["), "]]")
	parts := strings.SplitN(inner, "|", 2)
	target = strings.TrimSpace(parts[0])
	if target == "" {
		return "", nil, false
	}
	if len(parts) == 2 {
		d := strings.TrimSpace(parts[1])
		display = &d
	}
	return target, display, true
}

// FindAllInLine finds wikilinks in a single line.
//
// If allowTriple is false, matches preceded by '[' are skipped to avoid array syntax like [[[ref]]].
func FindAllInLine(line string, allowTriple bool) []Match {
	var out []Match

	matches := re.FindAllStringSubmatchIndex(line, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		start, end := m[0], m[1]

		// Skip if preceded by '[' (array syntax like [[[ref]]]) unless allowTriple is enabled.
		if !allowTriple && start > 0 && line[start-1] == '[' {
			continue
		}

		target := strings.TrimSpace(line[m[2]:m[3]])
		if target == "" {
			continue
		}

		var display *string
		if len(m) >= 6 && m[4] >= 0 && m[5] >= 0 {
			d := strings.TrimSpace(line[m[4]:m[5]])
			display = &d
		}

		out = append(out, Match{
			Target:      target,
			DisplayText: display,
			Start:       start,
			End:         end,
			Literal:     line[start:end],
		})
	}

	return out
}

// ScanAt scans a wikilink starting at `start` in `input`.
// `start` must point at the first '[' of a "[[" sequence.
// Returns the end offset (exclusive), target, literal, and ok.
func ScanAt(input string, start int) (end int, target string, literal string, ok bool) {
	if start < 0 || start+1 >= len(input) {
		return 0, "", "", false
	}
	if input[start] != '[' || input[start+1] != '[' {
		return 0, "", "", false
	}

	i := start + 2
	for i+1 < len(input) {
		if input[i] == ']' && input[i+1] == ']' {
			end = i + 2
			literal = input[start:end]
			t, _, parsed := ParseExact(literal)
			if !parsed {
				// If parsing fails (e.g., empty target), still return ok=false.
				return 0, "", "", false
			}
			return end, t, literal, true
		}
		i++
	}
	return 0, "", "", false
}

