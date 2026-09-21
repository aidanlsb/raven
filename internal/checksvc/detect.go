package checksvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/linktarget"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// DetectMissingRefs returns the page-style missing references found in the given
// files. It is scoped to those files (it does not walk the whole vault, unlike
// Run) and reuses the same validator/resolver the full check uses, so the
// resulting *check.MissingRef items carry the same type inference: certain from
// typed ref fields, inferred from path matching default_path, or unknown.
//
// Ambiguous references and stale fragments are intentionally not reported here,
// matching trackMissingRef semantics in the check validator.
//
// Detection requires the index to resolve reference targets. If the index is
// unavailable, detection is skipped (returns nil) rather than reporting false
// positives for targets that exist on disk but are not yet indexed.
func DetectMissingRefs(rt *vaultruntime.Runtime, relPaths ...string) ([]*check.MissingRef, error) {
	if rt == nil || rt.VaultCfg == nil || rt.Schema == nil || len(relPaths) == 0 {
		return nil, nil
	}
	vaultPath := rt.VaultPath

	if err := rt.OpenDB(); err != nil {
		return nil, nil //nolint:nilerr // unavailable index intentionally disables detection
	}
	db := rt.DB

	// index.Open creates an empty database if none exists. With an empty index no
	// targets can resolve, so every reference would look "missing". Skip detection
	// in that case rather than report false positives for an unindexed vault.
	if objectIDs, idErr := db.AllObjectIDs(); idErr != nil || len(objectIDs) == 0 {
		return nil, nil //nolint:nilerr // empty or unreadable index would create false positives
	}

	aliases, _ := db.AllAliases()
	canonicalResolver, _ := db.Resolver(indexschema.ResolverOptions{
		DailyDirectory: rt.VaultCfg.GetDailyDirectory(),
		Schema:         rt.Schema,
	})
	if canonicalResolver == nil {
		return nil, nil
	}

	validator := newCheckValidator(rt, nil, &indexResources{
		aliases:           aliases,
		canonicalResolver: canonicalResolver,
	})

	seen := make(map[string]struct{}, len(relPaths))
	for _, relPath := range relPaths {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			continue
		}
		absPath := filepath.Join(vaultPath, filepath.FromSlash(relPath))
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}

		content, readErr := os.ReadFile(absPath)
		if readErr != nil {
			continue
		}
		doc, parseErr := parser.ParseDocumentWithOptions(string(content), absPath, vaultPath, rt.ParseOptions)
		if parseErr != nil {
			continue
		}
		validator.ValidateDocument(doc)
	}

	return validator.MissingRefs(), nil
}

func newCheckValidator(rt *vaultruntime.Runtime, objectInfos []check.ObjectInfo, indexRes *indexResources) *check.Validator {
	objectsRoot, pagesRoot := directoryRoots(rt.VaultCfg)
	opts := check.Options{
		Schema:      rt.Schema,
		ObjectInfos: objectInfos,
		ObjectsRoot: objectsRoot,
		PagesRoot:   pagesRoot,
		DailyDir:    rt.VaultCfg.GetDailyDirectory(),
	}
	if indexRes != nil {
		opts.Aliases = indexRes.aliases
		opts.Resolver = indexRes.canonicalResolver
		opts.DuplicateAliases = indexRes.duplicateAliases
	}
	validator := check.New(opts)
	if indexRes == nil || indexRes.canonicalResolver == nil {
		validator.SetDailyDirectory(rt.VaultCfg.GetDailyDirectory())
	}
	return validator
}

func directoryRoots(vaultCfg *config.VaultConfig) (objectsRoot, pagesRoot string) {
	if vaultCfg != nil && vaultCfg.HasDirectoriesConfig() {
		return vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot()
	}
	return "", ""
}

func detectDocumentIssues(docs []*parser.ParsedDocument, validator *check.Validator) []check.Issue {
	var issues []check.Issue
	for _, doc := range docs {
		issues = append(issues, validator.ValidateDocument(doc)...)
	}
	return issues
}

func detectSchemaIssues(validator *check.Validator, scope *Scope) []check.SchemaIssue {
	if scope == nil {
		return nil
	}
	switch scope.Type {
	case "full", "type_filter", "trait_filter":
	default:
		return nil
	}

	var issues []check.SchemaIssue
	for _, issue := range validator.ValidateSchema() {
		if !isSchemaIssueInScope(issue, scope) {
			continue
		}
		issues = append(issues, issue)
	}
	return issues
}

