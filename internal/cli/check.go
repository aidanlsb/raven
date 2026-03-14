package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
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

type CheckIssueJSON = checksvc.CheckIssueJSON
type CheckSummaryJSON = checksvc.CheckSummaryJSON
type CheckScopeJSON = checksvc.CheckScopeJSON
type CheckResultJSON = checksvc.CheckResultJSON

type checkAction string

const (
	checkActionValidateOnly  checkAction = "validate"
	checkActionFix           checkAction = "fix"
	checkActionCreateMissing checkAction = "create-missing"
)

var checkCmd = &cobra.Command{
	Use:   "check [path]",
	Short: "Validate the vault",
	Long:  `Checks all files for errors and warnings (type mismatches, broken references, etc.)`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := checkActionValidateOnly
		if checkFix && checkCreateMissing {
			return handleErrorMsg(ErrInvalidInput, "cannot combine --fix with --create-missing", "Use one action at a time")
		}
		if checkFix {
			action = checkActionFix
		}
		if checkCreateMissing {
			action = checkActionCreateMissing
		}
		return runCheckCommand(args, action, true)
	},
}

var checkFixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Preview or apply safe auto-fixes for check findings",
	Long: `Runs check, then previews or applies only unambiguous safe fixes.

Preview is default; use --confirm to apply.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheckCommand(args, checkActionFix, false)
	},
}

var checkCreateMissingCmd = &cobra.Command{
	Use:   "create-missing",
	Short: "Create missing references discovered by check",
	Long: `Runs check, then creates missing referenced pages.

Non-JSON mode prompts interactively.
JSON mode requires --confirm and creates only deterministic typed targets.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheckCommand(args, checkActionCreateMissing, false)
	},
}

func runCheckCommand(args []string, action checkAction, legacyFlagInvocation bool) error {
	vaultPath := getVaultPath()

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	s, err := schema.Load(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	var pathArg string
	if len(args) > 0 {
		pathArg = args[0]
	}

	result, err := checksvc.Run(vaultPath, vaultCfg, s, checksvc.Options{
		PathArg:     pathArg,
		TypeFilter:  checkType,
		TraitFilter: checkTrait,
		Issues:      checkIssues,
		Exclude:     checkExclude,
		ErrorsOnly:  checkErrorsOnly,
	})
	if err != nil {
		return err
	}

	if !jsonOutput {
		printCheckScopeHeader(vaultPath, result.Scope)
	}

	switch action {
	case checkActionValidateOnly:
		runCheckValidateOutput(vaultPath, result)
	case checkActionFix:
		runCheckFixOutput(vaultPath, s, result)
	case checkActionCreateMissing:
		if result.Scope.Type != "full" {
			if legacyFlagInvocation {
				if !jsonOutput {
					fmt.Println(ui.Hint("`--create-missing` is ignored for scoped checks; run on full vault to create pages."))
				}
			} else {
				return handleErrorMsg(ErrInvalidInput, "check create-missing only supports full-vault scope", "Run without path/--type/--trait filters")
			}
		} else {
			runCheckCreateMissingOutput(vaultPath, vaultCfg, s, result)
		}
	default:
		return handleErrorMsg(ErrInvalidInput, "unknown check action", "")
	}

	if result.ErrorCount > 0 || (checkStrict && result.WarningCount > 0) {
		os.Exit(1)
	}

	return nil
}

func printCheckScopeHeader(vaultPath string, scope checksvc.Scope) {
	switch scope.Type {
	case "full":
		fmt.Printf("Checking vault: %s\n", ui.Muted.Render(vaultPath))
	case "file":
		fmt.Printf("Checking file: %s\n", ui.FilePath(scope.Value))
	case "directory":
		fmt.Printf("Checking directory: %s\n", ui.FilePath(scope.Value+"/"))
	case "type_filter":
		fmt.Printf("Checking type: %s\n", ui.Bold.Render(scope.Value))
	case "trait_filter":
		fmt.Printf("Checking trait: %s\n", ui.Bold.Render("@"+scope.Value))
	}
}

func runCheckValidateOutput(vaultPath string, result *checksvc.RunResult) {
	if jsonOutput {
		outputSuccess(checksvc.BuildJSON(vaultPath, result), nil)
		return
	}

	if checkByFile {
		printIssuesByFile(result.Issues, result.SchemaIssues, result.StaleWarningShown)
		fmt.Println()
		if result.ErrorCount == 0 && result.WarningCount == 0 {
			fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarningCount, result.FileCount)
		}
		return
	}

	if checkVerbose {
		printIssuesVerbose(result.Issues, result.SchemaIssues)
		fmt.Println()
		if result.ErrorCount == 0 && result.WarningCount == 0 {
			fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarningCount, result.FileCount)
		}
		return
	}

	fmt.Println()
	if result.ErrorCount == 0 && result.WarningCount == 0 {
		fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
	} else {
		printIssueSummary(result.Issues, result.SchemaIssues)
		fmt.Println()
		fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarningCount, result.FileCount)
		fmt.Println(ui.Hint("Use --verbose to see all issues, or --by-file to group by file."))
	}
}

