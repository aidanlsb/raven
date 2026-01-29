package ui

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// DefaultTermWidth is the fallback terminal width when detection fails.
const DefaultTermWidth = 120

// DisplayContext holds display parameters, auto-detecting terminal width.
// It is the single source of truth for display settings.
type DisplayContext struct {
	TermWidth int  // detected or fallback terminal width
	IsTTY     bool // whether stdout is a terminal
}

// NewDisplayContext creates a DisplayContext, auto-detecting terminal dimensions.
func NewDisplayContext() *DisplayContext {
	fd := os.Stdout.Fd()
	isTTY := term.IsTerminal(fd)

	width := DefaultTermWidth
	if isTTY {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			width = w
		}
	}

	return &DisplayContext{
		TermWidth: width,
		IsTTY:     isTTY,
	}
}

// NewDisplayContextWithWidth creates a DisplayContext with a fixed width (for testing).
func NewDisplayContextWithWidth(width int) *DisplayContext {
	return &DisplayContext{
		TermWidth: width,
		IsTTY:     true,
	}
}

// AvailableWidth returns the usable width after accounting for left margin.
func (d *DisplayContext) AvailableWidth(leftMargin int) int {
	return d.TermWidth - leftMargin
}
