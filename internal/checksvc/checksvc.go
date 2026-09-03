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
	"github.com/aidanlsb/raven/internal/linktarget"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
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

type CheckIssueJSON struct {
	Type       string `json:"type"`
	Level      string `json:"level"`
	FilePath   string `json:"file_path"`
	Line       int    `json:"line"`
	Message    string `json:"message"`
	Value      string `json:"value,omitempty"`
	FixCommand string `json:"fix_command,omitempty"`
	FixHint    string `json:"fix_hint,omitempty"`
}

type CheckSummaryJSON struct {
	IssueType    string   `json:"issue_type"`
	Count        int      `json:"count"`
	UniqueValues int      `json:"unique_values,omitempty"`
	FixCommand   string   `json:"fix_command,omitempty"`
	FixHint      string   `json:"fix_hint,omitempty"`
	TopValues    []string `json:"top_values,omitempty"`
}

type CheckScopeJSON struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

type CheckResultJSON struct {
	VaultPath  string             `json:"vault_path"`
	Scope      *CheckScopeJSON    `json:"scope,omitempty"`
	FileCount  int                `json:"file_count"`
	ErrorCount int                `json:"error_count"`
	WarnCount  int                `json:"warning_count"`
	Issues     []CheckIssueJSON   `json:"issues"`
	Summary    []CheckSummaryJSON `json:"summary"`
}

type issueCollector struct {
	issues       *[]check.Issue
	schemaIssues *[]check.SchemaIssue
	result       *RunResult
	include      map[check.IssueType]bool
	exclude      map[check.IssueType]bool
	errorsOnly   bool
}

func (c *issueCollector) appendFiltered(issue check.Issue) {
	if !shouldIncludeIssue(issue, c.include, c.exclude, c.errorsOnly) {
		return
	}
	*c.issues = append(*c.issues, issue)
	if issue.Level == check.LevelWarning {
		c.result.WarningCount++
	} else {
		c.result.ErrorCount++
	}
}

func (c *issueCollector) appendSchemaFiltered(issue check.SchemaIssue) {
	if !shouldIncludeSchemaIssue(issue, c.include, c.exclude, c.errorsOnly) {
		return
	}
	*c.schemaIssues = append(*c.schemaIssues, issue)
	if issue.Level == check.LevelWarning {
		c.result.WarningCount++
	} else {
		c.result.ErrorCount++
	}
}

