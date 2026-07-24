package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Minimal color palette with focused semantic accents.
// Uses ANSI colors for terminal theme compatibility.
//
// - Default: Primary text (terminal foreground)
// - Muted (8 = Bright Black/Gray): Secondary info, hints, line numbers
// - Bold: Emphasis, highlights
// - No colored success/error/warning - use unicode symbols only

var (
	// Muted style for secondary info, hints, line numbers
	Muted lipgloss.Style

	// Bold style for emphasis and highlights
	Bold lipgloss.Style

	// Accent style for semantic highlights. Raven uses fixed bold styling.
	Accent lipgloss.Style

	// Syntax style for code-like tokens and Raven syntax markers.
	Syntax lipgloss.Style

	// SyntaxSubtle style for supporting syntax values.
	SyntaxSubtle lipgloss.Style
)

func init() {
	ConfigureStyles()
}

// ConfigureStyles applies Raven's fixed terminal styles and honors NO_COLOR.
func ConfigureStyles() {
	if NoColorEnabled() {
		Muted = lipgloss.NewStyle()
		Bold = lipgloss.NewStyle()
		Accent = lipgloss.NewStyle()
		Syntax = lipgloss.NewStyle()
		SyntaxSubtle = lipgloss.NewStyle()
		return
	}

	Muted = mutedStyle()
	Bold = boldStyle()
	Accent = Bold
	Syntax = syntaxStyle()
	SyntaxSubtle = syntaxSubtleStyle()
}

// NoColorEnabled returns true when terminal color output should be suppressed.
func NoColorEnabled() bool {
	return os.Getenv("NO_COLOR") != ""
}

func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
}

func boldStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true)
}

func syntaxStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
}

func syntaxSubtleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
}
