package ui

import "github.com/charmbracelet/lipgloss"

// Color palette
// - Default (white/black): Primary text
// - Accent (Moonlight #8fa8c8): Highlights, paths, interactive elements
// - Muted (gray): Secondary info, line numbers
// - No colored success/error/warning - use unicode symbols only

var (
	// Accent style for file paths, object references, highlights
	Accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#8fa8c8"))

	// Muted style for secondary info, hints, line numbers
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))

	// Bold style for emphasis
	Bold = lipgloss.NewStyle().Bold(true)

	// AccentBold combines accent color with bold
	AccentBold = lipgloss.NewStyle().Foreground(lipgloss.Color("#8fa8c8")).Bold(true)
)