func runCheckFixOutput(vaultPath string, sch *schema.Schema, result *checksvc.RunResult) {
	fixes := checksvc.CollectFixableIssues(result.Issues, result.ShortRefs, sch)
	grouped := checksvc.GroupFixesByFile(fixes)

	if jsonOutput {
		if !checkConfirm {
			outputSuccess(map[string]interface{}{
				"preview":        true,
				"fixable_issues": len(fixes),
				"files":          grouped,
			}, nil)
			return
		}
		applied, err := checksvc.ApplyFixes(vaultPath, fixes)
		if err != nil {
			outputError(ErrValidationFailed, err.Error(), nil, "")
			return
		}
		outputSuccess(map[string]interface{}{
			"preview":        false,
			"fixable_issues": len(fixes),
			"fixed_issues":   applied.IssueCount,
			"fixed_files":    applied.FileCount,
		}, nil)
		return
	}

	if len(fixes) == 0 {
		fmt.Println(ui.Hint("\nNo auto-fixable issues found."))
		return
	}

	if !checkConfirm {
		fmt.Printf("\n%s\n", ui.SectionHeader("Auto-fixable Issues"))
		fmt.Println(ui.Hint("Use --confirm to apply these fixes."))
		fmt.Println()
		for _, file := range grouped {
			fmt.Printf("%s %s\n", ui.FilePath(file.FilePath), ui.Muted.Render(fmt.Sprintf("(%d fix%s)", len(file.Fixes), pluralize(len(file.Fixes)))))
			for _, fix := range file.Fixes {
				fmt.Printf("  %s %s\n", ui.Muted.Render(fmt.Sprintf("L%d", fix.Line)), fix.Description)
			}
		}
		fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Total: %d fixable issue(s) in %d file(s)", len(fixes), len(grouped))))
		return
	}

	applied, err := checksvc.ApplyFixes(vaultPath, fixes)
	if err != nil {
		fmt.Printf("\n%s\n", ui.Errorf("Fix failed: %v", err))
		return
	}
	fmt.Printf("\n%s\n", ui.Checkf("Fixed %d issue(s) in %d file(s).", applied.IssueCount, applied.FileCount))
}

func runCheckCreateMissingOutput(vaultPath string, vaultCfg interface {
	GetObjectsRoot() string
	GetPagesRoot() string
	GetTemplateDirectory() string
}, sch *schema.Schema, result *checksvc.RunResult) {
	if jsonOutput {
		if !checkConfirm {
			outputSuccess(map[string]interface{}{
				"preview":              true,
				"missing_refs":         len(result.MissingRefs),
				"undefined_traits":     len(result.UndefinedTraits),
				"requires_confirm":     true,
				"non_interactive_only": true,
			}, nil)
			return
		}
		created := checksvc.CreateMissingRefsNonInteractive(
			vaultPath,
			sch,
			result.MissingRefs,
			vaultCfg.GetObjectsRoot(),
			vaultCfg.GetPagesRoot(),
			vaultCfg.GetTemplateDirectory(),
		)
		outputSuccess(map[string]interface{}{
			"preview":               false,
			"created_pages":         created,
			"missing_refs":          len(result.MissingRefs),
			"undefined_traits":      len(result.UndefinedTraits),
			"undefined_traits_note": "undefined traits are interactive-only and were not changed in JSON mode",
		}, nil)
		return
	}

	if len(result.MissingRefs) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		created := handleMissingRefsInteractive(vaultPath, sch, result.MissingRefs, interaction, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot(), vaultCfg.GetTemplateDirectory())
		if created > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
		}
		added := 0
		if len(result.UndefinedTraits) > 0 {
			added = handleUndefinedTraitsInteractive(vaultPath, sch, result.UndefinedTraits, interaction)
		}
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
		return
	}
	if len(result.UndefinedTraits) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		added := handleUndefinedTraitsInteractive(vaultPath, sch, result.UndefinedTraits, interaction)
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
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

