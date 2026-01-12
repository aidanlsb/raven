package ui

import "github.com/charmbracelet/lipgloss"

// Minimal color palette: black, white, gray only.
// Uses ANSI colors for terminal theme compatibility.
//
// - Default: Primary text (terminal foreground)
// - Muted (8 = Bright Black/Gray): Secondary info, hints, line numbers
// - Bold: Emphasis, highlights
// - No colored success/error/warning - use unicode symbols only

var (
	// Muted style for secondary info, hints, line numbers
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Bright Black (gray)

	// Bold style for emphasis and highlights
	Bold = lipgloss.NewStyle().Bold(true)
)
