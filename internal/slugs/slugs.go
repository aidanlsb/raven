// Package slugs provides canonical slugification helpers used across Raven.
//
// Important: There are *two* slugging strategies in Raven today:
//   - Heading slugs: used for section/fragment IDs generated from markdown headings.
//     These are historically derived using a conservative, ASCII-ish transformation.
//   - Path slugs: used for filenames/object IDs and path matching, built on gosimple/slug.
//
// This package centralizes both strategies so their implementations are not duplicated.
package slugs

import (
	"strings"
	"unicode"

	goslug "github.com/gosimple/slug"
)

// HeadingSlug converts a heading text to a URL-friendly slug.
//
// This preserves existing behavior previously implemented in parser.Slugify.
func HeadingSlug(text string) string {
	var result strings.Builder
	prevDash := false

	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			result.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == ':':
			// Convert separators (including colon) to dashes
			if !prevDash && result.Len() > 0 {
				result.WriteRune('-')
				prevDash = true
			}
		}
	}

	s := result.String()
	// Trim trailing dash
	return strings.TrimSuffix(s, "-")
}

// ComponentSlug converts a string to a URL-safe slug appropriate for file/path components.
//
// This preserves existing behavior previously implemented in pages.Slugify.
func ComponentSlug(s string) string {
	s = strings.TrimSuffix(s, ".md")
	slugged := goslug.Make(s)
	if slugged == "" {
		slugged = strings.ToLower(strings.ReplaceAll(s, " ", "-"))
	}
	return slugged
}

// PathSlug slugifies each component of a path.
//
// This preserves existing behavior previously implemented in pages.SlugifyPath:
// - Strips a trailing ".md"
// - Slugifies each "/"-separated component using ComponentSlug
// - For embedded IDs, slugifies both sides of "#": "daily/2025-02-01#Team Sync" -> "daily/2025-02-01#team-sync"
func PathSlug(path string) string {
	// Remove .md extension if present
	path = strings.TrimSuffix(path, ".md")

	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Handle embedded object IDs (file#id)
		if strings.Contains(part, "#") {
			subParts := strings.SplitN(part, "#", 2)
			parts[i] = ComponentSlug(subParts[0]) + "#" + ComponentSlug(subParts[1])
		} else {
			parts[i] = ComponentSlug(part)
		}
	}
	return strings.Join(parts, "/")
}
