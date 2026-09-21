package checksvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type Options struct {
	PathArg     string
	TypeFilter  string
	TraitFilter string
	Issues      string
	Exclude     string
	ErrorsOnly  bool
}

type Scope struct {
	Type  string
	Value string

	targetFiles []string
}

type RunResult struct {
	Scope             Scope
	FileCount         int
	ErrorCount        int
	WarningCount      int
	Issues            []check.Issue
	SchemaIssues      []check.SchemaIssue
	StaleWarningShown bool
	MissingRefs       []*check.MissingRef
	UndefinedTraits   []*check.UndefinedTrait
	ShortRefs         map[string]string
}

type issueCollector struct {
	issues       []check.Issue
	schemaIssues []check.SchemaIssue
	result       *RunResult
	include      map[check.IssueType]bool
	exclude      map[check.IssueType]bool
	errorsOnly   bool
	scope        *Scope
	docsByPath   map[string]*parser.ParsedDocument
}

func newIssueCollector(result *RunResult, include, exclude map[check.IssueType]bool, errorsOnly bool, scope *Scope) *issueCollector {
	return &issueCollector{
		result:     result,
		include:    include,
		exclude:    exclude,
		errorsOnly: errorsOnly,
		scope:      scope,
		docsByPath: make(map[string]*parser.ParsedDocument),
	}
}

func (c *issueCollector) setDocs(docs []*parser.ParsedDocument) {
	c.docsByPath = make(map[string]*parser.ParsedDocument, len(docs))
	for _, doc := range docs {
		if doc != nil && doc.FilePath != "" {
			c.docsByPath[doc.FilePath] = doc
		}
	}
}

func (c *issueCollector) addAll(issues []check.Issue) {
	for _, issue := range issues {
		c.add(issue)
	}
}

func (c *issueCollector) add(issue check.Issue) {
	if doc := c.docsByPath[issue.FilePath]; doc != nil && !isIssueInScope(issue, doc, c.scope) {
		return
	}
	if !shouldInclude(issue.Level, issue.Type, c.include, c.exclude, c.errorsOnly) {
		return
	}
	c.issues = append(c.issues, issue)
	c.count(issue.Level)
}

func (c *issueCollector) addSchemaAll(issues []check.SchemaIssue) {
	for _, issue := range issues {
		if !shouldInclude(issue.Level, issue.Type, c.include, c.exclude, c.errorsOnly) {
			continue
		}
		c.schemaIssues = append(c.schemaIssues, issue)
		c.count(issue.Level)
	}
}

func (c *issueCollector) count(level check.IssueLevel) {
	if level == check.LevelWarning {
		c.result.WarningCount++
	} else {
		c.result.ErrorCount++
	}
}

func (c *issueCollector) recordIncomplete(subsystem string, cause error) {
	c.add(check.Issue{
		Level:      check.LevelWarning,
		Type:       check.IssueCheckIncomplete,
		FilePath:   "",
		Line:       0,
		Message:    fmt.Sprintf("Check incomplete: %s unavailable: %v", subsystem, cause),
		Value:      subsystem,
		FixCommand: "rvn reindex",
		FixHint:    "Fix the named index subsystem and re-run check",
	})
}

type indexResources struct {
	db                *index.Database
	aliases           map[string]string
	duplicateAliases  []resolver.AliasCollision
	canonicalResolver *resolver.Resolver
}

