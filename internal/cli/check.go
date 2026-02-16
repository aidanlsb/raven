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

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
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
	checkFix           bool
	checkConfirm       bool
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
func resolveCheckScope(vaultPath string, args []string, vaultCfg *config.VaultConfig) (*checkScope, error) {
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
	result, err := ResolveReference(pathArg, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg})
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

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		// Resolve the check scope
		scope, err := resolveCheckScope(vaultPath, args, vaultCfg)
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
				fmt.Printf("Checking type: %s\n", ui.Bold.Render(scope.scopeValue))
			case "trait_filter":
				fmt.Printf("Checking trait: %s\n", ui.Bold.Render("@"+scope.scopeValue))
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
		var duplicateAliases []index.DuplicateAlias
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
			duplicateAliases, _ = db.FindDuplicateAliases()
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
		walkOpts := &vault.WalkOptions{
			ParseOptions: &parser.ParseOptions{
				ObjectsRoot: vaultCfg.GetObjectsRoot(),
				PagesRoot:   vaultCfg.GetPagesRoot(),
			},
		}
		err = vault.WalkMarkdownFilesWithOptions(vaultPath, walkOpts, func(result vault.WalkResult) error {
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

		// Set directory roots for cleaner path suggestions (e.g., [[people/freya]] instead of [[objects/people/freya]])
		if vaultCfg.HasDirectoriesConfig() {
			validator.SetDirectoryRoots(vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot())
		}
		validator.SetDailyDirectory(vaultCfg.DailyDirectory)

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
		if jsonOutput && checkCreateMissing && scope.scopeType == "full" && checkConfirm {
			missingRefs := validator.MissingRefs()
			if len(missingRefs) > 0 {
				createMissingRefsNonInteractive(vaultPath, s, missingRefs, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot())
			}
		}

		if jsonOutput {
			result := buildCheckJSONWithScope(vaultPath, scope, fileCount, errorCount, warningCount, allIssues, schemaIssues)
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
		} else if checkByFile {
			// Group issues by file
			printIssuesByFile(allIssues, schemaIssues, staleWarningShown)
			fmt.Println()
			if errorCount == 0 && warningCount == 0 {
				fmt.Println(ui.Starf("No issues found in %d files.", fileCount))
			} else {
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
			}
		} else if checkVerbose {
			// Verbose mode: print all issues inline
			printIssuesVerbose(allIssues, schemaIssues)
			fmt.Println()
			if errorCount == 0 && warningCount == 0 {
				fmt.Println(ui.Starf("No issues found in %d files.", fileCount))
			} else {
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
			}
		} else {
			// Default: summary by issue type
			fmt.Println()
			if errorCount == 0 && warningCount == 0 {
				fmt.Println(ui.Starf("No issues found in %d files.", fileCount))
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
					created := handleMissingRefs(vaultPath, s, missingRefs, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot())
					if created > 0 {
						fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
					}
				}

				undefinedTraits := validator.UndefinedTraits()
				if len(undefinedTraits) > 0 {
					added := handleUndefinedTraits(vaultPath, s, undefinedTraits)
					if added > 0 {
						fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
					}
				}
			}

			// Handle --fix: auto-fix simple issues
			if checkFix {
				shortRefMap := validator.ShortRefs()
				fixableIssues := collectFixableIssues(allIssues, shortRefMap, s)

				if len(fixableIssues) == 0 {
					fmt.Println(ui.Hint("\nNo auto-fixable issues found."))
				} else {
					fixed, err := handleAutoFix(vaultPath, fixableIssues, checkConfirm)
					if err != nil {
						fmt.Printf("\n%s\n", ui.Errorf("Fix failed: %v", err))
					} else if checkConfirm {
						fmt.Printf("\n%s\n", ui.Checkf("Fixed %d issue(s) in %d file(s).", fixed.issueCount, fixed.fileCount))
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

// issueGroup represents a group of issues of the same type
type issueGroup struct {
	issueType check.IssueType
	level     check.IssueLevel
	count     int
	topValues []string // Up to 3 examples
}

// printIssueSummary prints a compact summary grouped by issue type
func printIssueSummary(issues []check.Issue, schemaIssues []check.SchemaIssue) {
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

	// Separate errors and warnings
	var errors, warnings []*issueGroup
	for _, g := range sortedGroups {
		if g.level == check.LevelError {
			errors = append(errors, g)
		} else {
			warnings = append(warnings, g)
		}
	}

	// Print errors section
	if len(errors) > 0 {
		fmt.Printf("%s %s\n", ui.SymbolAttention, ui.Bold.Render("Errors"))
		for _, g := range errors {
			printIssueGroup(g)
		}
	}

	// Print warnings section
	if len(warnings) > 0 {
		if len(errors) > 0 {
			fmt.Println() // blank line between sections
		}
		fmt.Printf("%s %s\n", ui.SymbolAttention, ui.Bold.Render("Warnings"))
		for _, g := range warnings {
			printIssueGroup(g)
		}
	}
}

// printIssueGroup prints a single issue group on one line
func printIssueGroup(g *issueGroup) {
	// Format: issue_type (count)  examples...
	issueLabel := ui.Bold.Render(string(g.issueType))
	countStr := fmt.Sprintf("(%d)", g.count)

	if len(g.topValues) > 0 {
		// Show examples with ellipsis if there might be more
		examples := strings.Join(g.topValues, ", ")
		if g.count > len(g.topValues) {
			examples += ", ..."
		}
		fmt.Printf("  %s %s  %s\n", issueLabel, countStr, ui.Muted.Render("("+examples+")"))
	} else {
		fmt.Printf("  %s %s\n", issueLabel, countStr)
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

func handleMissingRefs(vaultPath string, s *schema.Schema, refs []*check.MissingRef, objectsRoot, pagesRoot string) int {
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot)

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

	fmt.Printf("\n%s\n", ui.SectionHeader("Missing References"))
	reader := bufio.NewReader(os.Stdin)
	created := 0
	resolvePath := func(targetPath, typeName string) string {
		return creator.resolveAndSlugifyTargetPath(targetPath, typeName)
	}

	// Handle certain refs (from typed fields)
	if len(certain) > 0 {
		fmt.Printf("\n%s\n", ui.Bold.Render("Certain (from typed fields):"))
		for _, ref := range certain {
			source := ref.SourceObjectID
			if source == "" {
				source = ref.SourceFile
			}
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			item := fmt.Sprintf("%s → %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(from %s.%s)", source, ref.FieldSource)))
			fmt.Println(ui.Bullet(item))
		}

		fmt.Printf("\nCreate these pages? %s ", ui.Muted.Render("[Y/n]"))
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "" || response == "y" || response == "yes" {
			for _, ref := range certain {
				resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot); err != nil {
					fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
				} else {
					fmt.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, ref.InferredType))
					created++
				}
			}
		}
	}

	// Handle inferred refs (from path matching)
	if len(inferred) > 0 {
		fmt.Printf("\n%s\n", ui.Bold.Render("Inferred (from path matching default_path):"))
		for _, ref := range inferred {
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			item := fmt.Sprintf("? %s → %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(type: %s)", ref.InferredType)))
			fmt.Println(ui.Bullet(item))
		}

		for _, ref := range inferred {
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			fmt.Printf("\nCreate %s as '%s'? %s ", ui.FilePath(resolvedPath+".md"), ui.Bold.Render(ref.InferredType), ui.Muted.Render("[y/N]"))
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "y" || response == "yes" {
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot); err != nil {
					fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
				} else {
					fmt.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, ref.InferredType))
					created++
				}
			}
		}
	}

	// Handle unknown refs
	if len(unknown) > 0 {
		fmt.Printf("\n%s\n", ui.Bold.Render("Unknown type (please specify):"))
		for _, ref := range unknown {
			item := fmt.Sprintf("? %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.Muted.Render(fmt.Sprintf("(referenced in %s:%d)", ref.SourceFile, ref.Line)))
			fmt.Println(ui.Bullet(item))
		}

		// List available types
		var typeNames []string
		for name := range s.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		fmt.Printf("\nAvailable types: %s\n", ui.Bold.Render(strings.Join(typeNames, ", ")))

		for _, ref := range unknown {
			fmt.Printf("\nType for %s %s: ", ui.Bold.Render(ref.TargetPath), ui.Muted.Render("(or 'skip')"))
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(response)

			if response == "" || response == "skip" || response == "s" {
				fmt.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
				continue
			}

			// Validate type exists, offer to create if not
			if _, exists := s.Types[response]; !exists {
				created += handleNewTypeCreation(vaultPath, s, ref, response, reader, objectsRoot, pagesRoot)
				continue
			}

			resolvedPath := resolvePath(ref.TargetPath, response)
			if err := createMissingPage(vaultPath, s, ref.TargetPath, response, objectsRoot, pagesRoot); err != nil {
				fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
			} else {
				fmt.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, response))
				created++
			}
		}
	}

	return created
}

