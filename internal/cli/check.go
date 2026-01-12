package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	checkStrict        bool
	checkCreateMissing bool
	checkByFile        bool
	checkVerbose       bool
	checkType          string
	checkTrait         string
	checkIssues        string
	checkExclude       string
	checkErrorsOnly    bool
)

// CheckIssueJSON represents an issue in JSON output
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

// CheckSummaryJSON groups issues by type for easier agent processing
type CheckSummaryJSON struct {
	IssueType    string   `json:"issue_type"`
	Count        int      `json:"count"`
	UniqueValues int      `json:"unique_values,omitempty"` // Number of unique values (e.g., 5 different types)
	FixCommand   string   `json:"fix_command,omitempty"`
	FixHint      string   `json:"fix_hint,omitempty"`
	TopValues    []string `json:"top_values,omitempty"` // Top 10 unique values (most common first)
}

// CheckScopeJSON describes the scope of the check
type CheckScopeJSON struct {
	Type  string `json:"type"`            // "full", "file", "directory", "type_filter", "trait_filter"
	Value string `json:"value,omitempty"` // The path, type name, or trait name
}

// CheckResultJSON is the top-level JSON output
type CheckResultJSON struct {
	VaultPath  string             `json:"vault_path"`
	Scope      *CheckScopeJSON    `json:"scope,omitempty"`
	FileCount  int                `json:"file_count"`
	ErrorCount int                `json:"error_count"`
	WarnCount  int                `json:"warning_count"`
	Issues     []CheckIssueJSON   `json:"issues"`
	Summary    []CheckSummaryJSON `json:"summary"`
}

// checkScope describes what subset of the vault to check
type checkScope struct {
	scopeType   string   // "full", "file", "directory", "type_filter", "trait_filter"
	scopeValue  string   // The path, type name, or trait name
	targetFiles []string // For file/directory scopes, the absolute paths to check
}

// resolveCheckScope determines what to check based on args and flags
func resolveCheckScope(vaultPath string, args []string) (*checkScope, error) {
	scope := &checkScope{scopeType: "full"}

	// Type filter takes precedence
	if checkType != "" {
		scope.scopeType = "type_filter"
		scope.scopeValue = checkType
		return scope, nil
	}

	// Trait filter
	if checkTrait != "" {
		scope.scopeType = "trait_filter"
		scope.scopeValue = checkTrait
		return scope, nil
	}

	// No positional arg means full vault
	if len(args) == 0 {
		return scope, nil
	}

	pathArg := args[0]

	// Try as literal file/directory first
	fullPath := filepath.Join(vaultPath, pathArg)

	// Check if it's a directory
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		scope.scopeType = "directory"
		scope.scopeValue = pathArg
		return scope, nil
	}

	// Check if it's a file (with or without .md)
	filePath := fullPath
	if !strings.HasSuffix(filePath, ".md") {
		filePath = fullPath + ".md"
	}
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		scope.scopeType = "file"
		relPath, _ := filepath.Rel(vaultPath, filePath)
		scope.scopeValue = relPath
		scope.targetFiles = []string{filePath}
		return scope, nil
	}

	// Try resolving as a reference
	result, err := ResolveReference(pathArg, ResolveOptions{VaultPath: vaultPath})
	if err != nil {
		return nil, fmt.Errorf("could not resolve '%s': %w", pathArg, err)
	}

	scope.scopeType = "file"
	relPath, _ := filepath.Rel(vaultPath, result.FilePath)
	scope.scopeValue = relPath
	scope.targetFiles = []string{result.FilePath}
	return scope, nil
}

// parseIssueFilter parses the --issues and --exclude flags
func parseIssueFilter() (include map[check.IssueType]bool, exclude map[check.IssueType]bool) {
	include = make(map[check.IssueType]bool)
	exclude = make(map[check.IssueType]bool)

	if checkIssues != "" {
		for _, issueType := range strings.Split(checkIssues, ",") {
			issueType = strings.TrimSpace(issueType)
			if issueType != "" {
				include[check.IssueType(issueType)] = true
			}
		}
	}

	if checkExclude != "" {
		for _, issueType := range strings.Split(checkExclude, ",") {
			issueType = strings.TrimSpace(issueType)
			if issueType != "" {
				exclude[check.IssueType(issueType)] = true
			}
		}
	}

	return include, exclude
}

// shouldIncludeIssue checks if an issue should be included based on filters
func shouldIncludeIssue(issue check.Issue, include, exclude map[check.IssueType]bool) bool {
	// Check errors-only filter
	if checkErrorsOnly && issue.Level == check.LevelWarning {
		return false
	}

	// If include filter is set, issue must be in it
	if len(include) > 0 && !include[issue.Type] {
		return false
	}

	// If exclude filter is set, issue must not be in it
	if exclude[issue.Type] {
		return false
	}

	return true
}

