package checksvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
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

func Run(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, opts Options) (*RunResult, error) {
	scope, err := resolveScope(vaultPath, vaultCfg, sch, opts)
	if err != nil {
		return nil, err
	}

	includeIssues, excludeIssues := parseIssueFilter(opts)

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

	// Check staleness + pull aliases from index when available.
	var aliases map[string]string
	var duplicateAliases []index.DuplicateAlias
	var canonicalResolver *resolver.Resolver
	db, err := index.Open(vaultPath)
	if err == nil {
		defer db.Close()
		stalenessInfo, stalenessErr := db.CheckStaleness(vaultPath)
		if stalenessErr == nil && stalenessInfo.IsStale {
			staleCount := len(stalenessInfo.StaleFiles)
			if scope.Type == "full" {
				staleIssue := check.Issue{
					Level:      check.LevelWarning,
					Type:       check.IssueStaleIndex,
					FilePath:   "",
					Line:       0,
					Message:    fmt.Sprintf("Index may be stale (%d file(s) modified since last reindex)", staleCount),
					FixCommand: "rvn reindex",
					FixHint:    "Run 'rvn reindex' to update the index",
				}
				if shouldIncludeIssue(staleIssue, includeIssues, excludeIssues, opts.ErrorsOnly) {
					allIssues = append(allIssues, staleIssue)
					result.WarningCount++
				}
			}
			result.StaleWarningShown = true
		}

		aliases, _ = db.AllAliases()
		duplicateAliases, _ = db.FindDuplicateAliases()
		canonicalResolver, _ = db.Resolver(index.ResolverOptions{
			DailyDirectory: vaultCfg.GetDailyDirectory(),
			Schema:         sch,
		})
	}

	walkPath := vaultPath
	targetFileSet := map[string]bool{}
	switch scope.Type {
	case "file":
		for _, f := range scope.targetFiles {
			targetFileSet[f] = true
		}
	case "directory":
		walkPath = filepath.Join(vaultPath, scope.Value)
	}

	walkOpts := &vault.WalkOptions{
		ParseOptions: &parser.ParseOptions{
			ObjectsRoot: vaultCfg.GetObjectsRoot(),
			PagesRoot:   vaultCfg.GetPagesRoot(),
		},
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
			allObjectInfos = append(allObjectInfos, check.ObjectInfo{ID: obj.ID, Type: obj.ObjectType})
		}

		if isFileInScope(walkResult.Path, scope, walkPath, targetFileSet) {
			result.FileCount++
			allDocs = append(allDocs, walkResult.Document)
		}

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("error walking vault: %w", walkErr)
	}

	validator := check.NewValidatorWithTypesAliasesAndResolver(sch, allObjectInfos, aliases, canonicalResolver)
	validator.SetDuplicateAliases(duplicateAliases)
	if vaultCfg.HasDirectoriesConfig() {
		validator.SetDirectoryRoots(vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot())
	}
	if canonicalResolver == nil {
		validator.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	}

	for _, doc := range allDocs {
		issues := validator.ValidateDocument(doc)
		for _, issue := range issues {
			if !isIssueInScope(issue, doc, scope) {
				continue
			}
			if !shouldIncludeIssue(issue, includeIssues, excludeIssues, opts.ErrorsOnly) {
				continue
			}

			allIssues = append(allIssues, issue)
			if issue.Level == check.LevelWarning {
				result.WarningCount++
			} else {
				result.ErrorCount++
			}
		}
	}

	for _, pe := range parseErrors {
		if shouldIncludeIssue(pe, includeIssues, excludeIssues, opts.ErrorsOnly) {
			allIssues = append([]check.Issue{pe}, allIssues...)
			result.ErrorCount++
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
			if !shouldIncludeSchemaIssue(issue, includeIssues, excludeIssues, opts.ErrorsOnly) {
				continue
			}

			schemaIssues = append(schemaIssues, issue)
			if issue.Level == check.LevelWarning {
				result.WarningCount++
			} else {
				result.ErrorCount++
			}
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

		fixCmd := ""
		fixHint := ""
		for _, issue := range result.Issues {
			if string(issue.Type) == issueType && issue.FixCommand != "" {
				fixCmd = issue.FixCommand
				fixHint = issue.FixHint
				break
			}
		}

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

func CreateMissingRefsNonInteractive(
	vaultPath string,
	sch *schema.Schema,
	refs []*check.MissingRef,
	objectsRoot string,
	pagesRoot string,
	templateDir string,
	protectedPrefixes []string,
) int {
	created := 0
	seen := make(map[string]struct{})

	for _, ref := range refs {
		typeName := ref.InferredType
		if typeName == "" {
			continue
		}
		if _, exists := sch.Types[typeName]; !exists && !schema.IsBuiltinType(typeName) {
			continue
		}

		resolvedPath := pages.ResolveTargetPathWithRoots(ref.TargetPath, typeName, sch, objectsRoot, pagesRoot)
		slugPath := pages.SlugifyPath(resolvedPath)
		if _, alreadyHandled := seen[slugPath]; alreadyHandled {
			continue
		}
		seen[slugPath] = struct{}{}

		if pages.Exists(vaultPath, resolvedPath) {
			continue
		}

		_, err := pages.Create(pages.CreateOptions{
			VaultPath:                   vaultPath,
			TypeName:                    typeName,
			TargetPath:                  ref.TargetPath,
			Schema:                      sch,
			IncludeRequiredPlaceholders: true,
			TemplateDir:                 templateDir,
			ProtectedPrefixes:           protectedPrefixes,
			ObjectsRoot:                 objectsRoot,
			PagesRoot:                   pagesRoot,
		})
		if err == nil {
			created++
		}
	}

	return created
}

func resolveScope(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, opts Options) (*Scope, error) {
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

	rt := &readsvc.Runtime{
		VaultPath: vaultPath,
		VaultCfg:  vaultCfg,
		Schema:    sch,
	}
	resolved, err := readsvc.ResolveReference(pathArg, rt, false)
	if err != nil {
		return nil, fmt.Errorf("could not resolve '%s': %w", pathArg, err)
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
		return strings.HasPrefix(filePath, walkPath)
	default:
		return true
	}
}

func isIssueInScope(issue check.Issue, doc *parser.ParsedDocument, scope *Scope) bool {
	switch scope.Type {
	case "type_filter":
		for _, obj := range doc.Objects {
			if obj.ObjectType == scope.Value {
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