// createMissingRefsNonInteractive creates missing refs deterministically for agent/json mode.
// It only creates refs with a known type and skips unknown-type refs that require user input.
func createMissingRefsNonInteractive(vaultPath string, s *schema.Schema, refs []*check.MissingRef, objectsRoot, pagesRoot string) int {
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot)
	created := 0
	seen := make(map[string]struct{})

	for _, ref := range refs {
		typeName := ref.InferredType
		if typeName == "" {
			// Unknown type requires user input; skip in non-interactive mode.
			continue
		}
		if _, exists := s.Types[typeName]; !exists && !schema.IsBuiltinType(typeName) {
			// Defensive check: skip refs whose inferred type isn't currently known.
			continue
		}

		resolvedPath := creator.resolveTargetPath(ref.TargetPath, typeName)
		slugPath := pages.SlugifyPath(resolvedPath)
		if _, alreadyHandled := seen[slugPath]; alreadyHandled {
			continue
		}
		seen[slugPath] = struct{}{}

		if creator.exists(ref.TargetPath, typeName) {
			continue
		}

		if err := createMissingPage(vaultPath, s, ref.TargetPath, typeName, objectsRoot, pagesRoot); err == nil {
			created++
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

	fmt.Printf("\n%s\n", ui.SectionHeader("Undefined Traits"))
	fmt.Println("\nThe following traits are used but not defined in schema.yaml:")
	for _, trait := range traits {
		valueInfo := "no value"
		if trait.HasValue {
			valueInfo = "with value"
		}
		item := fmt.Sprintf("%s %s",
			ui.Bold.Render("@"+trait.TraitName),
			ui.Muted.Render(fmt.Sprintf("(%d usages, %s)", trait.UsageCount, valueInfo)))
		fmt.Println(ui.Bullet(item))
		for _, loc := range trait.Locations {
			fmt.Printf("      %s\n", ui.Muted.Render(loc))
		}
	}

	reader := bufio.NewReader(os.Stdin)
	added := 0

	fmt.Println("\nWould you like to add these traits to the schema?")

	for _, trait := range traits {
		fmt.Printf("\nAdd %s to schema? %s ", ui.Bold.Render("@"+trait.TraitName), ui.Muted.Render("[y/N]"))
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

		fmt.Printf("  %s\n", ui.Checkf("Added trait '@%s' (type: %s) to schema.yaml", trait.TraitName, traitType))
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
		ui.Bold.Render("@"+trait.TraitName),
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
func handleNewTypeCreation(vaultPath string, s *schema.Schema, ref *check.MissingRef, typeName string, reader *bufio.Reader, objectsRoot, pagesRoot string) int {
	fmt.Printf("\n  Type %s doesn't exist. Would you like to create it? %s ",
		ui.Bold.Render("'"+typeName+"'"),
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
	fmt.Printf("  %s\n", ui.Checkf("Created type '%s' in schema.yaml", typeName))
	if defaultPath != "" {
		fmt.Printf("    %s\n", ui.Muted.Render("default_path: "+defaultPath))
	}

	// Now create the page with the new type (resolving path with new default_path)
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot)
	resolvedPath := creator.resolveAndSlugifyTargetPath(ref.TargetPath, typeName)
	if err := createMissingPage(vaultPath, s, ref.TargetPath, typeName, objectsRoot, pagesRoot); err != nil {
		fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
		return 0
	}
	fmt.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, typeName))
	return 1
}

// createNewType adds a new type to schema.yaml.
func createNewType(vaultPath string, s *schema.Schema, typeName, defaultPath string) error {
	schemaPath := paths.SchemaPath(vaultPath)

	// Check built-in types
	if schema.IsBuiltinType(typeName) {
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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
func createMissingPage(vaultPath string, s *schema.Schema, targetPath, typeName, objectsRoot, pagesRoot string) error {
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot)
	_, err := creator.create(objectCreateParams{
		typeName:                    typeName,
		targetPath:                  targetPath,
		includeRequiredPlaceholders: true,
	})
	return err
}

// fixType describes how to apply a fix
type fixType string

const (
	fixTypeWikilink fixType = "wikilink" // Replace [[old]] with [[new]]
	fixTypeTrait    fixType = "trait"    // Replace @trait(old) with @trait(new)
)

// fixableIssue represents an issue that can be auto-fixed
type fixableIssue struct {
	filePath    string
	line        int
	issueType   check.IssueType
	fixType     fixType
	oldValue    string // The current value (e.g., short ref)
	newValue    string // The replacement value (e.g., full path)
	traitName   string // For trait fixes, the trait name
	description string // Human-readable description
}

// fixResult tracks the result of fix operations
type fixResult struct {
	fileCount  int
	issueCount int
}

// collectFixableIssues identifies issues that can be auto-fixed.
// Only truly unambiguous fixes are included - we never guess about user intent.
func collectFixableIssues(issues []check.Issue, shortRefMap map[string]string, s *schema.Schema) []fixableIssue {
	var fixable []fixableIssue

	for _, issue := range issues {
		switch issue.Type {
		case check.IssueShortRefCouldBeFullPath:
			// Look up the full path for this short ref
			if fullPath, ok := shortRefMap[issue.Value]; ok {
				fixable = append(fixable, fixableIssue{
					filePath:    issue.FilePath,
					line:        issue.Line,
					issueType:   issue.Type,
					fixType:     fixTypeWikilink,
					oldValue:    issue.Value,
					newValue:    fullPath,
					description: fmt.Sprintf("[[%s]] → [[%s]]", issue.Value, fullPath),
				})
			}

		case check.IssueInvalidEnumValue:
			// Check if the value is quoted and the unquoted value is valid
			if fix := tryFixQuotedEnumValue(issue, s); fix != nil {
				fixable = append(fixable, *fix)
			}

			// Note: We intentionally do NOT auto-fix missing references.
			// Even "obvious" typos like project/ → projects/ are ambiguous -
			// we can't know if the user meant the singular or plural form.
		}
	}

	return fixable
}

// tryFixQuotedEnumValue checks if an invalid enum value is just quoted
// and the unquoted value is valid.
func tryFixQuotedEnumValue(issue check.Issue, s *schema.Schema) *fixableIssue {
	value := issue.Value

	// Check for single or double quotes
	var unquoted string
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"') {
			unquoted = value[1 : len(value)-1]
		}
	}

	if unquoted == "" {
		return nil
	}

	// Extract trait name from the message
	// Message format: "Invalid value 'X' for trait '@traitname' (allowed: [...])"
	traitName := extractTraitNameFromMessage(issue.Message)
	if traitName == "" {
		return nil
	}

	// Check if unquoted value is valid for this trait
	traitDef, exists := s.Traits[traitName]
	if !exists || traitDef.Type != schema.FieldTypeEnum {
		return nil
	}

	for _, allowed := range traitDef.Values {
		if allowed == unquoted {
			return &fixableIssue{
				filePath:    issue.FilePath,
				line:        issue.Line,
				issueType:   issue.Type,
				fixType:     fixTypeTrait,
				oldValue:    value,
				newValue:    unquoted,
				traitName:   traitName,
				description: fmt.Sprintf("@%s(%s) → @%s(%s)", traitName, value, traitName, unquoted),
			}
		}
	}

	return nil
}

