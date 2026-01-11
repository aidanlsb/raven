package parser

import (
	"strings"
)

// FenceState tracks whether we're inside a fenced code block.
type FenceState struct {
	InFence  bool
	FenceCh  byte
	FenceLen int
}

// NormalizeFenceLine prepares a line for fence marker detection.
// It strips leading whitespace and blockquote prefixes so we can detect
// fenced code blocks in common markdown contexts.
func NormalizeFenceLine(line string) string {
	// Allow up to 3 leading spaces and handle blockquote prefixes (`>`),
	// so we can detect fenced code blocks in common markdown contexts.
	s := strings.TrimLeft(line, " \t")
	for strings.HasPrefix(s, ">") {
		s = strings.TrimPrefix(s, ">")
		s = strings.TrimLeft(s, " \t")
	}
	return s
}

// ParseFenceMarker checks if a line (after normalization) starts a code fence.
// Returns the fence character, fence length, and whether it's a valid fence.
func ParseFenceMarker(line string) (ch byte, n int, ok bool) {
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

// UpdateFenceState updates the fence state based on a line.
// Returns true if the line is a fence marker (opening or closing).
func (fs *FenceState) UpdateFenceState(line string) bool {
	fenceLine := NormalizeFenceLine(line)
	ch, n, ok := ParseFenceMarker(fenceLine)
	if !ok {
		return false
	}

	if !fs.InFence {
		// Opening a fence
		fs.InFence = true
		fs.FenceCh = ch
		fs.FenceLen = n
		return true
	}

	// Check if this closes the fence
	if fs.FenceCh == ch && n >= fs.FenceLen {
		fs.InFence = false
		fs.FenceCh = 0
		fs.FenceLen = 0
		return true
	}

	return false
}

// RemoveInlineCode removes inline code spans from a line, replacing them with spaces
// to preserve character positions for other parsing operations.
// Handles both single backticks (`code`) and double backticks (``code with `backtick` inside``).
func RemoveInlineCode(line string) string {
	result := []byte(line)
	i := 0

	for i < len(result) {
		if result[i] != '`' {
			i++
			continue
		}

		// Count opening backticks
		start := i
		openLen := 0
		for i < len(result) && result[i] == '`' {
			openLen++
			i++
		}

		// Find matching closing backticks
		found := false
		for j := i; j < len(result); j++ {
			if result[j] == '`' {
				// Count closing backticks
				closeLen := 0
				for j < len(result) && result[j] == '`' {
					closeLen++
					j++
				}

				if closeLen == openLen {
					// Found matching close - replace with spaces
					for k := start; k < j; k++ {
						result[k] = ' '
					}
					i = j
					found = true
					break
				}
			}
		}

		if !found {
			// No matching close found, continue from after opening backticks
			// (leave them as-is)
		}
	}

	return string(result)
}
