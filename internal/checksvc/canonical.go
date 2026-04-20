package checksvc

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

// detectNonCanonicalIssues scans parsed documents for files that live outside
// the configured directory roots (non_canonical_path) and references that
// include the configured root prefix unnecessarily (non_canonical_ref).
//
// Returns issues only when directory roots are configured. Files inside trash,
// templates, daily, or protected prefixes are exempt from path checks.
func detectNonCanonicalIssues(
	docs []*parser.ParsedDocument,
	sch *schema.Schema,
	vaultCfg *config.VaultConfig,
) []check.Issue {
	if vaultCfg == nil || !vaultCfg.HasDirectoriesConfig() {
		return nil
	}

	objectsRoot := paths.NormalizeDirRoot(vaultCfg.GetObjectsRoot())
	pagesRoot := paths.NormalizeDirRoot(vaultCfg.GetPagesRoot())

	exempt := exemptDirs(vaultCfg)

	var issues []check.Issue
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		issues = append(issues, detectNonCanonicalPath(doc, sch, objectsRoot, pagesRoot, exempt)...)
		issues = append(issues, detectNonCanonicalRefs(doc, objectsRoot, pagesRoot)...)
	}
	return issues
}

// exemptDirs returns the set of directory prefixes (each ending in "/") whose
// contents are exempt from non_canonical_path detection. Includes hard-protected
// prefixes (.raven/, .trash/, .git/), daily/template directories, and any
// user-configured protected prefixes.
func exemptDirs(vaultCfg *config.VaultConfig) []string {
	prefixes := []string{".raven/", ".trash/", ".git/"}
	if dir := paths.NormalizeDirRoot(vaultCfg.GetDailyDirectory()); dir != "" {
		prefixes = append(prefixes, dir)
	}
	if dir := paths.NormalizeDirRoot(vaultCfg.GetTemplateDirectory()); dir != "" {
		prefixes = append(prefixes, dir)
	}
	for _, raw := range vaultCfg.ProtectedPrefixes {
		if dir := paths.NormalizeDirRoot(raw); dir != "" {
			prefixes = append(prefixes, dir)
		}
	}
	return prefixes
}

// detectNonCanonicalPath flags file-level objects whose file location is not
// under the configured root for their kind. Embedded objects (those with a
// fragment in their ID) are skipped — they are validated through their parent
// file. When a canonical destination can be computed unambiguously, the issue
// is annotated with FixCommand and FixHint pointing at the canonical path.
func detectNonCanonicalPath(
	doc *parser.ParsedDocument,
	sch *schema.Schema,
	objectsRoot, pagesRoot string,
	exempt []string,
) []check.Issue {
	if doc.FilePath == "" {
		return nil
	}
	relPath := paths.NormalizeVaultRelPath(doc.FilePath)
	if relPath == "" {
		return nil
	}

	for _, prefix := range exempt {
		if strings.HasPrefix(relPath, prefix) {
			return nil
		}
	}

	fileObj := primaryFileObject(doc)
	if fileObj == nil {
		return nil
	}

	expectedRoot, isPage := expectedRootForType(fileObj.ObjectType, objectsRoot, pagesRoot)
	if expectedRoot == "" {
		return nil
	}
	if strings.HasPrefix(relPath, expectedRoot) {
		return nil
	}

	canonicalPath, canFix := canonicalDestinationPath(relPath, fileObj.ObjectType, isPage, expectedRoot, sch)

	displayValue := relPath
	message := fmt.Sprintf("File %q is outside the configured root %q for type %q", relPath, expectedRoot, displayType(fileObj.ObjectType))
	fixHint := fmt.Sprintf("Move file under %q to match the configured directories", expectedRoot)
	fixCommand := ""
	if canFix && canonicalPath != relPath {
		message = fmt.Sprintf("File %q is outside the configured root %q (canonical: %q)", relPath, expectedRoot, canonicalPath)
		fixHint = fmt.Sprintf("Move to %q (run 'rvn check fix --confirm' to apply)", canonicalPath)
		fixCommand = "rvn check fix --issues non_canonical_path --confirm"
		displayValue = relPath + " -> " + canonicalPath
	}

	return []check.Issue{{
		Level:      check.LevelError,
		Type:       check.IssueNonCanonicalPath,
		FilePath:   relPath,
		Line:       1,
		Message:    message,
		Value:      displayValue,
		FixCommand: fixCommand,
		FixHint:    fixHint,
	}}
}

