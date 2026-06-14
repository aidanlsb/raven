// Package cli implements the command-line interface.
// This file provides pipe-friendly output helpers for commands that return lists.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/aidanlsb/raven/internal/ui"
)

// PipeableItem represents an item that can be output in pipe-friendly format.
// Commands that return lists should use this for consistent pipe/picker integration.
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
	return !term.IsTerminal(os.Stdout.Fd())
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
// This format works well with rvn pick and cut for downstream processing.
// The number prefix allows users to reference results by number.
func WritePipeableList(w io.Writer, items []PipeableItem) {
	for _, item := range items {
		fmt.Fprintln(w, formatPipeableItemLine(item))
	}
}

func formatPipeableItemLine(item PipeableItem) string {
	content := sanitizePipeField(item.Content)
	location := sanitizePipeField(item.Location)
	return fmt.Sprintf("%d\t%s\t%s\t%s", item.Num, item.ID, content, location)
}

func sanitizePipeField(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	return strings.ReplaceAll(value, "\n", " ")
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
	return ui.TruncateWithEllipsis(content, maxLen)
}
