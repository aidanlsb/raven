package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/paths"
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

		// Load schema
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
			switch result.Scope.Type {
			case "full":
				fmt.Printf("Checking vault: %s\n", ui.Muted.Render(vaultPath))
			case "file":
				fmt.Printf("Checking file: %s\n", ui.FilePath(result.Scope.Value))
			case "directory":
				fmt.Printf("Checking directory: %s\n", ui.FilePath(result.Scope.Value+"/"))
			case "type_filter":
				fmt.Printf("Checking type: %s\n", ui.Bold.Render(result.Scope.Value))
			case "trait_filter":
				fmt.Printf("Checking trait: %s\n", ui.Bold.Render("@"+result.Scope.Value))
			}
		}

		if jsonOutput && checkCreateMissing && result.Scope.Type == "full" && checkConfirm {
			checksvc.CreateMissingRefsNonInteractive(
				vaultPath,
				s,
				result.MissingRefs,
				vaultCfg.GetObjectsRoot(),
				vaultCfg.GetPagesRoot(),
				vaultCfg.GetTemplateDirectory(),
			)
		}

		if jsonOutput {
			outputSuccess(checksvc.BuildJSON(vaultPath, result), nil)
		} else if checkByFile {
			// Group issues by file
			printIssuesByFile(result.Issues, result.SchemaIssues, result.StaleWarningShown)
			fmt.Println()
			if result.ErrorCount == 0 && result.WarningCount == 0 {
				fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
			} else {
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarningCount, result.FileCount)
			}
		} else if checkVerbose {
			// Verbose mode: print all issues inline
			printIssuesVerbose(result.Issues, result.SchemaIssues)
			fmt.Println()
			if result.ErrorCount == 0 && result.WarningCount == 0 {
				fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
			} else {
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarningCount, result.FileCount)
			}
		} else {
			// Default: summary by issue type
			fmt.Println()
			if result.ErrorCount == 0 && result.WarningCount == 0 {
				fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
			} else {
				printIssueSummary(result.Issues, result.SchemaIssues)
				fmt.Println()
				fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarningCount, result.FileCount)
				fmt.Println(ui.Hint("Use --verbose to see all issues, or --by-file to group by file."))
			}

			// Handle --create-missing (interactive mode only, full vault check only)
			if checkCreateMissing && result.Scope.Type == "full" {
				if len(result.MissingRefs) > 0 {
					created := handleMissingRefs(vaultPath, s, result.MissingRefs, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot(), vaultCfg.GetTemplateDirectory())
					if created > 0 {
						fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
					}
				}

				if len(result.UndefinedTraits) > 0 {
					added := handleUndefinedTraits(vaultPath, s, result.UndefinedTraits)
					if added > 0 {
						fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
					}
				}
			}

			// Handle --fix: auto-fix simple issues
			if checkFix {
				fixableIssues := collectFixableIssues(result.Issues, result.ShortRefs, s)

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

		if result.ErrorCount > 0 || (checkStrict && result.WarningCount > 0) {
			os.Exit(1)
		}

		return nil
	},
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

func handleMissingRefs(vaultPath string, s *schema.Schema, refs []*check.MissingRef, objectsRoot, pagesRoot, templateDir string) int {
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot, templateDir)

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
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot, templateDir); err != nil {
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
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot, templateDir); err != nil {
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
				created += handleNewTypeCreation(vaultPath, s, ref, response, reader, objectsRoot, pagesRoot, templateDir)
				continue
			}

			resolvedPath := resolvePath(ref.TargetPath, response)
			if err := createMissingPage(vaultPath, s, ref.TargetPath, response, objectsRoot, pagesRoot, templateDir); err != nil {
				fmt.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", resolvedPath, err))
			} else {
				fmt.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", resolvedPath, response))
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
		ui.Muted.Render("[boolean/string/number/date/datetime/enum/ref/url]"),
		ui.Muted.Render(fmt.Sprintf("(default: %s)", suggested)))
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

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
func handleNewTypeCreation(vaultPath string, s *schema.Schema, ref *check.MissingRef, typeName string, reader *bufio.Reader, objectsRoot, pagesRoot, templateDir string) int {
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
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot, templateDir)
	resolvedPath := creator.resolveAndSlugifyTargetPath(ref.TargetPath, typeName)
	if err := createMissingPage(vaultPath, s, ref.TargetPath, typeName, objectsRoot, pagesRoot, templateDir); err != nil {
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
func createMissingPage(vaultPath string, s *schema.Schema, targetPath, typeName, objectsRoot, pagesRoot, templateDir string) error {
	creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot, templateDir)
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
