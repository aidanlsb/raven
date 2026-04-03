package querysvc

import (
	"strings"
	"unicode"
)

// SplitInlineInvocation tokenizes one inline saved-query invocation like:
// "proj-todos raven" or `proj-todos project="raven app"`.
// Quotes are removed and backslash escapes are resolved (outside single quotes).
// Returns ok=false for invalid quoting/escaping.
func SplitInlineInvocation(s string) ([]string, bool) {
	var out []string
	var b strings.Builder

	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}

	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
				continue
			}
			b.WriteRune(r)
			continue
		}

		if inDouble {
			if r == '"' {
				inDouble = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			b.WriteRune(r)
			continue
		}

		switch {
		case r == '\\':
			escaped = true
		case r == '\'':
			inSingle = true
		case r == '"':
			inDouble = true
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
		}
	}

	if escaped || inSingle || inDouble {
		return nil, false
	}

	flush()
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