// extractTraitNameFromMessage extracts the trait name from an error message.
func extractTraitNameFromMessage(msg string) string {
	// Look for pattern: "for trait '@traitname'"
	const prefix = "for trait '@"
	idx := strings.Index(msg, prefix)
	if idx == -1 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.Index(msg[start:], "'")
	if end == -1 {
		return ""
	}
	return msg[start : start+end]
}

// handleAutoFix applies auto-fixes to the vault
func handleAutoFix(vaultPath string, fixes []fixableIssue, confirm bool) (fixResult, error) {
	result := fixResult{}

	// Group fixes by file
	fixesByFile := make(map[string][]fixableIssue)
	for _, fix := range fixes {
		fixesByFile[fix.filePath] = append(fixesByFile[fix.filePath], fix)
	}

	// Sort files for consistent output
	var filePaths []string
	for fp := range fixesByFile {
		filePaths = append(filePaths, fp)
	}
	sort.Strings(filePaths)

	if !confirm {
		// Preview mode
		fmt.Printf("\n%s\n", ui.SectionHeader("Auto-fixable Issues"))
		fmt.Println(ui.Hint("Use --confirm to apply these fixes."))
		fmt.Println()

		for _, filePath := range filePaths {
			fileFixest := fixesByFile[filePath]
			fmt.Printf("%s %s\n", ui.FilePath(filePath), ui.Muted.Render(fmt.Sprintf("(%d fix%s)", len(fileFixest), pluralize(len(fileFixest)))))
			for _, fix := range fileFixest {
				fmt.Printf("  %s %s\n", ui.Muted.Render(fmt.Sprintf("L%d", fix.line)), fix.description)
			}
		}
		fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Total: %d fixable issue(s) in %d file(s)", len(fixes), len(filePaths))))
		return result, nil
	}

	// Apply mode
	fmt.Printf("\n%s\n", ui.SectionHeader("Applying Fixes"))

	for _, filePath := range filePaths {
		fileFixes := fixesByFile[filePath]
		fullPath := filepath.Join(vaultPath, filePath)

		content, err := os.ReadFile(fullPath)
		if err != nil {
			return result, fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		newContent := string(content)
		fixedCount := 0

		// Apply each fix
		// Sort by line descending so we don't mess up line numbers as we edit
		sort.Slice(fileFixes, func(i, j int) bool {
			return fileFixes[i].line > fileFixes[j].line
		})

		for _, fix := range fileFixes {
			var oldPattern, newPattern string

			switch fix.fixType {
			case fixTypeWikilink:
				oldPattern = "[[" + fix.oldValue + "]]"
				newPattern = "[[" + fix.newValue + "]]"
			case fixTypeTrait:
				// Replace @trait(oldValue) with @trait(newValue)
				oldPattern = "@" + fix.traitName + "(" + fix.oldValue + ")"
				newPattern = "@" + fix.traitName + "(" + fix.newValue + ")"
			default:
				continue
			}

			// Replace in content
			if strings.Contains(newContent, oldPattern) {
				newContent = strings.ReplaceAll(newContent, oldPattern, newPattern)
				fixedCount++
			}
		}

		if fixedCount > 0 {
			if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
				return result, fmt.Errorf("failed to write %s: %w", filePath, err)
			}
			fmt.Printf("%s %s\n", ui.SymbolCheck, ui.FilePath(filePath))
			result.fileCount++
			result.issueCount += fixedCount
		}
	}

	return result, nil
}

