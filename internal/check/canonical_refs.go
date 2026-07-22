package check

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
)

// DetectNonCanonicalRefs reports references that include a configured
// filesystem root even though Raven object IDs omit that root.
func DetectNonCanonicalRefs(doc *parser.ParsedDocument, objectsRoot, pagesRoot string) []Issue {
	if doc == nil || len(doc.Refs) == 0 {
		return nil
	}

	roots := uniqueDirectoryRoots(objectsRoot, pagesRoot)
	if len(roots) == 0 {
		return nil
	}

	var issues []Issue
	for _, ref := range doc.Refs {
		if ref == nil {
			continue
		}
		raw := strings.TrimSpace(ref.TargetRaw)
		if raw == "" {
			continue
		}
		replacement, matched := stripDirectoryRoot(raw, roots)
		if !matched || replacement == "" {
			continue
		}
		issues = append(issues, Issue{
			Level:          LevelWarning,
			Type:           IssueNonCanonicalRef,
			FilePath:       doc.FilePath,
			Line:           ref.LineOrZero(),
			Message:        fmt.Sprintf("Reference [[%s]] includes the configured root prefix; canonical form is [[%s]]", raw, replacement),
			Value:          raw,
			FixHint:        fmt.Sprintf("Drop the configured root prefix: [[%s]]", replacement),
			FixReplacement: replacement,
		})
	}
	return issues
}

func uniqueDirectoryRoots(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	roots := make([]string, 0, len(values))
	for _, value := range values {
		root := paths.NormalizeDirRoot(value)
		if root == "" {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return roots
}

func stripDirectoryRoot(raw string, roots []string) (string, bool) {
	for _, root := range roots {
		if strings.HasPrefix(raw, root) {
			return strings.TrimPrefix(raw, root), true
		}
	}
	return raw, false
}
