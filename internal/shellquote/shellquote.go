package shellquote

import "strings"

// Quote wraps s in single quotes, escaping any internal single quotes.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuoteIfNeeded quotes strings that are likely to be interpreted by a shell.
func QuoteIfNeeded(s string) string {
	if strings.ContainsAny(s, "#[]()|!\"'") {
		return Quote(s)
	}
	return s
}
