package index

import "strings"

// BuildFTSContentQuery builds a safe FTS5 MATCH query that scopes to the `content`
// column and avoids common parser footguns with hyphenated tokens.
//
// It is intended for:
// - Database.Search / Database.SearchWithType
// - Query language `content("...")` predicates
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

// sanitizeFTSQuery quotes unquoted tokens containing '-' to prevent SQLite FTS
// from interpreting them as operators (which can surface as "no such column" errors).
//
// This keeps quoted phrases intact and preserves boolean operators/parentheses.
func sanitizeFTSQuery(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)

	inQuotes := false
	i := 0
	for i < len(q) {
		c := q[i]

		// Toggle quoted phrase state; keep the quote.
		if c == '"' {
			inQuotes = !inQuotes
			b.WriteByte(c)
			i++
			continue
		}

		if inQuotes {
			b.WriteByte(c)
			i++
			continue
		}

		// Preserve whitespace as-is.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			b.WriteByte(c)
			i++
			continue
		}

		// Preserve grouping punctuation.
		if c == '(' || c == ')' {
			b.WriteByte(c)
			i++
			continue
		}

		// Consume a token until whitespace or paren.
		start := i
		for i < len(q) {
			cc := q[i]
			if cc == '"' || cc == '(' || cc == ')' || cc == ' ' || cc == '\t' || cc == '\n' || cc == '\r' {
				break
			}
			i++
		}
		tok := q[start:i]

		upper := strings.ToUpper(tok)
		switch upper {
		case "AND", "OR", "NOT", "NEAR":
			b.WriteString(tok)
			continue
		}

		// Don't rewrite column-scoped tokens like `content:foo`.
		if strings.Contains(tok, ":") {
			b.WriteString(tok)
			continue
		}

		// Quote hyphenated tokens (but avoid treating unary NOT `-foo` as a phrase).
		if strings.Contains(tok, "-") && !strings.HasPrefix(tok, "-") {
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(tok, `"`, `""`))
			b.WriteByte('"')
			continue
		}

		b.WriteString(tok)
	}

	return b.String()
}

