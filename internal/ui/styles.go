package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Minimal color palette: black, white, gray by default.
// Optional accent color can be configured via [ui].accent.
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

	// Accent style for optional user-configurable highlights.
	// Defaults to Bold with no color when accent is not configured.
	Accent = Bold

	accentColor string
)

// ConfigureTheme configures optional UI theme colors from config.
// Supported accent values:
//   - ANSI codes: "0" to "255"
//   - Hex colors: "#RRGGBB" or "#RGB"
//
// Special values "none", "off", and "default" disable the accent color.
func ConfigureTheme(accent string) {
	normalized, ok := normalizeAccentColor(accent)
	if !ok {
		accentColor = ""
		Accent = Bold
		return
	}

	accentColor = normalized
	Accent = lipgloss.NewStyle().Foreground(lipgloss.Color(normalized)).Bold(true)
}

// AccentColor returns the currently configured accent color, if any.
func AccentColor() (string, bool) {
	if accentColor == "" {
		return "", false
	}
	return accentColor, true
}

func normalizeAccentColor(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}

	switch strings.ToLower(value) {
	case "none", "off", "default":
		return "", false
	}

	if strings.HasPrefix(value, "#") {
		switch {
		case len(value) == 4 && isHexColor(value[1:]):
			// Expand #RGB to #RRGGBB
			return "#" + strings.Repeat(string(value[1]), 2) +
				strings.Repeat(string(value[2]), 2) +
				strings.Repeat(string(value[3]), 2), true
		case len(value) == 7 && isHexColor(value[1:]):
			return value, true
		default:
			return "", false
		}
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return "", false
	}
	if n < 0 || n > 255 {
		return "", false
	}
	return strconv.Itoa(n), true
}

func isHexColor(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