func loadIndexResources(
	rt *vaultruntime.Runtime,
	scope *Scope,
	excludeMatcher *ravenignore.Matcher,
	collector *issueCollector,
	result *RunResult,
) *indexResources {
	resources := &indexResources{}

	if err := rt.OpenDB(); err != nil {
		collector.recordIncomplete("index", err)
		return resources
	}

	resources.db = rt.DB
	stalenessInfo, stalenessErr := rt.DB.CheckStaleness(rt.VaultPath)
	if stalenessErr != nil {
		collector.recordIncomplete("index staleness", stalenessErr)
	} else if stalenessInfo.IsStale {
		staleFiles := filterIncludedPaths(stalenessInfo.StaleFiles, excludeMatcher)
		staleCount := len(staleFiles)
		if staleCount > 0 && scope.Type == "full" {
			collector.add(check.Issue{
				Level:      check.LevelWarning,
				Type:       check.IssueStaleIndex,
				FilePath:   "",
				Line:       0,
				Message:    fmt.Sprintf("Index may be stale (%d file(s) modified since last reindex)", staleCount),
				FixCommand: "rvn reindex",
				FixHint:    "Run 'rvn reindex' to update the index",
			})
		}
		result.StaleWarningShown = staleCount > 0
	}

	var aliasesErr error
	resources.aliases, aliasesErr = rt.DB.AllAliases()
	if aliasesErr != nil {
		collector.recordIncomplete("aliases", aliasesErr)
	}

	var duplicateAliasesErr error
	resources.duplicateAliases, duplicateAliasesErr = rt.DB.FindDuplicateAliases()
	if duplicateAliasesErr != nil {
		collector.recordIncomplete("duplicate aliases", duplicateAliasesErr)
	}

	var resolverErr error
	resources.canonicalResolver, resolverErr = rt.DB.Resolver(indexschema.ResolverOptions{
		DailyDirectory: rt.VaultCfg.GetDailyDirectory(),
		Schema:         rt.Schema,
	})
	if resolverErr != nil {
		collector.recordIncomplete("resolver", resolverErr)
	}

	return resources
}

func Run(rt *vaultruntime.Runtime, opts Options) (*RunResult, error) {
	if rt == nil || rt.VaultCfg == nil || rt.Schema == nil {
		return nil, fmt.Errorf("vault runtime with config and schema is required")
	}
	scope, err := resolveScope(rt, opts)
	if err != nil {
		return nil, err
	}

	includeIssues, excludeIssues := parseIssueFilter(opts)
	excludeMatcher, err := ravenignore.NewMatcher(rt.VaultCfg.GetExcludePatterns())
	if err != nil {
		return nil, svcerr.ValidationError(fmt.Errorf("invalid exclude config: %w", err))
	}

	result := &RunResult{
		Scope: Scope{
			Type:  scope.Type,
			Value: scope.Value,
		},
	}
	collector := newIssueCollector(result, includeIssues, excludeIssues, opts.ErrorsOnly, scope)

	indexRes := loadIndexResources(rt, scope, excludeMatcher, collector, result)

	walked, walkErr := walkScopedDocuments(rt, scope, excludeMatcher)
	if walkErr != nil {
		return nil, walkErr
	}
	result.FileCount = walked.fileCount
	collector.setDocs(walked.docs)
	collector.addAll(walked.parseErrors)

	validator := newCheckValidator(rt, walked.objectInfos, indexRes)
	collector.addAll(detectDocumentIssues(walked.docs, validator))
	collector.addAll(detectMarkdownLinkToVaultNoteIssues(walked.docs, rt.VaultPath))
	collector.addAll(detectNonCanonicalIssues(walked.docs, rt.Schema, rt.VaultCfg))
	detectBrokenFileLinksFromIndex(indexRes, rt.VaultPath, excludeMatcher, scope, walked, collector)
	collector.addSchemaAll(detectSchemaIssues(validator, scope))

	result.Issues = collector.issues
	result.SchemaIssues = collector.schemaIssues
	result.MissingRefs = validator.MissingRefs()
	result.UndefinedTraits = validator.UndefinedTraits()
	result.ShortRefs = validator.ShortRefs()
	sortIssues(result.Issues)
	sortSchemaIssues(result.SchemaIssues)
	return result, nil
}

