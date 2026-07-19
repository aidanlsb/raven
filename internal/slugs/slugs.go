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
	"path/filepath"
	"strings"
	"unicode"

	goslug "github.com/gosimple/slug"

	"github.com/aidanlsb/raven/internal/paths"
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

// ComponentSlug converts a string to a URL-safe slug appropriate for a single
// file/path component.
//
// The result never contains path separators, so it is safe to use for turning a
// free-form display string (e.g. an object title) into one filename segment.
func ComponentSlug(s string) string {
	s = strings.TrimSuffix(s, ".md")
	slugged := goslug.Make(s)
	if slugged == "" {
		// Fallback for inputs goslug cannot slugify (e.g. only punctuation).
		// Replace separators—including path separators—so the result stays a
		// single safe component.
		replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-")
		slugged = strings.ToLower(replacer.Replace(s))
	}
	return slugged
}

// PathSlug slugifies each component of a path.
//
// - Strips a trailing ".md"
// - Slugifies each "/"-separated component using ComponentSlug
// - For section IDs, slugifies both sides of "#": "daily/2025-02-01#Team Sync" -> "daily/2025-02-01#team-sync"
func PathSlug(path string) string {
	// Remove .md extension if present
	path = strings.TrimSuffix(path, ".md")
	path = strings.ReplaceAll(filepath.ToSlash(path), "\\", "/")

	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Handle section IDs (file#id)
		if fileID, fragment, isSection := paths.ParseSectionID(part); isSection {
			parts[i] = ComponentSlug(fileID) + "#" + ComponentSlug(fragment)
		} else {
			parts[i] = ComponentSlug(part)
		}
	}
	return strings.Join(parts, "/")
}
