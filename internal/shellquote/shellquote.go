package shellquote

import (
	"fmt"
	"strings"
	"unicode"
)

// Quote wraps s in single quotes, escaping any internal single quotes.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Split tokenizes a command string with shell-style quoting. It does not
// expand variables, substitutions, or redirects, and it does not treat
// metacharacters such as `;`, `|`, `&`, or `$()` as syntax. Unclosed quotes
// or a trailing backslash return an error.
func Split(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty command")
	}

	var tokens []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	hasToken := false

	flush := func() {
		tokens = append(tokens, b.String())
		b.Reset()
		hasToken = false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			b.WriteByte(c)
			hasToken = true
			escaped = false
			continue
		}
		if !inSingle && c == '\\' {
			escaped = true
			hasToken = true
			continue
		}
		if inSingle {
			if c == '\'' {
				inSingle = false
			} else {
				b.WriteByte(c)
			}
			continue
		}
		if inDouble {
			if c == '"' {
				inDouble = false
			} else {
				b.WriteByte(c)
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
			hasToken = true
		case '"':
			inDouble = true
			hasToken = true
		default:
			if unicode.IsSpace(rune(c)) {
				if hasToken {
					flush()
				}
				continue
			}
			b.WriteByte(c)
			hasToken = true
		}
	}

	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in command")
	}
	if hasToken {
		flush()
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return tokens, nil
}

// QuoteIfNeeded quotes strings that are likely to be interpreted by a shell.
func QuoteIfNeeded(s string) string {
	if strings.ContainsAny(s, "#[]()|!\"'") {
		return Quote(s)
	}
	return s
}