func resolveScope(rt *vaultruntime.Runtime, opts Options) (*Scope, error) {
	vaultPath := rt.VaultPath
	scope := &Scope{Type: "full"}
	if opts.TypeFilter != "" {
		scope.Type = "type_filter"
		scope.Value = opts.TypeFilter
		return scope, nil
	}
	if opts.TraitFilter != "" {
		scope.Type = "trait_filter"
		scope.Value = opts.TraitFilter
		return scope, nil
	}
	if strings.TrimSpace(opts.PathArg) == "" {
		return scope, nil
	}

	pathArg := opts.PathArg
	fullPath := filepath.Join(vaultPath, pathArg)
	if fileInfo, err := os.Stat(fullPath); err == nil && fileInfo.IsDir() {
		scope.Type = "directory"
		scope.Value = pathArg
		return scope, nil
	}

	filePath := fullPath
	if !strings.HasSuffix(filePath, ".md") {
		filePath = fullPath + ".md"
	}
	if fileInfo, err := os.Stat(filePath); err == nil && !fileInfo.IsDir() {
		scope.Type = "file"
		relPath, _ := filepath.Rel(vaultPath, filePath)
		scope.Value = relPath
		scope.targetFiles = []string{filePath}
		return scope, nil
	}

	resolved, err := refresolve.Resolve(pathArg, rt, false)
	if err != nil {
		return nil, svcerr.ValidationError(fmt.Errorf("could not resolve '%s': %w", pathArg, err))
	}

	scope.Type = "file"
	relPath, _ := filepath.Rel(vaultPath, resolved.FilePath)
	scope.Value = relPath
	scope.targetFiles = []string{resolved.FilePath}
	return scope, nil
}

func parseIssueFilter(opts Options) (include map[check.IssueType]bool, exclude map[check.IssueType]bool) {
	return parseIssueTypeSet(opts.Issues), parseIssueTypeSet(opts.Exclude)
}

func parseIssueTypeSet(raw string) map[check.IssueType]bool {
	out := make(map[check.IssueType]bool)
	if raw == "" {
		return out
	}
	for _, issueType := range strings.Split(raw, ",") {
		issueType = strings.TrimSpace(issueType)
		if issueType != "" {
			out[check.IssueType(issueType)] = true
		}
	}
	return out
}

func shouldInclude(level check.IssueLevel, issueType check.IssueType, include, exclude map[check.IssueType]bool, errorsOnly bool) bool {
	if errorsOnly && level == check.LevelWarning {
		return false
	}
	if len(include) > 0 && !include[issueType] {
		return false
	}
	if exclude[issueType] {
		return false
	}
	return true
}

func isIssueInScope(issue check.Issue, doc *parser.ParsedDocument, scope *Scope) bool {
	switch scope.Type {
	case "type_filter":
		for _, obj := range doc.Objects {
			if obj.Type == scope.Value {
				return true
			}
		}
		return false
	case "trait_filter":
		if issue.Type == check.IssueUndefinedTrait ||
			issue.Type == check.IssueInvalidTraitValue ||
			issue.Type == check.IssueMissingRequiredTrait {
			return issue.Value == scope.Value || strings.HasPrefix(issue.Value, scope.Value)
		}
		for _, trait := range doc.Traits {
			if trait.TraitType == scope.Value {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func sortIssues(issues []check.Issue) {
	sort.Slice(issues, func(i, j int) bool {
		a := issues[i]
		b := issues[j]
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Level != b.Level {
			return a.Level.String() < b.Level.String()
		}
		if a.Type != b.Type {
			return string(a.Type) < string(b.Type)
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return a.Message < b.Message
	})
}

func sortSchemaIssues(issues []check.SchemaIssue) {
	sort.Slice(issues, func(i, j int) bool {
		a := issues[i]
		b := issues[j]
		if a.Level != b.Level {
			return a.Level.String() < b.Level.String()
		}
		if a.Type != b.Type {
			return string(a.Type) < string(b.Type)
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return a.Message < b.Message
	})
}