func handleMissingRefsInteractive(vaultPath string, s *schema.Schema, refs []*check.MissingRef, interaction checkInteraction, objectsRoot, pagesRoot, templateDir string) int {
	groups := checksvc.GroupMissingRefsForInteractive(refs)

	interaction.Printf("\n%s\n", ui.SectionHeader("Missing References"))
	created := 0
	resolvePath := func(targetPath, typeName string) string {
		return checksvc.ResolveAndSlugifyTargetPath(targetPath, typeName, s, objectsRoot, pagesRoot)
	}

	// Handle certain refs (from typed fields)
	if len(groups.Certain) > 0 {
		interaction.Printf("\n%s\n", ui.Bold.Render("Certain (from typed fields):"))
		for _, ref := range groups.Certain {
			source := ref.SourceObjectID
			if source == "" {
				source = ref.SourceFile
			}
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			item := fmt.Sprintf("%s → %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(from %s.%s)", source, ref.FieldSource)))
			interaction.Println(ui.Bullet(item))
		}

		interaction.Printf("\nCreate these pages? %s ", ui.Muted.Render("[Y/n]"))
		response := readTrimmedLowerLine(interaction)
		if response == "" || response == "y" || response == "yes" {
			for _, ref := range groups.Certain {
				resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
				if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot, templateDir); err != nil {
					interaction.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
				} else {
					interaction.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, ref.InferredType))
					created++
				}
			}
		}
	}

	// Handle inferred refs (from path matching)
	if len(groups.Inferred) > 0 {
		interaction.Printf("\n%s\n", ui.Bold.Render("Inferred (from path matching default_path):"))
		for _, ref := range groups.Inferred {
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			item := fmt.Sprintf("? %s → %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(type: %s)", ref.InferredType)))
			interaction.Println(ui.Bullet(item))
		}

		for _, ref := range groups.Inferred {
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			interaction.Printf("\nCreate %s as '%s'? %s ", ui.FilePath(resolvedPath+".md"), ui.Bold.Render(ref.InferredType), ui.Muted.Render("[y/N]"))
			response := readTrimmedLowerLine(interaction)
			if response == "y" || response == "yes" {
				if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot, templateDir); err != nil {
					interaction.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
				} else {
					interaction.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, ref.InferredType))
					created++
				}
			}
		}
	}

	// Handle unknown refs
	if len(groups.Unknown) > 0 {
		interaction.Printf("\n%s\n", ui.Bold.Render("Unknown type (please specify):"))
		for _, ref := range groups.Unknown {
			item := fmt.Sprintf("? %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.Muted.Render(fmt.Sprintf("(referenced in %s:%d)", ref.SourceFile, ref.Line)))
			interaction.Println(ui.Bullet(item))
		}

		typeNames := checksvc.AvailableTypeNames(s)
		interaction.Printf("\nAvailable types: %s\n", ui.Bold.Render(strings.Join(typeNames, ", ")))

		for _, ref := range groups.Unknown {
			interaction.Printf("\nType for %s %s: ", ui.Bold.Render(ref.TargetPath), ui.Muted.Render("(or 'skip')"))
			response := readTrimmedLine(interaction)

			if response == "" || response == "skip" || response == "s" {
				interaction.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
				continue
			}

			// Validate type exists, offer to create if not
			if _, exists := s.Types[response]; !exists {
				created += handleNewTypeCreationInteractive(vaultPath, s, ref, response, interaction, objectsRoot, pagesRoot, templateDir)
				continue
			}

			resolvedPath := resolvePath(ref.TargetPath, response)
			if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, response, objectsRoot, pagesRoot, templateDir); err != nil {
				interaction.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
			} else {
				interaction.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, response))
				created++
			}
		}
	}

	return created
}

