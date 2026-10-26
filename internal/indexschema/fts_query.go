package indexschema

import (
	"strings"
	"unicode"
)

// BuildFTSSearchQuery builds a safe FTS5 MATCH query that searches both the
// `title` and `content` columns. It is intended for rvn search (the general
// full-text search command) where users expect to find results by title as well
// as body content.
//
// The returned string is meant to be passed as the RHS of `fts_content MATCH ?`.
func BuildFTSSearchQuery(userQuery string) string {
	q := strings.TrimSpace(userQuery)
	if q == "" {
		return `{title content}:""`
	}
	return "{title content}: (" + sanitizeFTSQuery(q) + ")"
}

// BuildFTSContentQuery builds a safe FTS5 MATCH query that scopes to the `content`
// column and avoids common parser footguns with hyphenated tokens.
//
// It is intended for the query language `content("...")` predicate, which
// explicitly searches body content only (not titles).
//
// The returned string is meant to be passed as the RHS of `fts_content MATCH ?`.
func BuildFTSContentQuery(userQuery string) string {
	q := strings.TrimSpace(userQuery)
	if q == "" {
		// Match nothing (FTS phrase query for empty string).
		return `content:""`
	}

	// Wrap the entire expression so the column scope applies to boolean ops.
	// (Without parentheses, `content: a OR b` scopes only `a` to the column.)
	return "content: (" + sanitizeFTSQuery(q) + ")"
}

type ftsEmitKind int

const (
	ftsEmitNone ftsEmitKind = iota
	ftsEmitTerm
	ftsEmitOpen
	ftsEmitClose
	ftsEmitOperator
)

// sanitizeFTSQuery quotes unquoted literal tokens so SQLite FTS5 cannot treat
// them as query syntax.
//
// Callers already wrap the expression in a column scope (`content: (...)` or
// `{title content}: (...)`). Unquoted tokens such as `type:project`,
// `https://github.com`, `-todo`, and `@todo` would otherwise be column
// filters, unary NOT, or other special forms. Bare words are quoted too, and
// a term that would otherwise sit against `(` is joined with explicit AND:
// FTS5 parses `foo (bar)` as a function call, while `"foo" AND ("bar")` is a
// safe implicit AND of two phrases.
//
// Quoted phrases stay intact. Boolean operators, NEAR, grouping parentheses,
// and clean prefix queries (`foo*`) are preserved.
func sanitizeFTSQuery(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 16)

	last := ftsEmitNone
	pendingWS := ""
	inQuotes := false
	i := 0

	needAND := func(next ftsEmitKind) bool {
		switch next {
		case ftsEmitOpen:
			return last == ftsEmitTerm || last == ftsEmitClose
		case ftsEmitTerm:
			return last == ftsEmitClose
		default:
			return false
		}
	}
	emit := func(kind ftsEmitKind, text string) {
		if needAND(kind) {
			b.WriteString(" AND ")
			pendingWS = ""
		} else {
			b.WriteString(pendingWS)
			pendingWS = ""
		}
		b.WriteString(text)
		last = kind
	}

	for i < len(q) {
		c := q[i]

		if c == '"' {
			if inQuotes {
				b.WriteByte(c)
				inQuotes = false
				last = ftsEmitTerm
				i++
				continue
			}
			emit(ftsEmitTerm, `"`)
			inQuotes = true
			i++
			continue
		}

		if inQuotes {
			b.WriteByte(c)
			i++
			continue
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			pendingWS += string(c)
			i++
			continue
		}

		if c == '(' {
			emit(ftsEmitOpen, "(")
			i++
			continue
		}
		if c == ')' {
			emit(ftsEmitClose, ")")
			i++
			continue
		}

		// Consume a token until whitespace or grouping punctuation. Parentheses
		// attached to a token (for example `content()`) are treated as literal
		// punctuation so users can search for code-like strings.
		start := i
		hasLiteralParen := false
		for i < len(q) {
			cc := q[i]
			if cc == '"' || cc == ' ' || cc == '\t' || cc == '\n' || cc == '\r' {
				break
			}
			if cc == '(' {
				if i == start {
					break
				}
				hasLiteralParen = true
				i++
				continue
			}
			if cc == ')' {
				if hasLiteralParen {
					i++
					continue
				}
				break
			}
			i++
		}
		tok := q[start:i]

		if isFTSOperatorToken(tok) {
			emit(ftsEmitOperator, tok)
			continue
		}

		if shouldQuoteFTSLiteralToken(tok) {
			emit(ftsEmitTerm, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
			continue
		}

		emit(ftsEmitTerm, tok)
	}

	b.WriteString(pendingWS)
	return b.String()
}

func isFTSOperatorToken(tok string) bool {
	upper := strings.ToUpper(tok)
	switch upper {
	case "AND", "OR", "NOT", "NEAR":
		return true
	}
	if strings.HasPrefix(upper, "NEAR/") {
		n := upper[len("NEAR/"):]
		if n != "" && isAllDigits(n) {
			return true
		}
	}
	return false
}

func shouldQuoteFTSLiteralToken(tok string) bool {
	if tok == "" {
		return false
	}
	// Prefix queries (`foo*`) are intentional FTS5 syntax. Quote only when the
	// rest of the token still contains non-word characters.
	if strings.HasSuffix(tok, "*") && len(tok) > 1 {
		body := tok[:len(tok)-1]
		for _, r := range body {
			if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' {
				return true
			}
		}
		return false
	}
	// Quote every other literal token, including bare words. FTS5 treats
	// `foo (bar)` as a function call; sanitizeFTSQuery quotes the words and
	// inserts AND so the MATCH expression is `"foo" AND ("bar")`.
	return true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
