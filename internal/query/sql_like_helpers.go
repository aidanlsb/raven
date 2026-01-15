package query

import "strings"

// escapeLikePattern escapes special characters for LIKE pattern matching.
func escapeLikePattern(s string) string {
	// Escape backslash first, then % and _
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
