package index

import "github.com/aidanlsb/raven/internal/indexschema"

// BuildFTSSearchQuery builds a safe FTS5 MATCH query that searches both the
// `title` and `content` columns. It is intended for rvn search (the general
// full-text search command) where users expect to find results by title as well
// as body content.
//
// The returned string is meant to be passed as the RHS of `fts_content MATCH ?`.
func BuildFTSSearchQuery(userQuery string) string {
	return indexschema.BuildFTSSearchQuery(userQuery)
}

// BuildFTSContentQuery builds a safe FTS5 MATCH query that scopes to the `content`
// column and avoids common parser footguns with hyphenated tokens.
//
// It is intended for the query language `content("...")` predicate, which
// explicitly searches body content only (not titles).
//
// The returned string is meant to be passed as the RHS of `fts_content MATCH ?`.
func BuildFTSContentQuery(userQuery string) string {
	return indexschema.BuildFTSContentQuery(userQuery)
}
