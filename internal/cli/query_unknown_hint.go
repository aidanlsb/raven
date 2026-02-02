package cli

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

func buildUnknownQuerySuggestion(db *index.Database, queryStr string, dailyDir string, sch *schema.Schema) string {
	base := "Queries must start with 'object:' or 'trait:', or be a saved query name. Run 'rvn query --list' to see saved queries."

	q := strings.TrimSpace(queryStr)
	if !isSingleToken(q) {
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