// primaryFileObject returns the file-level object for a parsed document, if
// any. File-level objects are those without a parent (no embedded fragment).
// Returns nil when a document only contains embedded objects.
func primaryFileObject(doc *parser.ParsedDocument) *parser.ParsedObject {
	for _, obj := range doc.Objects {
		if obj == nil {
			continue
		}
		if obj.ParentID != nil {
			continue
		}
		if strings.Contains(obj.ID, "#") {
			continue
		}
		return obj
	}
	return nil
}

// expectedRootForType reports the configured root directory under which the
// given object type should live, plus whether the type is treated as an untyped
// page. Empty objectType is treated as a page. Returns ("", _) when no root
// applies (e.g. flat vault for that kind).
func expectedRootForType(objectType, objectsRoot, pagesRoot string) (string, bool) {
	isPage := objectType == "" || objectType == "page"
	if isPage {
		return pagesRoot, true
	}
	return objectsRoot, false
}

// canonicalDestinationPath computes where a file SHOULD live to be canonical,
// given its current path, type, and the configured roots. Returns (path, true)
// when the destination can be derived unambiguously, or ("", false) when the
// current layout is too ambiguous to auto-fix safely.
//
// For untyped pages: the canonical destination is "<pagesRoot><filename>" —
// nested page directories are not auto-flattened.
//
// For typed objects with a default_path: the canonical destination is
// "<objectsRoot><tail>" where <tail> is the path starting at the type's
// default_path within the current path. If default_path is not present in
// the current path, returns false.
//
// For typed objects without a default_path: the canonical destination is
// "<objectsRoot><filename>".
func canonicalDestinationPath(
	relPath, objectType string,
	isPage bool,
	expectedRoot string,
	sch *schema.Schema,
) (string, bool) {
	if isPage {
		filename := filepath.Base(relPath)
		return expectedRoot + filename, true
	}

	defaultPath := ""
	if sch != nil {
		if typeDef, ok := sch.Types[objectType]; ok && typeDef != nil {
			defaultPath = paths.NormalizeDirRoot(typeDef.DefaultPath)
		}
	}

	if defaultPath != "" {
		searchKey := "/" + defaultPath
		idx := strings.Index(relPath, searchKey)
		if idx == -1 {
			if strings.HasPrefix(relPath, defaultPath) {
				return expectedRoot + relPath, true
			}
			return "", false
		}
		tail := relPath[idx+1:]
		return expectedRoot + tail, true
	}

	filename := filepath.Base(relPath)
	return expectedRoot + filename, true
}

// detectNonCanonicalRefs scans refs in a document for wikilink targets that
// include the configured root prefix (e.g. "[[type/person/john]]"). These
// resolve correctly today via the literal_path fallback in readsvc, but the
// canonical form drops the root. Each finding includes a FixHint and Value
// suitable for the wikilink fix path in ApplyFixes.
func detectNonCanonicalRefs(doc *parser.ParsedDocument, objectsRoot, pagesRoot string) []check.Issue {
	if len(doc.Refs) == 0 {
		return nil
	}

	roots := uniqueNonEmpty(objectsRoot, pagesRoot)
	if len(roots) == 0 {
		return nil
	}

	var issues []check.Issue
	for _, ref := range doc.Refs {
		if ref == nil {
			continue
		}
		raw := strings.TrimSpace(ref.TargetRaw)
		if raw == "" {
			continue
		}
		stripped, matched := stripRootPrefix(raw, roots)
		if !matched || stripped == "" {
			continue
		}
		issues = append(issues, check.Issue{
			Level:    check.LevelWarning,
			Type:     check.IssueNonCanonicalRef,
			FilePath: doc.FilePath,
			Line:     ref.Line,
			Message:  fmt.Sprintf("Reference [[%s]] includes the configured root prefix; canonical form is [[%s]]", raw, stripped),
			Value:    raw,
			FixHint:  fmt.Sprintf("Drop the configured root prefix: [[%s]]", stripped),
		})
	}
	return issues
}

// stripRootPrefix removes any matching directory-root prefix from a wikilink
// target. Roots are tried in order; the first match wins. Returns the stripped
// value and true on match; the original value and false otherwise.
func stripRootPrefix(raw string, roots []string) (string, bool) {
	for _, root := range roots {
		if root == "" {
			continue
		}
		if strings.HasPrefix(raw, root) {
			return strings.TrimPrefix(raw, root), true
		}
	}
	return raw, false
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func displayType(objectType string) string {
	if objectType == "" {
		return "page"
	}
	return objectType
}