// handleUndefinedTraits prompts the user to add undefined traits to the schema.
// Returns the number of traits added.
func handleUndefinedTraitsInteractive(vaultPath string, s *schema.Schema, traits []*check.UndefinedTrait, interaction checkInteraction) int {
	if len(traits) == 0 {
		return 0
	}

	// Sort by usage count (most used first)
	sort.Slice(traits, func(i, j int) bool {
		return traits[i].UsageCount > traits[j].UsageCount
	})

	interaction.Printf("\n%s\n", ui.SectionHeader("Undefined Traits"))
	interaction.Println("\nThe following traits are used but not defined in schema.yaml:")
	for _, trait := range traits {
		valueInfo := "no value"
		if trait.HasValue {
			valueInfo = "with value"
		}
		item := fmt.Sprintf("%s %s",
			ui.Bold.Render("@"+trait.TraitName),
			ui.Muted.Render(fmt.Sprintf("(%d usages, %s)", trait.UsageCount, valueInfo)))
		interaction.Println(ui.Bullet(item))
		for _, loc := range trait.Locations {
			interaction.Printf("      %s\n", ui.Muted.Render(loc))
		}
	}

	added := 0

	interaction.Println("\nWould you like to add these traits to the schema?")

	for _, trait := range traits {
		interaction.Printf("\nAdd %s to schema? %s ", ui.Bold.Render("@"+trait.TraitName), ui.Muted.Render("[y/N]"))
		response := readTrimmedLowerLine(interaction)

		if response != "y" && response != "yes" {
			interaction.Printf("  %s\n", ui.Muted.Render("Skipped @"+trait.TraitName))
			continue
		}

		// Determine trait type
		traitType := promptTraitType(trait, interaction)
		if traitType == "" {
			interaction.Printf("  %s\n", ui.Muted.Render("Skipped @"+trait.TraitName))
			continue
		}

		// Get additional options based on type
		var enumValues []string
		var defaultValue string

		if traitType == "enum" {
			interaction.Printf("  Enum values %s: ", ui.Muted.Render("(comma-separated, e.g., 'low,medium,high')"))
			valuesStr := readTrimmedLine(interaction)
			if valuesStr != "" {
				enumValues = strings.Split(valuesStr, ",")
				for i := range enumValues {
					enumValues[i] = strings.TrimSpace(enumValues[i])
				}
			}
		}

		if traitType == "boolean" || traitType == "enum" {
			interaction.Printf("  Default value %s: ", ui.Muted.Render("(or leave empty)"))
			defaultValue = readTrimmedLine(interaction)
		}

		// Create the trait
		if err := checksvc.AddTrait(vaultPath, s, trait.TraitName, traitType, enumValues, defaultValue); err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to add @%s: %v", trait.TraitName, err))
			continue
		}

		interaction.Printf("  %s\n", ui.Checkf("Added trait '@%s' (type: %s) to schema.yaml", trait.TraitName, traitType))
		added++
	}

	return added
}

// promptTraitType asks the user what type a trait should be.
func promptTraitType(trait *check.UndefinedTrait, interaction checkInteraction) string {
	// Suggest a type based on usage
	suggested := "boolean"
	if trait.HasValue {
		suggested = "string"
	}

	interaction.Printf("  Type for %s? %s %s: ",
		ui.Bold.Render("@"+trait.TraitName),
		ui.Muted.Render("[boolean/string/number/date/datetime/enum/ref/url]"),
		ui.Muted.Render(fmt.Sprintf("(default: %s)", suggested)))
	response := readTrimmedLowerLine(interaction)

	if response == "" {
		return suggested
	}

	validTypes := map[string]bool{
		"boolean": true, "bool": true,
		"string":   true,
		"number":   true,
		"date":     true,
		"datetime": true,
		"enum":     true,
		"ref":      true,
		"url":      true,
	}

	// Normalize bool -> boolean
	if response == "bool" {
		response = "boolean"
	}

	if !validTypes[response] {
		interaction.Printf("  %s\n", ui.Errorf("Invalid type '%s'", response))
		return ""
	}

	return response
}

