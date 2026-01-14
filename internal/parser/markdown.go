package parser

import (
	"github.com/aidanlsb/raven/internal/slugs"
)

// Heading represents a parsed heading.
type Heading struct {
	Level int
	Text  string
	Line  int // 1-indexed
}

// Slugify converts a heading text to a URL-friendly slug.
func Slugify(text string) string {
	return slugs.HeadingSlug(text)
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