func (c *issueCollector) recordIncomplete(subsystem string, cause error) {
	c.appendFiltered(check.Issue{
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
			collector.appendFiltered(check.Issue{
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

type scopeWalkSetup struct {
	walkPath      string
	targetFileSet map[string]bool
}

func prepareScopeWalkSetup(vaultPath string, scope *Scope) scopeWalkSetup {
	setup := scopeWalkSetup{
		walkPath:      vaultPath,
		targetFileSet: make(map[string]bool),
	}
	switch scope.Type {
	case "file":
		for _, f := range scope.targetFiles {
			setup.targetFileSet[f] = true
		}
	case "directory":
		setup.walkPath = filepath.Join(vaultPath, scope.Value)
	}
	return setup
}

func Run(rt *vaultruntime.Runtime, opts Options) (*RunResult, error) {
	if rt == nil || rt.VaultCfg == nil || rt.Schema == nil {
		return nil, fmt.Errorf("vault runtime with config and schema is required")
	}
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	sch := rt.Schema
	scope, err := resolveScope(rt, opts)
	if err != nil {
		return nil, err
	}

	includeIssues, excludeIssues := parseIssueFilter(opts)
	excludeMatcher, err := ravenignore.NewMatcher(vaultCfg.GetExcludePatterns())
	if err != nil {
		return nil, svcerr.ValidationError(fmt.Errorf("invalid exclude config: %w", err))
	}

	result := &RunResult{
		Scope: Scope{
			Type:  scope.Type,
			Value: scope.Value,
		},
	}

	var allDocs []*parser.ParsedDocument
	var allObjectInfos []check.ObjectInfo
	var allIssues []check.Issue
	var parseErrors []check.Issue
	var schemaIssues []check.SchemaIssue

	collector := &issueCollector{
		issues:       &allIssues,
		schemaIssues: &schemaIssues,
		result:       result,
		include:      includeIssues,
		exclude:      excludeIssues,
		errorsOnly:   opts.ErrorsOnly,
	}

	// Check staleness + pull aliases from index when available.
	indexRes := loadIndexResources(rt, scope, excludeMatcher, collector, result)
	db := indexRes.db
	aliases := indexRes.aliases
	duplicateAliases := indexRes.duplicateAliases
	canonicalResolver := indexRes.canonicalResolver

	scopeSetup := prepareScopeWalkSetup(vaultPath, scope)
	walkPath := scopeSetup.walkPath
	targetFileSet := scopeSetup.targetFileSet

	walkOpts := &vault.WalkOptions{
		ParseOptions:   parseopts.FromVaultConfig(vaultCfg),
		ExcludeMatcher: excludeMatcher,
	}
	walkErr := vault.WalkMarkdownFilesWithOptions(vaultPath, walkOpts, func(walkResult vault.WalkResult) error {
		if walkResult.Error != nil {
			if isFileInScope(walkResult.Path, scope, walkPath, targetFileSet) {
				result.FileCount++
				parseErrors = append(parseErrors, check.Issue{
					Level:    check.LevelError,
					Type:     check.IssueParseError,
					FilePath: walkResult.RelativePath,
					Line:     1,
					Message:  walkResult.Error.Error(),
					FixHint:  "Fix the YAML frontmatter or markdown syntax",
				})
			}
			return nil
		}

		for _, obj := range walkResult.Document.Objects {
			allObjectInfos = append(allObjectInfos, check.ObjectInfo{ID: obj.ID, Type: obj.Type})
		}

		if isFileInScope(walkResult.Path, scope, walkPath, targetFileSet) {
			result.FileCount++
			allDocs = append(allDocs, walkResult.Document)
		}

		return nil
	})
	if walkErr != nil {
		return nil, svcerr.ValidationError(fmt.Errorf("error walking vault: %w", walkErr))
	}

	var objectsRoot, pagesRoot string
	if vaultCfg.HasDirectoriesConfig() {
		objectsRoot = vaultCfg.GetObjectsRoot()
		pagesRoot = vaultCfg.GetPagesRoot()
	}
	validator := check.New(check.Options{
		Schema:           sch,
		ObjectInfos:      allObjectInfos,
		Aliases:          aliases,
		Resolver:         canonicalResolver,
		DuplicateAliases: duplicateAliases,
		ObjectsRoot:      objectsRoot,
		PagesRoot:        pagesRoot,
		DailyDir:         vaultCfg.GetDailyDirectory(),
	})
	if canonicalResolver == nil {
		validator.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	}

	for _, doc := range allDocs {
		issues := validator.ValidateDocument(doc)
		for _, issue := range issues {
			if !isIssueInScope(issue, doc, scope) {
				continue
			}
			collector.appendFiltered(issue)
		}
	}

	for _, issue := range detectMarkdownLinkToVaultNoteIssues(allDocs, vaultPath) {
		doc := docByPath(allDocs, issue.FilePath)
		if doc != nil && !isIssueInScope(issue, doc, scope) {
			continue
		}
		collector.appendFiltered(issue)
	}

	for _, issue := range detectNonCanonicalIssues(allDocs, sch, vaultCfg) {
		doc := docByPath(allDocs, issue.FilePath)
		if doc != nil && !isIssueInScope(issue, doc, scope) {
			continue
		}
		collector.appendFiltered(issue)
	}

	if db != nil {
		fileLinks, linksErr := db.FileLinks()
		if linksErr != nil {
			collector.recordIncomplete("file links", linksErr)
		} else {
			for _, issue := range detectBrokenFileLinkIssues(fileLinks, vaultPath, excludeMatcher, scope, walkPath, targetFileSet, allDocs) {
				collector.appendFiltered(issue)
			}
		}
	}

	for _, pe := range parseErrors {
		if shouldIncludeIssue(pe, includeIssues, excludeIssues, opts.ErrorsOnly) {
			allIssues = append([]check.Issue{pe}, allIssues...)
			if pe.Level == check.LevelWarning {
				result.WarningCount++
			} else {
				result.ErrorCount++
			}
		}
	}

	if scope.Type == "full" || scope.Type == "type_filter" || scope.Type == "trait_filter" {
		rawSchemaIssues := validator.ValidateSchema()
		for _, issue := range rawSchemaIssues {
			if scope.Type == "type_filter" {
				if !strings.Contains(issue.Value, scope.Value) && !strings.HasPrefix(issue.Value, scope.Value+".") {
					continue
				}
			}
			if scope.Type == "trait_filter" && issue.Value != scope.Value {
				continue
			}
			collector.appendSchemaFiltered(issue)
		}
	}

	result.Issues = allIssues
	result.SchemaIssues = schemaIssues
	result.MissingRefs = validator.MissingRefs()
	result.UndefinedTraits = validator.UndefinedTraits()
	result.ShortRefs = validator.ShortRefs()
	sort.Slice(result.Issues, func(i, j int) bool {
		a := result.Issues[i]
		b := result.Issues[j]
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
	sort.Slice(result.SchemaIssues, func(i, j int) bool {
		a := result.SchemaIssues[i]
		b := result.SchemaIssues[j]
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
	return result, nil
}

func BuildJSON(vaultPath string, result *RunResult) CheckResultJSON {
	jsonResult := CheckResultJSON{
		VaultPath:  vaultPath,
		FileCount:  result.FileCount,
		ErrorCount: result.ErrorCount,
		WarnCount:  result.WarningCount,
		Issues:     make([]CheckIssueJSON, 0, len(result.Issues)+len(result.SchemaIssues)),
	}
	if result.Scope.Type != "" && result.Scope.Type != "full" {
		jsonResult.Scope = &CheckScopeJSON{
			Type:  result.Scope.Type,
			Value: result.Scope.Value,
		}
	}

	for _, issue := range result.Issues {
		jsonResult.Issues = append(jsonResult.Issues, CheckIssueJSON{
			Type:       string(issue.Type),
			Level:      issue.Level.String(),
			FilePath:   issue.FilePath,
			Line:       issue.Line,
			Message:    issue.Message,
			Value:      issue.Value,
			FixCommand: issue.FixCommand,
			FixHint:    issue.FixHint,
		})
	}
	for _, issue := range result.SchemaIssues {
		jsonResult.Issues = append(jsonResult.Issues, CheckIssueJSON{
			Type:       string(issue.Type),
			Level:      issue.Level.String(),
			FilePath:   "schema.yaml",
			Line:       0,
			Message:    issue.Message,
			Value:      issue.Value,
			FixCommand: issue.FixCommand,
			FixHint:    issue.FixHint,
		})
	}

	typeCountMap := make(map[string]int)
	typeValueCountMap := make(map[string]map[string]int)
	for _, issue := range result.Issues {
		typeKey := string(issue.Type)
		typeCountMap[typeKey]++
		if typeValueCountMap[typeKey] == nil {
			typeValueCountMap[typeKey] = make(map[string]int)
		}
		if issue.Value != "" {
			typeValueCountMap[typeKey][issue.Value]++
		}
	}

	for issueType, count := range typeCountMap {
		valueCounts := typeValueCountMap[issueType]
		type valueCount struct {
			value string
			count int
		}
		var sortedValues []valueCount
		for value, valueCountValue := range valueCounts {
			sortedValues = append(sortedValues, valueCount{value: value, count: valueCountValue})
		}
		sort.Slice(sortedValues, func(i, j int) bool {
			if sortedValues[i].count != sortedValues[j].count {
				return sortedValues[i].count > sortedValues[j].count
			}
			return sortedValues[i].value < sortedValues[j].value
		})

		topValues := make([]string, 0, 10)
		for i := 0; i < len(sortedValues) && i < 10; i++ {
			topValues = append(topValues, sortedValues[i].value)
		}

		fixCmd, fixHint := summaryFix(issueType, result.Issues)

		jsonResult.Summary = append(jsonResult.Summary, CheckSummaryJSON{
			IssueType:    issueType,
			Count:        count,
			UniqueValues: len(valueCounts),
			FixCommand:   fixCmd,
			FixHint:      fixHint,
			TopValues:    topValues,
		})
	}
	sort.Slice(jsonResult.Summary, func(i, j int) bool {
		if jsonResult.Summary[i].Count != jsonResult.Summary[j].Count {
			return jsonResult.Summary[i].Count > jsonResult.Summary[j].Count
		}
		return jsonResult.Summary[i].IssueType < jsonResult.Summary[j].IssueType
	})

	return jsonResult
}

func summaryFix(issueType string, issues []check.Issue) (string, string) {
	if issueType == string(check.IssueMissingReference) {
		return "rvn check create-missing --json", "Preview missing referenced pages, then run with --confirm after review"
	}

	for _, issue := range issues {
		if string(issue.Type) == issueType && issue.FixCommand != "" {
			return issue.FixCommand, issue.FixHint
		}
	}
	return "", ""
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

func detectBrokenFileLinkIssues(links []model.Link, vaultPath string, excludeMatcher *ravenignore.Matcher, scope *Scope, walkPath string, targetFileSet map[string]bool, docs []*parser.ParsedDocument) []check.Issue {
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
		if doc := docByPath(docs, link.FilePath); doc != nil {
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
	include = make(map[check.IssueType]bool)
	exclude = make(map[check.IssueType]bool)

	if opts.Issues != "" {
		for _, issueType := range strings.Split(opts.Issues, ",") {
			issueType = strings.TrimSpace(issueType)
			if issueType != "" {
				include[check.IssueType(issueType)] = true
			}
		}
	}
	if opts.Exclude != "" {
		for _, issueType := range strings.Split(opts.Exclude, ",") {
			issueType = strings.TrimSpace(issueType)
			if issueType != "" {
				exclude[check.IssueType(issueType)] = true
			}
		}
	}

	return include, exclude
}

func shouldIncludeIssue(issue check.Issue, include, exclude map[check.IssueType]bool, errorsOnly bool) bool {
	if errorsOnly && issue.Level == check.LevelWarning {
		return false
	}
	if len(include) > 0 && !include[issue.Type] {
		return false
	}
	if exclude[issue.Type] {
		return false
	}
	return true
}

func shouldIncludeSchemaIssue(issue check.SchemaIssue, include, exclude map[check.IssueType]bool, errorsOnly bool) bool {
	if errorsOnly && issue.Level == check.LevelWarning {
		return false
	}
	if len(include) > 0 && !include[issue.Type] {
		return false
	}
	if exclude[issue.Type] {
		return false
	}
	return true
}

func isFileInScope(filePath string, scope *Scope, walkPath string, targetFileSet map[string]bool) bool {
	switch scope.Type {
	case "file":
		return targetFileSet[filePath]
	case "directory":
		rel, err := filepath.Rel(walkPath, filePath)
		if err != nil || filepath.IsAbs(rel) {
			return false
		}
		return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	default:
		return true
	}
}

func docByPath(docs []*parser.ParsedDocument, filePath string) *parser.ParsedDocument {
	if filePath == "" {
		return nil
	}
	for _, doc := range docs {
		if doc != nil && doc.FilePath == filePath {
			return doc
		}
	}
	return nil
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