func isSchemaIssueInScope(issue check.SchemaIssue, scope *Scope) bool {
	switch scope.Type {
	case "type_filter":
		return strings.Contains(issue.Value, scope.Value) || strings.HasPrefix(issue.Value, scope.Value+".")
	case "trait_filter":
		return issue.Value == scope.Value
	default:
		return true
	}
}

func detectMarkdownLinkToVaultNoteIssues(docs []*parser.ParsedDocument, vaultPath string) []check.Issue {
	var issues []check.Issue
	for _, doc := range docs {
		for _, link := range doc.MarkdownLinks {
			if !linktarget.IsRavenTargetAuthored(link.RawTarget, link.Target, doc.FilePath, vaultPath) {
				continue
			}
			targetInfo := linktarget.AnalyzeAuthored(link.RawTarget, link.Target, doc.FilePath, vaultPath)
			if !strings.EqualFold(targetInfo.Ext, "md") {
				continue
			}

			linkKind := "link"
			if link.IsImage {
				linkKind = "image"
			}
			issues = append(issues, check.Issue{
				Level:    check.LevelError,
				Type:     check.IssueMarkdownLinkToVaultNote,
				FilePath: doc.FilePath,
				Line:     link.Line,
				Message:  fmt.Sprintf("Markdown %s target %q points to a vault note but is not tracked as a Raven reference", linkKind, link.RawTarget),
				Value:    link.RawTarget,
				FixHint:  "Use a Raven wikilink/object reference (for example, [[target]]) so backlinks and moves can track it",
			})
		}
	}
	return issues
}

func detectBrokenFileLinksFromIndex(
	indexRes *indexResources,
	vaultPath string,
	excludeMatcher *ravenignore.Matcher,
	scope *Scope,
	walked scopedWalk,
	collector *issueCollector,
) {
	if indexRes == nil || indexRes.db == nil {
		return
	}
	fileLinks, linksErr := indexRes.db.FileLinks()
	if linksErr != nil {
		collector.recordIncomplete("file links", linksErr)
		return
	}
	collector.addAll(detectBrokenFileLinkIssues(fileLinks, vaultPath, excludeMatcher, scope, walked.walkPath, walked.targetFiles, walked.docs))
}

func detectBrokenFileLinkIssues(links []model.Link, vaultPath string, excludeMatcher *ravenignore.Matcher, scope *Scope, walkPath string, targetFileSet map[string]bool, docs []*parser.ParsedDocument) []check.Issue {
	docsByPath := make(map[string]*parser.ParsedDocument, len(docs))
	for _, doc := range docs {
		if doc != nil && doc.FilePath != "" {
			docsByPath[doc.FilePath] = doc
		}
	}

	var issues []check.Issue
	for _, link := range links {
		if excludeMatcher.Match(link.FilePath, false) {
			continue
		}
		sourcePath := filepath.Join(vaultPath, filepath.FromSlash(link.FilePath))
		if !isFileInScope(sourcePath, scope, walkPath, targetFileSet) {
			continue
		}

		issue := check.Issue{
			Level:    check.LevelError,
			Type:     check.IssueBrokenFileLink,
			FilePath: link.FilePath,
			Line:     link.Line,
			Message:  fmt.Sprintf("File link target %q does not exist", link.RawTarget),
			Value:    link.RawTarget,
			FixHint:  "Restore the target file or update/remove this Markdown link",
		}
		if doc := docsByPath[link.FilePath]; doc != nil {
			if !isIssueInScope(issue, doc, scope) {
				continue
			}
		} else if scope.Type == "type_filter" || scope.Type == "trait_filter" {
			continue
		}

		targetPath := linktarget.ResolveFileKey(link.NormalizedKey, vaultPath)
		if _, err := os.Stat(targetPath); err == nil || !os.IsNotExist(err) {
			continue
		}
		issues = append(issues, issue)
	}
	return issues
}

func filterIncludedPaths(paths []string, excludeMatcher *ravenignore.Matcher) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if excludeMatcher.Match(path, false) {
			continue
		}
		out = append(out, path)
	}
	return out
}
