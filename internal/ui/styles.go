package ui

import "github.com/charmbracelet/lipgloss"

// Color palette using ANSI 0-15 colors for terminal theme compatibility.
// These colors adapt to the user's terminal theme (Dracula, Nord, Solarized, etc.)
//
// ANSI color reference:
//   0-7:   Black, Red, Green, Yellow, Blue, Magenta, Cyan, White
//   8-15:  Bright variants of the above
//
// - Default (white/black): Primary text
// - Accent (14 = Bright Cyan): Highlights, paths, interactive elements
// - Muted (8 = Bright Black/Gray): Secondary info, line numbers
// - No colored success/error/warning - use unicode symbols only

var (
	// Accent style for file paths, object references, highlights
	Accent = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // Bright Cyan

	// Muted style for secondary info, hints, line numbers
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Bright Black (gray)

	// Bold style for emphasis
	Bold = lipgloss.NewStyle().Bold(true)

	// AccentBold combines accent color with bold
	AccentBold = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
)
