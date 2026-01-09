// Package paths provides canonical helpers for converting between:
// - vault-relative markdown file paths (e.g. "objects/people/freya.md")
// - Raven object IDs (e.g. "people/freya")
//
// It also centralizes directory-root handling (objects/pages roots) so that
// parsing, CLI operations, watching, and resolution stay consistent.
package paths

import (
	"path/filepath"
	"strings"
)

// NormalizeDirRoot normalizes a directory root to have:
// - no leading slash
// - exactly one trailing slash (unless empty)
//
// Examples:
// - "/objects/" -> "objects/"
// - "objects"   -> "objects/"
// - ""          -> ""
func NormalizeDirRoot(root string) string {
	root = filepath.ToSlash(root)
	root = strings.Trim(root, "/")
	if root == "" {
		return ""
	}
	return root + "/"
}

// normalizeRelPath normalizes a vault-relative path-like value:
// - converts OS separators to '/'
// - trims leading "./" and leading "/"
// - collapses repeated '/'
func normalizeRelPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

// FilePathToObjectID converts a vault-relative file path to an object ID.
//
// It:
// - strips a trailing ".md"
// - normalizes path separators
// - strips the configured objects/pages root prefixes (objects first, then pages)
func FilePathToObjectID(filePath, objectsRoot, pagesRoot string) string {
	id := normalizeRelPath(filePath)
	id = strings.TrimSuffix(id, ".md")

	objectsRoot = NormalizeDirRoot(objectsRoot)
	pagesRoot = NormalizeDirRoot(pagesRoot)

	// Prefer stripping objects root first (typed objects tend to be more specific).
	if objectsRoot != "" && strings.HasPrefix(id, objectsRoot) {
		return strings.TrimPrefix(id, objectsRoot)
	}
	if pagesRoot != "" && strings.HasPrefix(id, pagesRoot) {
		return strings.TrimPrefix(id, pagesRoot)
	}
	return id
}

// ObjectIDToFilePath converts an object ID to a vault-relative markdown file path.
//
// If roots are configured, typeName is used to decide whether to prefix with
// pagesRoot or objectsRoot:
// - typeName == "" or "page" -> pagesRoot (falls back to objectsRoot if pagesRoot is empty)
// - otherwise                -> objectsRoot
//
// If the objectID already includes a configured root prefix, this function will
// not add another prefix.
func ObjectIDToFilePath(objectID, typeName, objectsRoot, pagesRoot string) string {
	id := normalizeRelPath(objectID)
	id = strings.TrimSuffix(id, ".md")

	objectsRoot = NormalizeDirRoot(objectsRoot)
	pagesRoot = NormalizeDirRoot(pagesRoot)

	// If the caller already provided a rooted path-like ID, keep it.
	if objectsRoot != "" && strings.HasPrefix(id, objectsRoot) {
		return id + ".md"
	}
	if pagesRoot != "" && strings.HasPrefix(id, pagesRoot) {
		return id + ".md"
	}

	root := ""
	if typeName == "" || typeName == "page" {
		root = pagesRoot
		if root == "" {
			root = objectsRoot
		}
	} else {
		root = objectsRoot
	}

	if root != "" {
		return root + id + ".md"
	}
	return id + ".md"
}

// CandidateFilePaths returns vault-relative markdown paths to try for a reference.
//
// It always includes the "literal" interpretation (ref + ".md" after stripping any
// ".md" suffix), plus rooted interpretations if roots are configured.
func CandidateFilePaths(ref, objectsRoot, pagesRoot string) []string {
	ref = normalizeRelPath(ref)
	ref = strings.TrimSuffix(ref, ".md")

	objectsRoot = NormalizeDirRoot(objectsRoot)
	pagesRoot = NormalizeDirRoot(pagesRoot)

	seen := make(map[string]struct{}, 4)
	add := func(p string, out *[]string) {
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		*out = append(*out, p)
	}

	var out []string

	// 1) Treat as literal relative path from vault root.
	add(ref+".md", &out)

	// 2) If roots are configured, also try rooted interpretations when ref is not already rooted.
	if objectsRoot != "" && !strings.HasPrefix(ref, objectsRoot) {
		add(objectsRoot+ref+".md", &out)
	}
	if pagesRoot != "" && !strings.HasPrefix(ref, pagesRoot) {
		add(pagesRoot+ref+".md", &out)
	}

	return out
}