// handleNewTypeCreation prompts the user to create a new type when they enter a type that doesn't exist.
// Returns the number of pages created (0 or 1).
func handleNewTypeCreationInteractive(vaultPath string, s *schema.Schema, ref *check.MissingRef, typeName string, interaction checkInteraction, objectsRoot, pagesRoot, templateDir string) int {
	interaction.Printf("\n  Type %s doesn't exist. Would you like to create it? %s ",
		ui.Bold.Render("'"+typeName+"'"),
		ui.Muted.Render("[y/N]"))
	response := readTrimmedLowerLine(interaction)

	if response != "y" && response != "yes" {
		interaction.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
		return 0
	}

	// Prompt for default_path (optional)
	interaction.Printf("  Default path for '%s' files %s: ", typeName, ui.Muted.Render(fmt.Sprintf("(e.g., '%s/', or leave empty)", typeName+"s")))
	defaultPath := readTrimmedLine(interaction)

	// Create the type
	if err := checksvc.AddType(vaultPath, s, typeName, defaultPath); err != nil {
		interaction.Printf("  %s\n", ui.Errorf("Failed to create type '%s': %v", typeName, err))
		return 0
	}
	interaction.Printf("  %s\n", ui.Checkf("Created type '%s' in schema.yaml", typeName))
	if defaultPath != "" {
		interaction.Printf("    %s\n", ui.Muted.Render("default_path: "+defaultPath))
	}

	// Now create the page with the new type (resolving path with new default_path)
	resolvedPath := checksvc.ResolveAndSlugifyTargetPath(ref.TargetPath, typeName, s, objectsRoot, pagesRoot)
	if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, typeName, objectsRoot, pagesRoot, templateDir); err != nil {
		interaction.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
		return 0
	}
	interaction.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, typeName))
	return 1
}

// pluralize returns "es" for counts != 1
func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

func init() {
	bindCheckScopeFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVarP(&checkType, "type", "t", "", "Check only objects of this type")
		cmd.Flags().StringVar(&checkTrait, "trait", "", "Check only usages of this trait")
		cmd.Flags().StringVar(&checkIssues, "issues", "", "Only check these issue types (comma-separated)")
		cmd.Flags().StringVar(&checkExclude, "exclude", "", "Exclude these issue types (comma-separated)")
		cmd.Flags().BoolVar(&checkErrorsOnly, "errors-only", false, "Only report errors, skip warnings")
	}

	bindCheckScopeFlags(checkCmd)
	bindCheckScopeFlags(checkFixCmd)

	checkCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	checkCmd.Flags().BoolVar(&checkCreateMissing, "create-missing", false, "Create missing referenced pages (interactive by default; with --json requires --confirm)")
	checkCmd.Flags().BoolVar(&checkByFile, "by-file", false, "Group issues by file path")
	checkCmd.Flags().BoolVarP(&checkVerbose, "verbose", "V", false, "Show all issues with full details")
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Auto-fix simple issues (short refs → full paths)")
	checkCmd.Flags().BoolVar(&checkConfirm, "confirm", false, "Apply fixes/create-missing in non-interactive mode (without this flag, shows preview only)")

	checkFixCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	checkFixCmd.Flags().BoolVar(&checkConfirm, "confirm", false, "Apply fixes (without this flag, shows preview only)")

	checkCreateMissingCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	checkCreateMissingCmd.Flags().BoolVar(&checkConfirm, "confirm", false, "Apply create-missing changes in non-interactive mode (without this flag, shows preview only)")

	checkCmd.AddCommand(checkFixCmd)
	checkCmd.AddCommand(checkCreateMissingCmd)
	rootCmd.AddCommand(checkCmd)
}
