package ui

import "github.com/charmbracelet/lipgloss"

// Color palette
// - Default (white/black): Primary text
// - Accent (soft purple #A78BFA): Highlights, paths, interactive elements
// - Muted (gray): Secondary info, line numbers
// - No colored success/error/warning - use unicode symbols only

var (
	// Accent style for file paths, object references, highlights
	Accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))

	// Muted style for secondary info, hints, line numbers
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))

	// Bold style for emphasis
	Bold = lipgloss.NewStyle().Bold(true)

	// AccentBold combines accent color with bold
	AccentBold = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
)
