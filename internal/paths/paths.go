// Package paths provides canonical helpers for:
//   - Converting between vault-relative markdown file paths (e.g. "objects/people/freya.md")
//     and Raven object IDs (e.g. "people/freya")
//   - Validating paths are within the vault (security)
//   - Parsing embedded object IDs (e.g. "file#section")
//
// It also centralizes directory-root handling (objects/pages roots) so that
// parsing, CLI operations, watching, and resolution stay consistent.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// pathError is a simple error type for path-related errors.
type pathError string

func (e pathError) Error() string { return string(e) }

// ErrPathOutsideVault is returned when a path is outside the vault.
var ErrPathOutsideVault = errors.New("path is outside vault")

// MDExtension is the markdown file extension.
const MDExtension = ".md"

// HasMDExtension returns true if the path ends with .md.
func HasMDExtension(p string) bool {
	return strings.HasSuffix(p, MDExtension)
}

// EnsureMDExtension adds .md extension if not already present.
func EnsureMDExtension(p string) string {
	if HasMDExtension(p) {
		return p
	}
	return p + MDExtension
}

// TrimMDExtension removes the .md extension if present.
func TrimMDExtension(p string) string {
	return strings.TrimSuffix(p, MDExtension)
}

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
	id = TrimMDExtension(id)

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
	id = TrimMDExtension(id)

	objectsRoot = NormalizeDirRoot(objectsRoot)
	pagesRoot = NormalizeDirRoot(pagesRoot)

	// If the caller already provided a rooted path-like ID, keep it.
	if objectsRoot != "" && strings.HasPrefix(id, objectsRoot) {
		return EnsureMDExtension(id)
	}
	if pagesRoot != "" && strings.HasPrefix(id, pagesRoot) {
		return EnsureMDExtension(id)
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
		return EnsureMDExtension(root + id)
	}
	return EnsureMDExtension(id)
}

// ParseEmbeddedID parses an object ID that may contain an embedded fragment.
// For IDs like "file#section", it returns (fileID, fragment, true).
// For IDs without a fragment, it returns (id, "", false).
//
// This is the canonical function for parsing embedded object IDs.
// Use this instead of manually calling strings.SplitN(id, "#", 2).
func ParseEmbeddedID(id string) (fileID, fragment string, isEmbedded bool) {
	if idx := strings.Index(id, "#"); idx >= 0 {
		return id[:idx], id[idx+1:], true
	}
	return id, "", false
}

// ShortNameFromID extracts the short name from an object ID.
// For "people/freya" -> "freya"
// For "daily/2025-02-01#standup" -> "standup"
func ShortNameFromID(id string) string {
	fileID, fragment, isEmbedded := ParseEmbeddedID(id)
	if isEmbedded {
		return TrimMDExtension(fragment)
	}

	id = TrimMDExtension(fileID)
	return filepath.Base(id)
}

// CandidateFilePaths returns vault-relative markdown paths to try for a reference.
//
// It always includes the "literal" interpretation (ref + ".md" after stripping any
// ".md" suffix), plus rooted interpretations if roots are configured.
func CandidateFilePaths(ref, objectsRoot, pagesRoot string) []string {
	ref = normalizeRelPath(ref)
	ref = TrimMDExtension(ref)

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
	add(EnsureMDExtension(ref), &out)

	// 2) If roots are configured, also try rooted interpretations when ref is not already rooted.
	if objectsRoot != "" && !strings.HasPrefix(ref, objectsRoot) {
		add(EnsureMDExtension(objectsRoot+ref), &out)
	}
	if pagesRoot != "" && !strings.HasPrefix(ref, pagesRoot) {
		add(EnsureMDExtension(pagesRoot+ref), &out)
	}

	return out
}

// ValidateWithinVault checks that a target path is within the vault directory.
// Returns an error if the path would escape the vault (security check).
//
// This function:
// - Resolves both paths to absolute paths
// - Evaluates symlinks for security
// - Handles paths that don't exist yet by checking parent directories
func ValidateWithinVault(vaultPath, targetPath string) error {
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		return err
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	// Resolve any symlinks for security
	realVault, err := filepath.EvalSymlinks(absVault)
	if err != nil {
		realVault = absVault
	}

	resolvedTarget, err := resolveTargetPath(absTarget)
	if err != nil {
		return err
	}

	// Ensure target is within vault
	if !isWithinPath(realVault, resolvedTarget) {
		return ErrPathOutsideVault
	}

	return nil
}

// resolveTargetPath resolves symlinks for the nearest existing ancestor of absTarget,
// then reconstructs the full path. This prevents symlink escapes when the final
// path doesn't exist yet.
func resolveTargetPath(absTarget string) (string, error) {
	existing := absTarget
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}

	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(existing, absTarget)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return realExisting, nil
	}
	return filepath.Join(realExisting, rel), nil
}

func isWithinPath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// ProtectedPathConfig controls which vault-relative paths are considered
// system-managed and should not be modified by automation features.
//
// Callers should treat IsProtectedRelPath as an additional guardrail on top of
// ValidateWithinVault; it is not a security boundary, but it prevents foot-guns.
var hardProtectedPrefixes = []string{
	".raven/",
	".trash/",
	".git/",
}

var hardProtectedFiles = map[string]struct{}{
	"raven.yaml":  {},
	"schema.yaml": {},
}

// IsProtectedRelPath returns true if relPath (vault-relative) is protected.
//
// extraPrefixes are additional user-configured protected prefixes (vault-relative).
// They are treated as directory prefixes (normalized with NormalizeDirRoot).
func IsProtectedRelPath(relPath string, extraPrefixes []string) bool {
	p := normalizeRelPath(relPath)

	if _, ok := hardProtectedFiles[p]; ok {
		return true
	}

	for _, pref := range hardProtectedPrefixes {
		if strings.HasPrefix(p, pref) {
			return true
		}
	}

	for _, raw := range extraPrefixes {
		n := NormalizeDirRoot(raw)
		if n == "" {
			continue
		}
		if strings.HasPrefix(p, n) {
			return true
		}
	}

	return false
}