// shouldIncludeSchemaIssue checks if a schema issue should be included based on filters
func shouldIncludeSchemaIssue(issue check.SchemaIssue, include, exclude map[check.IssueType]bool) bool {
	// Check errors-only filter
	if checkErrorsOnly && issue.Level == check.LevelWarning {
		return false
	}

	// If include filter is set, issue must be in it
	if len(include) > 0 && !include[issue.Type] {
		return false
	}

	// If exclude filter is set, issue must not be in it
	if exclude[issue.Type] {
		return false
	}

	return true
}

var checkCmd = &cobra.Command{
	Use:   "check [path]",
	Short: "Validate the vault",
	Long:  `Checks all files for errors and warnings (type mismatches, broken references, etc.)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Resolve the check scope
		scope, err := resolveCheckScope(vaultPath, args)
		if err != nil {
			return err
		}

		if !jsonOutput {
			switch scope.scopeType {
			case "full":
				fmt.Printf("Checking vault: %s\n", ui.Muted.Render(vaultPath))
			case "file":
				fmt.Printf("Checking file: %s\n", ui.FilePath(scope.scopeValue))
			case "directory":
				fmt.Printf("Checking directory: %s\n", ui.FilePath(scope.scopeValue+"/"))
			case "type_filter":
				fmt.Printf("Checking type: %s\n", ui.Accent.Render(scope.scopeValue))
			case "trait_filter":
				fmt.Printf("Checking trait: %s\n", ui.Accent.Render("@"+scope.scopeValue))
			}
		}

		// Parse issue filters
		includeIssues, excludeIssues := parseIssueFilter()

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		var errorCount, warningCount, fileCount int
		var allDocs []*parser.ParsedDocument
		var allObjectInfos []check.ObjectInfo
		var allIssues []check.Issue
		var parseErrors []check.Issue
		var schemaIssues []check.SchemaIssue

		// Check for stale index first and fetch aliases
		staleWarningShown := false
		var aliases map[string]string
		var duplicateAliases []check.DuplicateAlias
		db, err := index.Open(vaultPath)
		if err == nil {
			defer db.Close()
			stalenessInfo, err := db.CheckStaleness(vaultPath)
			if err == nil && stalenessInfo.IsStale {
				staleCount := len(stalenessInfo.StaleFiles)
				if !jsonOutput {
					fmt.Println(ui.Warningf("Index may be stale (%d file(s) modified since last reindex)", staleCount))
					if staleCount <= 5 {
						for _, f := range stalenessInfo.StaleFiles {
							fmt.Printf("       - %s\n", ui.FilePath(f))
						}
					} else {
						for i := 0; i < 3; i++ {
							fmt.Printf("       - %s\n", ui.FilePath(stalenessInfo.StaleFiles[i]))
						}
						fmt.Printf("       %s\n", ui.Muted.Render(fmt.Sprintf("... and %d more", staleCount-3)))
					}
					fmt.Printf("       %s\n\n", ui.Hint("Run 'rvn reindex' to update the index."))
				}
				// Add to issues for JSON output (only for full vault checks)
				if scope.scopeType == "full" {
					staleIssue := check.Issue{
						Level:      check.LevelWarning,
						Type:       check.IssueStaleIndex,
						FilePath:   "",
						Line:       0,
						Message:    fmt.Sprintf("Index may be stale (%d file(s) modified since last reindex)", staleCount),
						FixCommand: "rvn reindex",
						FixHint:    "Run 'rvn reindex' to update the index",
					}
					if shouldIncludeIssue(staleIssue, includeIssues, excludeIssues) {
						allIssues = append(allIssues, staleIssue)
						warningCount++
					}
				}
				staleWarningShown = true
			}

			// Fetch aliases for validation
			aliases, _ = db.AllAliases()

			// Fetch duplicate aliases
			dbDuplicates, _ := db.FindDuplicateAliases()
			for _, dup := range dbDuplicates {
				duplicateAliases = append(duplicateAliases, check.DuplicateAlias{
					Alias:     dup.Alias,
					ObjectIDs: dup.ObjectIDs,
				})
			}
		}

		// Determine which files to walk based on scope
		var walkPath string
		var targetFileSet map[string]bool

		switch scope.scopeType {
		case "file":
			// For single file, we still need all object IDs for reference validation
			// but we only validate the target file
			walkPath = vaultPath
			targetFileSet = make(map[string]bool)
			for _, f := range scope.targetFiles {
				targetFileSet[f] = true
			}
		case "directory":
			walkPath = filepath.Join(vaultPath, scope.scopeValue)
		default:
			walkPath = vaultPath
		}

		// First pass: parse all documents in the vault to collect object IDs
		// (needed for reference validation even when checking a subset)
		err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
			if result.Error != nil {
				// Only count files in scope
				if isFileInScope(result.Path, scope, walkPath, targetFileSet) {
					fileCount++
					parseErrors = append(parseErrors, check.Issue{
						Level:    check.LevelError,
						Type:     check.IssueParseError,
						FilePath: result.RelativePath,
						Line:     1,
						Message:  result.Error.Error(),
						FixHint:  "Fix the YAML frontmatter or markdown syntax",
					})
					errorCount++
				}
				return nil
			}

			// Collect all object infos for reference resolution
			for _, obj := range result.Document.Objects {
				allObjectInfos = append(allObjectInfos, check.ObjectInfo{
					ID:   obj.ID,
					Type: obj.ObjectType,
				})
			}

			// Only include documents that are in scope for validation
			if isFileInScope(result.Path, scope, walkPath, targetFileSet) {
				fileCount++
				allDocs = append(allDocs, result.Document)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking vault: %w", err)
		}

		// Second pass: validate with full context (including type information and aliases)
		validator := check.NewValidatorWithTypesAndAliases(s, allObjectInfos, aliases)
		validator.SetDuplicateAliases(duplicateAliases)

		for _, doc := range allDocs {
			issues := validator.ValidateDocument(doc)

			for _, issue := range issues {
				// Apply type/trait filter
				if !isIssueInScope(issue, doc, scope, s) {
					continue
				}

				// Apply issue type filter
				if !shouldIncludeIssue(issue, includeIssues, excludeIssues) {
					continue
				}

				allIssues = append(allIssues, issue)

				if issue.Level == check.LevelWarning {
					warningCount++
				} else {
					errorCount++
				}
			}
		}

		// Add filtered parse errors to all issues
		for _, pe := range parseErrors {
			if shouldIncludeIssue(pe, includeIssues, excludeIssues) {
				allIssues = append([]check.Issue{pe}, allIssues...)
			}
		}

		// Run schema integrity checks (only for full vault checks, or when checking types/traits)
		if scope.scopeType == "full" || scope.scopeType == "type_filter" || scope.scopeType == "trait_filter" {
			schemaIssues = validator.ValidateSchema()

			// Filter schema issues based on scope
			var filteredSchemaIssues []check.SchemaIssue
			for _, issue := range schemaIssues {
				// For type_filter, only show schema issues related to that type
				if scope.scopeType == "type_filter" {
					if !strings.Contains(issue.Value, scope.scopeValue) &&
						!strings.HasPrefix(issue.Value, scope.scopeValue+".") {
						continue
					}
				}

				// For trait_filter, only show schema issues related to that trait
				if scope.scopeType == "trait_filter" {
					if issue.Value != scope.scopeValue {
						continue
					}
				}

				if !shouldIncludeSchemaIssue(issue, includeIssues, excludeIssues) {
					continue
				}

				filteredSchemaIssues = append(filteredSchemaIssues, issue)

				if issue.Level == check.LevelWarning {
					warningCount++
				} else {
					errorCount++
				}
			}
			schemaIssues = filteredSchemaIssues
		}

		// JSON output mode
		if jsonOutput {
			result := buildCheckJSONWithScope(vaultPath, scope, fileCount, errorCount, warningCount, allIssues, schemaIssues)
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
		} else if checkByFile {
			// Group issues by file
			printIssuesByFile(allIssues, schemaIssues, staleWarningShown)
			fmt.Println()
			if errorCount == 0 && warningCount == 0 {
				fmt.Println(ui.Successf("No issues found in %d files.", fileCount))
			} else {
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
			}
		} else if checkVerbose {
			// Verbose mode: print all issues inline
			printIssuesVerbose(allIssues, schemaIssues)
			fmt.Println()
			if errorCount == 0 && warningCount == 0 {
				fmt.Println(ui.Successf("No issues found in %d files.", fileCount))
			} else {
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
			}
		} else {
			// Default: summary by issue type
			fmt.Println()
			if errorCount == 0 && warningCount == 0 {
				fmt.Println(ui.Successf("No issues found in %d files.", fileCount))
			} else {
				printIssueSummary(allIssues, schemaIssues)
				fmt.Println()
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
				fmt.Println(ui.Hint("Use --verbose to see all issues, or --by-file to group by file."))
			}

			// Handle --create-missing (interactive mode only, full vault check only)
			if checkCreateMissing && scope.scopeType == "full" {
				missingRefs := validator.MissingRefs()
				if len(missingRefs) > 0 {
					created := handleMissingRefs(vaultPath, s, missingRefs)
					if created > 0 {
						fmt.Printf("\n%s\n", ui.Successf("Created %d missing page(s).", created))
					}
				}

				undefinedTraits := validator.UndefinedTraits()
				if len(undefinedTraits) > 0 {
					added := handleUndefinedTraits(vaultPath, s, undefinedTraits)
					if added > 0 {
						fmt.Printf("\n%s\n", ui.Successf("Added %d trait(s) to schema.", added))
					}
				}
			}
		}

		if errorCount > 0 || (checkStrict && warningCount > 0) {
			os.Exit(1)
		}

		return nil
	},
}

// isFileInScope checks if a file path is within the check scope
func isFileInScope(filePath string, scope *checkScope, walkPath string, targetFileSet map[string]bool) bool {
	switch scope.scopeType {
	case "file":
		return targetFileSet[filePath]
	case "directory":
		return strings.HasPrefix(filePath, walkPath)
	default:
		return true
	}
}

// isIssueInScope checks if an issue matches the type/trait filter
func isIssueInScope(issue check.Issue, doc *parser.ParsedDocument, scope *checkScope, s *schema.Schema) bool {
	switch scope.scopeType {
	case "type_filter":
		// Check if any object in the document matches the type
		for _, obj := range doc.Objects {
			if obj.ObjectType == scope.scopeValue {
				return true
			}
		}
		return false

	case "trait_filter":
		// Check if the issue is related to the trait
		// Issues about traits include the trait name in the Value field
		if issue.Type == check.IssueUndefinedTrait ||
			issue.Type == check.IssueInvalidTraitValue ||
			issue.Type == check.IssueMissingRequiredTrait {
			return issue.Value == scope.scopeValue || strings.HasPrefix(issue.Value, scope.scopeValue)
		}
		// For other issues, include them if the document uses the trait
		for _, trait := range doc.Traits {
			if trait.TraitType == scope.scopeValue {
				return true
			}
		}
		return false

	default:
		return true
	}
}

// printIssuesByFile groups and prints issues by file path
func printIssuesByFile(issues []check.Issue, schemaIssues []check.SchemaIssue, staleWarningShown bool) {
	// Group issues by file
	issuesByFile := make(map[string][]check.Issue)
	var globalIssues []check.Issue

	for _, issue := range issues {
		if issue.FilePath == "" {
			globalIssues = append(globalIssues, issue)
		} else {
			issuesByFile[issue.FilePath] = append(issuesByFile[issue.FilePath], issue)
		}
	}

	// Print global issues first (like stale index)
	if len(globalIssues) > 0 && !staleWarningShown {
		for _, issue := range globalIssues {
			if issue.Level == check.LevelWarning {
				fmt.Println(ui.Warning(issue.Message))
			} else {
				fmt.Println(ui.Error(issue.Message))
			}
		}
		fmt.Println()
	}

	// Print schema issues
	if len(schemaIssues) > 0 {
		fmt.Println(ui.FilePath("schema.yaml") + ":")
		for _, issue := range schemaIssues {
			symbol := ui.SymbolError
			if issue.Level == check.LevelWarning {
				symbol = ui.SymbolWarning
			}
			fmt.Printf("  %s %s\n", symbol, issue.Message)
		}
		fmt.Println()
	}

	// Get sorted file paths
	var filePaths []string
	for fp := range issuesByFile {
		filePaths = append(filePaths, fp)
	}
	sort.Strings(filePaths)

	// Print issues for each file
	for _, filePath := range filePaths {
		fileIssues := issuesByFile[filePath]

		// Count errors and warnings for this file
		var errCount, warnCount int
		for _, issue := range fileIssues {
			if issue.Level == check.LevelWarning {
				warnCount++
			} else {
				errCount++
			}
		}

		// Print file header with styled path and count badge
		countBadge := ui.Muted.Render(ui.ErrorWarningCounts(errCount, warnCount))
		fmt.Printf("%s %s:\n", ui.FilePath(filePath), countBadge)

		// Sort issues by line number
		sort.Slice(fileIssues, func(i, j int) bool {
			return fileIssues[i].Line < fileIssues[j].Line
		})

		// Print each issue
		for _, issue := range fileIssues {
			symbol := ui.SymbolError
			if issue.Level == check.LevelWarning {
				symbol = ui.SymbolWarning
			}
			lineNum := ui.Muted.Render(fmt.Sprintf("L%d", issue.Line))
			fmt.Printf("  %s %s %s\n", symbol, lineNum, issue.Message)
		}
		fmt.Println()
	}
}

// printIssueSummary prints a compact summary grouped by issue type
func printIssueSummary(issues []check.Issue, schemaIssues []check.SchemaIssue) {
	// Group by issue type
	type issueGroup struct {
		issueType check.IssueType
		level     check.IssueLevel
		count     int
		topValues []string // Up to 3 examples
	}

	groups := make(map[check.IssueType]*issueGroup)
	valuesByType := make(map[check.IssueType]map[string]int)

	// Process file issues
	for _, issue := range issues {
		if groups[issue.Type] == nil {
			groups[issue.Type] = &issueGroup{
				issueType: issue.Type,
				level:     issue.Level,
			}
			valuesByType[issue.Type] = make(map[string]int)
		}
		groups[issue.Type].count++
		if issue.Value != "" {
			valuesByType[issue.Type][issue.Value]++
		} else if issue.FilePath != "" {
			// Use file path as value for display
			valuesByType[issue.Type][issue.FilePath]++
		}
	}

	// Process schema issues
	for _, issue := range schemaIssues {
		if groups[issue.Type] == nil {
			groups[issue.Type] = &issueGroup{
				issueType: issue.Type,
				level:     issue.Level,
			}
			valuesByType[issue.Type] = make(map[string]int)
		}
		groups[issue.Type].count++
		if issue.Value != "" {
			valuesByType[issue.Type][issue.Value]++
		}
	}

	// Compute top values for each group
	for issueType, valueCounts := range valuesByType {
		type vc struct {
			value string
			count int
		}
		var sorted []vc
		for v, c := range valueCounts {
			sorted = append(sorted, vc{v, c})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].count > sorted[j].count
		})
		// Take top 3
		for i := 0; i < len(sorted) && i < 3; i++ {
			groups[issueType].topValues = append(groups[issueType].topValues, sorted[i].value)
		}
	}

	// Sort groups: errors first, then by count descending
	var sortedGroups []*issueGroup
	for _, g := range groups {
		sortedGroups = append(sortedGroups, g)
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		// Errors before warnings
		if sortedGroups[i].level != sortedGroups[j].level {
			return sortedGroups[i].level == check.LevelError
		}
		// Then by count descending
		return sortedGroups[i].count > sortedGroups[j].count
	})

	// Print each group
	for _, g := range sortedGroups {
		symbol := ui.SymbolError
		if g.level == check.LevelWarning {
			symbol = ui.SymbolWarning
		}

		// Format: ✗ 4 unknown_frontmatter_key  (due, priority)
		issueLabel := ui.Accent.Render(string(g.issueType))
		countStr := fmt.Sprintf("%d", g.count)

		if len(g.topValues) > 0 {
			examples := ui.Muted.Render("(" + strings.Join(g.topValues, ", ") + ")")
			fmt.Printf("%s %s %s  %s\n", symbol, countStr, issueLabel, examples)
		} else {
			fmt.Printf("%s %s %s\n", symbol, countStr, issueLabel)
		}
	}
}

// printIssuesVerbose prints all issues inline (verbose mode)
func printIssuesVerbose(issues []check.Issue, schemaIssues []check.SchemaIssue) {
	// Print schema issues first
	for _, issue := range schemaIssues {
		schemaLabel := ui.Muted.Render("[schema]")
		if issue.Level == check.LevelWarning {
			fmt.Printf("%s %s %s\n", ui.SymbolWarning, schemaLabel, issue.Message)
		} else {
			fmt.Printf("%s %s %s\n", ui.SymbolError, schemaLabel, issue.Message)
		}
	}

	// Print file issues
	for _, issue := range issues {
		if issue.FilePath == "" {
			// Global issue (like stale index)
			if issue.Level == check.LevelWarning {
				fmt.Println(ui.Warning(issue.Message))
			} else {
				fmt.Println(ui.Error(issue.Message))
			}
		} else {
			location := fmt.Sprintf("%s:%s", ui.FilePath(issue.FilePath), ui.LineNum(issue.Line))
			if issue.Level == check.LevelWarning {
				fmt.Printf("%s %s %s\n", ui.SymbolWarning, location, issue.Message)
			} else {
				fmt.Printf("%s %s %s\n", ui.SymbolError, location, issue.Message)
			}
		}
	}
}

// buildCheckJSONWithScope creates the structured JSON output for check command with scope info
func buildCheckJSONWithScope(vaultPath string, scope *checkScope, fileCount, errorCount, warnCount int, issues []check.Issue, schemaIssues []check.SchemaIssue) CheckResultJSON {
	result := CheckResultJSON{
		VaultPath:  vaultPath,
		FileCount:  fileCount,
		ErrorCount: errorCount,
		WarnCount:  warnCount,
		Issues:     make([]CheckIssueJSON, 0, len(issues)+len(schemaIssues)),
	}

	// Add scope information
	if scope != nil && scope.scopeType != "full" {
		result.Scope = &CheckScopeJSON{
			Type:  scope.scopeType,
			Value: scope.scopeValue,
		}
	}

	return buildCheckJSONInternal(result, issues, schemaIssues)
}

// buildCheckJSON creates the structured JSON output for check command (legacy, for compatibility)
func buildCheckJSON(vaultPath string, fileCount, errorCount, warnCount int, issues []check.Issue, schemaIssues []check.SchemaIssue) CheckResultJSON {
	result := CheckResultJSON{
		VaultPath:  vaultPath,
		FileCount:  fileCount,
		ErrorCount: errorCount,
		WarnCount:  warnCount,
		Issues:     make([]CheckIssueJSON, 0, len(issues)+len(schemaIssues)),
	}
	return buildCheckJSONInternal(result, issues, schemaIssues)
}

// buildCheckJSONInternal is the shared implementation for building check JSON output
func buildCheckJSONInternal(result CheckResultJSON, issues []check.Issue, schemaIssues []check.SchemaIssue) CheckResultJSON {

	// Convert issues to JSON format
	for _, issue := range issues {
		result.Issues = append(result.Issues, CheckIssueJSON{
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

	// Convert schema issues to JSON format
	for _, issue := range schemaIssues {
		result.Issues = append(result.Issues, CheckIssueJSON{
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

	// Build summary - group by issue type, count unique values
	typeCountMap := make(map[string]int)
	typeValueCountMap := make(map[string]map[string]int) // issueType -> value -> count

	for _, issue := range issues {
		typeCountMap[string(issue.Type)]++
		if typeValueCountMap[string(issue.Type)] == nil {
			typeValueCountMap[string(issue.Type)] = make(map[string]int)
		}
		if issue.Value != "" {
			typeValueCountMap[string(issue.Type)][issue.Value]++
		}
	}

	// Convert to slice sorted by count
	for issueType, count := range typeCountMap {
		valueCounts := typeValueCountMap[issueType]

		// Sort values by frequency (most common first)
		type valueCount struct {
			value string
			count int
		}
		var sortedValues []valueCount
		for v, c := range valueCounts {
			sortedValues = append(sortedValues, valueCount{v, c})
		}
		sort.Slice(sortedValues, func(i, j int) bool {
			return sortedValues[i].count > sortedValues[j].count
		})

		// Take top 10 values
		topValues := make([]string, 0, 10)
		for i := 0; i < len(sortedValues) && i < 10; i++ {
			topValues = append(topValues, sortedValues[i].value)
		}

		// Find a representative fix command
		fixCmd := ""
		fixHint := ""
		for _, issue := range issues {
			if string(issue.Type) == issueType && issue.FixCommand != "" {
				fixCmd = issue.FixCommand
				fixHint = issue.FixHint
				break
			}
		}

		result.Summary = append(result.Summary, CheckSummaryJSON{
			IssueType:    issueType,
			Count:        count,
			UniqueValues: len(valueCounts),
			FixCommand:   fixCmd,
			FixHint:      fixHint,
			TopValues:    topValues,
		})
	}

	// Sort summary by count descending
	sort.Slice(result.Summary, func(i, j int) bool {
		return result.Summary[i].Count > result.Summary[j].Count
	})

	return result
}

func handleMissingRefs(vaultPath string, s *schema.Schema, refs []*check.MissingRef) int {
	// Categorize refs by confidence
	var certain, inferred, unknown []*check.MissingRef
	for _, ref := range refs {
		switch ref.Confidence {
		case check.ConfidenceCertain:
			certain = append(certain, ref)
		case check.ConfidenceInferred:
			inferred = append(inferred, ref)
		default:
			unknown = append(unknown, ref)
		}
	}

	// Sort each category by path for consistent output
	sortRefs := func(refs []*check.MissingRef) {
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].TargetPath < refs[j].TargetPath
		})
	}
	sortRefs(certain)
	sortRefs(inferred)
	sortRefs(unknown)

	fmt.Printf("\n%s\n", ui.Header("--- Missing References ---"))
	reader := bufio.NewReader(os.Stdin)
	created := 0

	// Handle certain refs (from typed fields)
	if len(certain) > 0 {
		fmt.Printf("\n%s\n", ui.Bold.Render("Certain (from typed fields):"))
		for _, ref := range certain {
			source := ref.SourceObjectID
			if source == "" {
				source = ref.SourceFile
			}
			resolvedPath := pages.SlugifyPath(pages.ResolveTargetPath(ref.TargetPath, ref.InferredType, s))
			fmt.Printf("  • %s → %s %s\n",
				ui.Accent.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(from %s.%s)", source, ref.FieldSource)))
		}

		fmt.Printf("\nCreate these pages? %s ", ui.Muted.Render("[Y/n]"))
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "" || response == "y" || response == "yes" {
			for _, ref := range certain {
				resolvedPath := pages.SlugifyPath(pages.ResolveTargetPath(ref.TargetPath, ref.InferredType, s))
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType); err != nil {
					fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
				} else {
					fmt.Printf("  %s\n", ui.Successf("Created %s.md (type: %s)", resolvedPath, ref.InferredType))
					created++
				}
			}
		}
	}

	// Handle inferred refs (from path matching)
	if len(inferred) > 0 {
		fmt.Printf("\n%s\n", ui.Bold.Render("Inferred (from path matching default_path):"))
		for _, ref := range inferred {
			resolvedPath := pages.SlugifyPath(pages.ResolveTargetPath(ref.TargetPath, ref.InferredType, s))
			fmt.Printf("  ? %s → %s %s\n",
				ui.Accent.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(type: %s)", ref.InferredType)))
		}

		for _, ref := range inferred {
			resolvedPath := pages.SlugifyPath(pages.ResolveTargetPath(ref.TargetPath, ref.InferredType, s))
			fmt.Printf("\nCreate %s as '%s'? %s ", ui.FilePath(resolvedPath+".md"), ui.Accent.Render(ref.InferredType), ui.Muted.Render("[y/N]"))
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "y" || response == "yes" {
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType); err != nil {
					fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
				} else {
					fmt.Printf("  %s\n", ui.Successf("Created %s.md (type: %s)", resolvedPath, ref.InferredType))
					created++
				}
			}
		}
	}

	// Handle unknown refs
	if len(unknown) > 0 {
		fmt.Printf("\n%s\n", ui.Bold.Render("Unknown type (please specify):"))
		for _, ref := range unknown {
			fmt.Printf("  ? %s %s\n",
				ui.Accent.Render(ref.TargetPath),
				ui.Muted.Render(fmt.Sprintf("(referenced in %s:%d)", ref.SourceFile, ref.Line)))
		}

		// List available types
		var typeNames []string
		for name := range s.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		fmt.Printf("\nAvailable types: %s\n", ui.Accent.Render(strings.Join(typeNames, ", ")))

		for _, ref := range unknown {
			fmt.Printf("\nType for %s %s: ", ui.Accent.Render(ref.TargetPath), ui.Muted.Render("(or 'skip')"))
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(response)

			if response == "" || response == "skip" || response == "s" {
				fmt.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
				continue
			}

			// Validate type exists, offer to create if not
			if _, exists := s.Types[response]; !exists {
				created += handleNewTypeCreation(vaultPath, s, ref, response, reader)
				continue
			}

			resolvedPath := pages.SlugifyPath(pages.ResolveTargetPath(ref.TargetPath, response, s))
			if err := createMissingPage(vaultPath, s, ref.TargetPath, response); err != nil {
				fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
			} else {
				fmt.Printf("  %s\n", ui.Successf("Created %s.md (type: %s)", resolvedPath, response))
				created++
			}
		}
	}

	return created
}

// handleUndefinedTraits prompts the user to add undefined traits to the schema.
// Returns the number of traits added.
func handleUndefinedTraits(vaultPath string, s *schema.Schema, traits []*check.UndefinedTrait) int {
	if len(traits) == 0 {
		return 0
	}

	// Sort by usage count (most used first)
	sort.Slice(traits, func(i, j int) bool {
		return traits[i].UsageCount > traits[j].UsageCount
	})

	fmt.Printf("\n%s\n", ui.Header("--- Undefined Traits ---"))
	fmt.Println("\nThe following traits are used but not defined in schema.yaml:")
	for _, trait := range traits {
		valueInfo := "no value"
		if trait.HasValue {
			valueInfo = "with value"
		}
		fmt.Printf("  • %s %s\n",
			ui.Accent.Render("@"+trait.TraitName),
			ui.Muted.Render(fmt.Sprintf("(%d usages, %s)", trait.UsageCount, valueInfo)))
		for _, loc := range trait.Locations {
			fmt.Printf("      %s\n", ui.Muted.Render(loc))
		}
	}

	reader := bufio.NewReader(os.Stdin)
	added := 0

	fmt.Println("\nWould you like to add these traits to the schema?")

	for _, trait := range traits {
		fmt.Printf("\nAdd %s to schema? %s ", ui.Accent.Render("@"+trait.TraitName), ui.Muted.Render("[y/N]"))
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Printf("  %s\n", ui.Muted.Render("Skipped @"+trait.TraitName))
			continue
		}

		// Determine trait type
		traitType := promptTraitType(trait, reader)
		if traitType == "" {
			fmt.Printf("  %s\n", ui.Muted.Render("Skipped @"+trait.TraitName))
			continue
		}

		// Get additional options based on type
		var enumValues []string
		var defaultValue string

		if traitType == "enum" {
			fmt.Printf("  Enum values %s: ", ui.Muted.Render("(comma-separated, e.g., 'low,medium,high')"))
			valuesStr, _ := reader.ReadString('\n')
			valuesStr = strings.TrimSpace(valuesStr)
			if valuesStr != "" {
				enumValues = strings.Split(valuesStr, ",")
				for i := range enumValues {
					enumValues[i] = strings.TrimSpace(enumValues[i])
				}
			}
		}

		if traitType == "boolean" || traitType == "enum" {
			fmt.Printf("  Default value %s: ", ui.Muted.Render("(or leave empty)"))
			defaultValue, _ = reader.ReadString('\n')
			defaultValue = strings.TrimSpace(defaultValue)
		}

		// Create the trait
		if err := createNewTrait(vaultPath, s, trait.TraitName, traitType, enumValues, defaultValue); err != nil {
			fmt.Printf("  %s\n", ui.Errorf("Failed to add @%s: %v", trait.TraitName, err))
			continue
		}

		fmt.Printf("  %s\n", ui.Successf("Added trait '@%s' (type: %s) to schema.yaml", trait.TraitName, traitType))
		added++
	}

	return added
}

// promptTraitType asks the user what type a trait should be.
func promptTraitType(trait *check.UndefinedTrait, reader *bufio.Reader) string {
	// Suggest a type based on usage
	suggested := "boolean"
	if trait.HasValue {
		suggested = "string"
	}

	fmt.Printf("  Type for %s? %s %s: ",
		ui.Accent.Render("@"+trait.TraitName),
		ui.Muted.Render("[boolean/string/date/enum]"),
		ui.Muted.Render(fmt.Sprintf("(default: %s)", suggested)))
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" {
		return suggested
	}

	validTypes := map[string]bool{
		"boolean": true, "bool": true,
		"string":   true,
		"date":     true,
		"datetime": true,
		"enum":     true,
		"ref":      true,
	}

	// Normalize bool -> boolean
	if response == "bool" {
		response = "boolean"
	}

	if !validTypes[response] {
		fmt.Printf("  %s\n", ui.Errorf("Invalid type '%s'", response))
		return ""
	}

	return response
}

// createNewTrait adds a new trait to schema.yaml.
func createNewTrait(vaultPath string, s *schema.Schema, traitName, traitType string, enumValues []string, defaultValue string) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Parse as YAML to modify
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Ensure traits map exists
	traits, ok := schemaDoc["traits"].(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
		schemaDoc["traits"] = traits
	}

	// Build new trait definition
	newTrait := make(map[string]interface{})
	newTrait["type"] = traitType

	if len(enumValues) > 0 {
		newTrait["values"] = enumValues
	}

	if defaultValue != "" {
		// Convert "true"/"false" to boolean for boolean traits
		if traitType == "boolean" {
			if defaultValue == "true" {
				newTrait["default"] = true
			} else if defaultValue == "false" {
				newTrait["default"] = false
			}
		} else {
			newTrait["default"] = defaultValue
		}
	}

	traits[traitName] = newTrait

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	// Update the in-memory schema
	s.Traits[traitName] = &schema.TraitDefinition{
		Type:   schema.FieldType(traitType),
		Values: enumValues,
	}
	if defaultValue != "" {
		if traitType == "boolean" {
			s.Traits[traitName].Default = defaultValue == "true"
		} else {
			s.Traits[traitName].Default = defaultValue
		}
	}

	return nil
}

// handleNewTypeCreation prompts the user to create a new type when they enter a type that doesn't exist.
// Returns the number of pages created (0 or 1).
func handleNewTypeCreation(vaultPath string, s *schema.Schema, ref *check.MissingRef, typeName string, reader *bufio.Reader) int {
	fmt.Printf("\n  Type %s doesn't exist. Would you like to create it? %s ",
		ui.Accent.Render("'"+typeName+"'"),
		ui.Muted.Render("[y/N]"))
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
		return 0
	}

	// Prompt for default_path (optional)
	fmt.Printf("  Default path for '%s' files %s: ", typeName, ui.Muted.Render(fmt.Sprintf("(e.g., '%s/', or leave empty)", typeName+"s")))
	defaultPath, _ := reader.ReadString('\n')
	defaultPath = strings.TrimSpace(defaultPath)

	// Create the type
	if err := createNewType(vaultPath, s, typeName, defaultPath); err != nil {
		fmt.Printf("  %s\n", ui.Errorf("Failed to create type '%s': %v", typeName, err))
		return 0
	}
	fmt.Printf("  %s\n", ui.Successf("Created type '%s' in schema.yaml", typeName))
	if defaultPath != "" {
		fmt.Printf("    %s\n", ui.Muted.Render("default_path: "+defaultPath))
	}

	// Now create the page with the new type (resolving path with new default_path)
	resolvedPath := pages.SlugifyPath(pages.ResolveTargetPath(ref.TargetPath, typeName, s))
	if err := createMissingPage(vaultPath, s, ref.TargetPath, typeName); err != nil {
		fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
		return 0
	}
	fmt.Printf("  %s\n", ui.Successf("Created %s.md (type: %s)", resolvedPath, typeName))
	return 1
}

// createNewType adds a new type to schema.yaml.
func createNewType(vaultPath string, s *schema.Schema, typeName, defaultPath string) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check built-in types
	if typeName == "page" || typeName == "section" || typeName == "date" {
		return fmt.Errorf("'%s' is a built-in type", typeName)
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Parse as YAML to modify
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Ensure types map exists
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		types = make(map[string]interface{})
		schemaDoc["types"] = types
	}

	// Build new type definition
	newType := make(map[string]interface{})
	if defaultPath != "" {
		newType["default_path"] = defaultPath
	}

	types[typeName] = newType

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	// Update the in-memory schema so subsequent page creation works
	s.Types[typeName] = &schema.TypeDefinition{
		DefaultPath: defaultPath,
	}

	return nil
}

// createMissingPage creates a new page file using the pages package.
// pages.Create handles default_path resolution automatically via the schema.
func createMissingPage(vaultPath string, s *schema.Schema, targetPath, typeName string) error {
	_, err := pages.Create(pages.CreateOptions{
		VaultPath:                   vaultPath,
		TypeName:                    typeName,
		TargetPath:                  targetPath,
		Schema:                      s,
		IncludeRequiredPlaceholders: true,
	})
	return err
}

func init() {
	checkCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	checkCmd.Flags().BoolVar(&checkCreateMissing, "create-missing", false, "Interactively create missing referenced pages")
	checkCmd.Flags().BoolVar(&checkByFile, "by-file", false, "Group issues by file path")
	checkCmd.Flags().BoolVarP(&checkVerbose, "verbose", "V", false, "Show all issues with full details")
	checkCmd.Flags().StringVarP(&checkType, "type", "t", "", "Check only objects of this type")
	checkCmd.Flags().StringVar(&checkTrait, "trait", "", "Check only usages of this trait")
	checkCmd.Flags().StringVar(&checkIssues, "issues", "", "Only check these issue types (comma-separated)")
	checkCmd.Flags().StringVar(&checkExclude, "exclude", "", "Exclude these issue types (comma-separated)")
	checkCmd.Flags().BoolVar(&checkErrorsOnly, "errors-only", false, "Only report errors, skip warnings")
	rootCmd.AddCommand(checkCmd)
}