// pluralize returns "es" for counts != 1
func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

func init() {
	checkCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	checkCmd.Flags().BoolVar(&checkCreateMissing, "create-missing", false, "Create missing referenced pages (interactive by default; with --json requires --confirm)")
	checkCmd.Flags().BoolVar(&checkByFile, "by-file", false, "Group issues by file path")
	checkCmd.Flags().BoolVarP(&checkVerbose, "verbose", "V", false, "Show all issues with full details")
	checkCmd.Flags().StringVarP(&checkType, "type", "t", "", "Check only objects of this type")
	checkCmd.Flags().StringVar(&checkTrait, "trait", "", "Check only usages of this trait")
	checkCmd.Flags().StringVar(&checkIssues, "issues", "", "Only check these issue types (comma-separated)")
	checkCmd.Flags().StringVar(&checkExclude, "exclude", "", "Exclude these issue types (comma-separated)")
	checkCmd.Flags().BoolVar(&checkErrorsOnly, "errors-only", false, "Only report errors, skip warnings")
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Auto-fix simple issues (short refs → full paths)")
	checkCmd.Flags().BoolVar(&checkConfirm, "confirm", false, "Apply fixes/create-missing in non-interactive mode (without this flag, shows preview only)")
	rootCmd.AddCommand(checkCmd)
}
