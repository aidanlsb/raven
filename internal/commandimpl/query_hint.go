package commandimpl

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/shellquote"
)

func isSingleToken(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t\r\n")
}

// buildUnknownQuerySuggestion produces a helpful suggestion for a query string
// that is neither a concrete query root nor a saved query. The database and
// schema are used only to enrich the hint (recognizing a type name or a
// resolvable reference); a nil database or schema degrades to the base message
// rather than changing behavior.
func buildUnknownQuerySuggestion(db *index.Database, queryStr, dailyDir string, sch *schema.Schema) string {
	base := "Queries must start with 'type:', 'trait:', 'section', or 'link', or be a saved query name. Run 'rvn query saved list' to see saved queries."

	q := strings.TrimSpace(queryStr)
	if !isSingleToken(q) {
		// Multi-token input looks like a query attempt rather than a bare
		// reference: prefer a syntax-specific hint (e.g. single quotes, SQL-style
		// where) when one applies, otherwise fall back to the query-root reminder.
		if hint, ok := querySyntaxHint(q); ok {
			return hint
		}
		return base
	}
	if sch != nil {
		if _, ok := sch.Types[q]; ok {
			return base + fmt.Sprintf(" Did you mean to query type %q? Try: %s", q, "rvn query type:"+q)
		}
	}

	if db == nil {
		return base
	}

	// Try to resolve the token as a reference to give a better hint. This does NOT
	// change behavior; it only improves the suggestion text.
	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: dailyDir,
		Schema:         sch,
	})
	if err != nil {
		return base
	}
	rr := res.Resolve(q)
	if rr.Ambiguous {
		return base + fmt.Sprintf(" Did you mean to resolve a reference? Try: %s", "rvn resolve "+shellquote.QuoteIfNeeded(q))
	}
	if rr.TargetID == "" {
		return base
	}

	// Looks like a valid reference.
	return base + fmt.Sprintf(" Did you mean to open/read an object reference? Try: %s or %s",
		"rvn read "+shellquote.QuoteIfNeeded(q),
		"rvn open "+shellquote.QuoteIfNeeded(q),
	)
}
