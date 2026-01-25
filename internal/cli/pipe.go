// Package cli implements the command-line interface.
// This file provides pipe-friendly output helpers for commands that return lists.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// PipeableItem represents an item that can be output in pipe-friendly format.
// Commands that return lists should use this for consistent pipe/fzf integration.
type PipeableItem struct {
	Num      int    // 1-indexed result number for reference
	ID       string // The unique identifier (used by downstream commands)
	Content  string // Human-readable description
	Location string // Short location hint (e.g., "daily/2026-01-25:42")
}

// pipeFormatOverride stores explicit --pipe/--no-pipe flag values.
// nil means use auto-detection.
var pipeFormatOverride *bool

// SetPipeFormat sets an explicit pipe format override.
// Pass nil to use auto-detection.
func SetPipeFormat(usePipe *bool) {
	pipeFormatOverride = usePipe
}

// IsPipedOutput returns true if stdout is being piped (not a TTY).
func IsPipedOutput() bool {
	return !isatty.IsTerminal(os.Stdout.Fd())
}

// ShouldUsePipeFormat returns true if output should use pipe-friendly format.
// Priority: explicit --pipe/--no-pipe flag > auto-detection based on TTY.
// JSON output mode always returns false (JSON has its own format).
func ShouldUsePipeFormat() bool {
	// JSON mode has its own format
	if isJSONOutput() {
		return false
	}

	// Explicit flag takes priority
	if pipeFormatOverride != nil {
		return *pipeFormatOverride
	}

	// Auto-detect based on TTY
	return IsPipedOutput()
}

// WritePipeableList writes items in pipe-friendly tab-separated format.
// Format: Num<tab>ID<tab>Content<tab>Location
// This format works well with fzf and cut for downstream processing.
// The number prefix allows users to reference results by number.
func WritePipeableList(w io.Writer, items []PipeableItem) {
	for _, item := range items {
		// Sanitize content - remove tabs and newlines
		content := strings.ReplaceAll(item.Content, "\t", " ")
		content = strings.ReplaceAll(content, "\n", " ")

		location := strings.ReplaceAll(item.Location, "\t", " ")

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", item.Num, item.ID, content, location)
	}
}

// WritePipeableIDs writes just the IDs, one per line.
// Useful for simpler piping when content/location aren't needed.
func WritePipeableIDs(w io.Writer, items []PipeableItem) {
	for _, item := range items {
		fmt.Fprintln(w, item.ID)
	}
}

// TruncateContent truncates content to a maximum length, adding "..." if truncated.
// Tries to break at word boundaries.
func TruncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	// Try to truncate at a word boundary
	truncated := content[:maxLen-3]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}
