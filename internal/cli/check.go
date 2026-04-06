package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandexec"
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

var checkFixCmd = newCanonicalLeafCommand("check_fix", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	BuildArgs:       buildCheckFixArgs,
	Invoke:          invokeCheckLeaf,
	HandleError:     handleCheckLeafFailure,
	HandleResult:    handleCheckFixResult,
	SkipFlagBinding: true,
})

var checkCreateMissingCmd = newCanonicalLeafCommand("check create-missing", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	BuildArgs:       buildCheckCreateMissingArgs,
	Invoke:          invokeCheckLeaf,
	HandleError:     handleCheckLeafFailure,
	HandleResult:    handleCheckCreateMissingResult,
	SkipFlagBinding: true,
})

func runCheckCommand(args []string, action checkAction, legacyFlagInvocation bool) error {
	vaultPath := getVaultPath()
	argsMap := map[string]interface{}{
		"strict":         checkStrict,
		"type":           checkType,
		"trait":          checkTrait,
		"issues":         checkIssues,
		"exclude":        checkExclude,
		"errors-only":    checkErrorsOnly,
		"by-file":        checkByFile,
		"verbose":        checkVerbose,
		"fix":            action == checkActionFix,
		"confirm":        checkConfirm,
		"create-missing": action == checkActionCreateMissing,
	}
	if len(args) > 0 {
		argsMap["path"] = args[0]
	}

	confirm := checkConfirm
	if action == checkActionCreateMissing && !jsonOutput {
		confirm = false
		argsMap["confirm"] = false
	}

	result := executeCanonicalRequest(commandexec.Request{
		CommandID: "check",
		VaultPath: vaultPath,
		Args:      argsMap,
		Confirm:   confirm,
	})
	if !result.OK {
		if legacyFlagInvocation && action == checkActionCreateMissing && result.Error != nil && result.Error.Code == ErrInvalidInput {
			if !jsonOutput {
				fmt.Println(ui.Hint("`--create-missing` is ignored for scoped checks; run on full vault to create pages."))
			}
			argsMap["create-missing"] = false
			action = checkActionValidateOnly
			result = executeCanonicalRequest(commandexec.Request{
				CommandID: "check",
				VaultPath: vaultPath,
				Args:      argsMap,
			})
		}
		if !result.OK {
			if result.Error == nil {
				return handleErrorMsg(ErrInternal, "check failed", "")
			}
			if result.Error.Details != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(result.Error.Code, result.Error.Message, result.Error.Suggestion)
		}
	}

	if jsonOutput {
		outputJSON(result)
		if checkShouldExit(result) {
			os.Exit(1)
		}
		return nil
	}

	printCheckScopeHeader(vaultPath, checkScopeFromResult(result))

	switch action {
	case checkActionValidateOnly:
		renderCanonicalCheckValidate(result)
	case checkActionFix:
		renderCanonicalCheckFix(result)
	case checkActionCreateMissing:
		if err := renderCanonicalCheckCreateMissing(vaultPath, result); err != nil {
			return err
		}
	default:
		return handleErrorMsg(ErrInvalidInput, "unknown check action", "")
	}

	if checkShouldExit(result) {
		os.Exit(1)
	}

	return nil
}

func buildCheckFixArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	argsMap := map[string]interface{}{
		"strict":      checkStrict,
		"type":        checkType,
		"trait":       checkTrait,
		"issues":      checkIssues,
		"exclude":     checkExclude,
		"errors-only": checkErrorsOnly,
		"confirm":     checkConfirm,
	}
	if len(args) > 0 {
		argsMap["path"] = args[0]
	}
	return argsMap, nil
}

func buildCheckCreateMissingArgs(_ *cobra.Command, _ []string) (map[string]interface{}, error) {
	confirm := checkConfirm
	if !jsonOutput {
		confirm = false
	}
	return map[string]interface{}{
		"strict":  checkStrict,
		"confirm": confirm,
	}, nil
}

func invokeCheckLeaf(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm := checkConfirm
	if commandID == "check create-missing" && !jsonOutput {
		confirm = false
	}
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   confirm,
	})
}

func handleCheckLeafFailure(result commandexec.Result) error {
	if result.Error == nil {
		return handleErrorMsg(ErrInternal, "check failed", "")
	}
	if result.Error.Details != nil {
		return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
	}
	return handleErrorMsg(result.Error.Code, result.Error.Message, result.Error.Suggestion)
}

func handleCheckFixResult(_ *cobra.Command, result commandexec.Result) error {
	if jsonOutput {
		outputJSON(result)
		if checkShouldExit(result) {
			os.Exit(1)
		}
		return nil
	}
	printCheckScopeHeader(getVaultPath(), checkScopeFromResult(result))
	renderCanonicalCheckFix(result)
	if checkShouldExit(result) {
		os.Exit(1)
	}
	return nil
}

func handleCheckCreateMissingResult(_ *cobra.Command, result commandexec.Result) error {
	if jsonOutput {
		outputJSON(result)
		if checkShouldExit(result) {
			os.Exit(1)
		}
		return nil
	}
	printCheckScopeHeader(getVaultPath(), checkScopeFromResult(result))
	if err := renderCanonicalCheckCreateMissing(getVaultPath(), result); err != nil {
		return err
	}
	if checkShouldExit(result) {
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

func checkScopeFromResult(result commandexec.Result) checksvc.Scope {
	data := canonicalDataMap(result)
	if scopeMap, ok := data["scope"].(map[string]interface{}); ok {
		return checksvc.Scope{
			Type:  stringValue(scopeMap["type"]),
			Value: stringValue(scopeMap["value"]),
		}
	}
	if decoded, ok := decodeCanonicalCheckJSON(result); ok && decoded.Scope != nil {
		return checksvc.Scope{
			Type:  decoded.Scope.Type,
			Value: decoded.Scope.Value,
		}
	}
	return checksvc.Scope{Type: "full"}
}

func checkShouldExit(result commandexec.Result) bool {
	data := canonicalDataMap(result)
	errorCount := intValue(data["error_count"])
	warningCount := intValue(data["warning_count"])
	if errorCount == 0 && warningCount == 0 {
		if decoded, ok := decodeCanonicalCheckJSON(result); ok {
			errorCount = decoded.ErrorCount
			warningCount = decoded.WarnCount
		}
	}
	return errorCount > 0 || (checkStrict && warningCount > 0)
}

func decodeCanonicalCheckJSON(result commandexec.Result) (CheckResultJSON, bool) {
	var decoded CheckResultJSON
	data := canonicalDataMap(result)
	if len(data) == 0 {
		return decoded, false
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return decoded, false
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return decoded, false
	}
	return decoded, true
}

func renderCanonicalCheckValidate(result commandexec.Result) {
	decoded, ok := decodeCanonicalCheckJSON(result)
	if !ok {
		fmt.Println(ui.Warning("failed to decode check results"))
		return
	}

	if checkByFile {
		printIssuesByFileFromJSON(decoded.Issues)
		fmt.Println()
		if decoded.ErrorCount == 0 && decoded.WarnCount == 0 {
			fmt.Println(ui.Starf("No issues found in %d files.", decoded.FileCount))
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", decoded.ErrorCount, decoded.WarnCount, decoded.FileCount)
		}
		return
	}

	if checkVerbose {
		printIssuesVerboseFromJSON(decoded.Issues)
		fmt.Println()
		if decoded.ErrorCount == 0 && decoded.WarnCount == 0 {
			fmt.Println(ui.Starf("No issues found in %d files.", decoded.FileCount))
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", decoded.ErrorCount, decoded.WarnCount, decoded.FileCount)
		}
		return
	}

	fmt.Println()
	if decoded.ErrorCount == 0 && decoded.WarnCount == 0 {
		fmt.Println(ui.Starf("No issues found in %d files.", decoded.FileCount))
		return
	}
	printIssueSummaryFromJSON(decoded.Summary, decoded.Issues)
	fmt.Println()
	fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", decoded.ErrorCount, decoded.WarnCount, decoded.FileCount)
	fmt.Println(ui.Hint("Use --verbose to see all issues, or --by-file to group by file."))
}

func renderCanonicalCheckFix(result commandexec.Result) {
	data := canonicalDataMap(result)
	fixableIssues := intValue(data["fixable_issues"])
	preview := boolValue(data["preview"])

	if fixableIssues == 0 {
		fmt.Println(ui.Hint("\nNo auto-fixable issues found."))
		return
	}

	if preview {
		fmt.Printf("\n%s\n", ui.SectionHeader("Auto-fixable Issues"))
		fmt.Println(ui.Hint("Use --confirm to apply these fixes."))
		fmt.Println()
		switch grouped := data["files"].(type) {
		case []checksvc.FileFixes:
			for _, file := range grouped {
				fmt.Printf("%s %s\n", ui.FilePath(file.FilePath), ui.Muted.Render(fmt.Sprintf("(%d fix%s)", len(file.Fixes), pluralize(len(file.Fixes)))))
				for _, fix := range file.Fixes {
					fmt.Printf("  %s %s\n", ui.Muted.Render(fmt.Sprintf("L%d", fix.Line)), fix.Description)
				}
			}
			fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Total: %d fixable issue(s) in %d file(s)", fixableIssues, len(grouped))))
			return
		case []interface{}:
			for _, raw := range grouped {
				file, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				fixes, _ := file["fixes"].([]interface{})
				fmt.Printf("%s %s\n", ui.FilePath(stringValue(file["file_path"])), ui.Muted.Render(fmt.Sprintf("(%d fix%s)", len(fixes), pluralize(len(fixes)))))
				for _, fixRaw := range fixes {
					fix, ok := fixRaw.(map[string]interface{})
					if !ok {
						continue
					}
					fmt.Printf("  %s %s\n", ui.Muted.Render(fmt.Sprintf("L%d", intValue(fix["line"]))), stringValue(fix["description"]))
				}
			}
			fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Total: %d fixable issue(s) in %d file(s)", fixableIssues, len(grouped))))
		}
		return
	}

	fmt.Printf("\n%s\n", ui.Checkf("Fixed %d issue(s) in %d file(s).", intValue(data["fixed_issues"]), intValue(data["fixed_files"])))
}

func renderCanonicalCheckCreateMissing(vaultPath string, result commandexec.Result) error {
	data := canonicalDataMap(result)
	missingRefs := decodeMissingRefs(data["missing_ref_items"])
	undefinedTraits := decodeUndefinedTraits(data["undefined_trait_items"])
	if jsonOutput {
		outputJSON(result)
		return nil
	}

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}
	s, err := schema.Load(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	if len(missingRefs) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		created := handleMissingRefsInteractive(vaultPath, s, missingRefs, interaction, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot(), vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes)
		if created > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
		}
		added := 0
		if len(undefinedTraits) > 0 {
			added = handleUndefinedTraitsInteractive(vaultPath, s, undefinedTraits, interaction)
		}
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
		return nil
	}
	if len(undefinedTraits) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		added := handleUndefinedTraitsInteractive(vaultPath, s, undefinedTraits, interaction)
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
	}
	return nil
}

func decodeMissingRefs(raw interface{}) []*check.MissingRef {
	var refs []*check.MissingRef
	decodeCanonicalValue(raw, &refs)
	return refs
}

func decodeUndefinedTraits(raw interface{}) []*check.UndefinedTrait {
	var traits []*check.UndefinedTrait
	decodeCanonicalValue(raw, &traits)
	return traits
}

func decodeCanonicalValue(raw interface{}, target interface{}) bool {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(encoded, target) == nil
}

func printIssuesByFileFromJSON(issues []CheckIssueJSON) {
	issuesByFile := make(map[string][]CheckIssueJSON)
	var globalIssues []CheckIssueJSON

	for _, issue := range issues {
		if issue.FilePath == "" {
			globalIssues = append(globalIssues, issue)
		} else {
			issuesByFile[issue.FilePath] = append(issuesByFile[issue.FilePath], issue)
		}
	}

	for _, issue := range globalIssues {
		if issue.Level == "warning" {
			fmt.Println(ui.Warning(issue.Message))
		} else {
			fmt.Println(ui.Error(issue.Message))
		}
	}
	if len(globalIssues) > 0 {
		fmt.Println()
	}

	var filePaths []string
	for filePath := range issuesByFile {
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)

	for _, filePath := range filePaths {
		fileIssues := issuesByFile[filePath]
		var errCount, warnCount int
		for _, issue := range fileIssues {
			if issue.Level == "warning" {
				warnCount++
			} else {
				errCount++
			}
		}

		countBadge := ui.Muted.Render(ui.ErrorWarningCounts(errCount, warnCount))
		fmt.Printf("%s %s:\n", ui.FilePath(filePath), countBadge)
		sort.Slice(fileIssues, func(i, j int) bool {
			return fileIssues[i].Line < fileIssues[j].Line
		})
		for _, issue := range fileIssues {
			symbol := ui.SymbolError
			if issue.Level == "warning" {
				symbol = ui.SymbolWarning
			}
			lineNum := ui.Muted.Render(fmt.Sprintf("L%d", issue.Line))
			fmt.Printf("  %s %s %s\n", symbol, lineNum, issue.Message)
		}
		fmt.Println()
	}
}

func printIssuesVerboseFromJSON(issues []CheckIssueJSON) {
	for _, issue := range issues {
		symbol := ui.SymbolError
		if issue.Level == "warning" {
			symbol = ui.SymbolWarning
		}

		prefix := issue.FilePath
		if prefix == "" {
			prefix = "global"
		}
		if issue.Line > 0 {
			prefix = fmt.Sprintf("%s:%d", prefix, issue.Line)
		}
		fmt.Printf("%s %s %s\n", symbol, ui.FilePath(prefix), issue.Message)
		if issue.FixHint != "" {
			fmt.Printf("  %s\n", ui.Muted.Render(issue.FixHint))
		}
	}
}

func printIssueSummaryFromJSON(summary []CheckSummaryJSON, issues []CheckIssueJSON) {
	levels := make(map[string]string, len(issues))
	for _, issue := range issues {
		if _, exists := levels[issue.Type]; !exists {
			levels[issue.Type] = issue.Level
		}
	}

	var errorsSummary, warningsSummary []CheckSummaryJSON
	for _, item := range summary {
		if levels[item.IssueType] == "warning" || (levels[item.IssueType] == "" && looksLikeWarningIssue(item.IssueType)) {
			warningsSummary = append(warningsSummary, item)
		} else {
			errorsSummary = append(errorsSummary, item)
		}
	}

	if len(errorsSummary) > 0 {
		fmt.Printf("%s %s\n", ui.SymbolAttention, ui.Bold.Render("Errors"))
		for _, item := range errorsSummary {
			printIssueSummaryItem(item)
		}
	}
	if len(warningsSummary) > 0 {
		if len(errorsSummary) > 0 {
			fmt.Println()
		}
		fmt.Printf("%s %s\n", ui.SymbolAttention, ui.Bold.Render("Warnings"))
		for _, item := range warningsSummary {
			printIssueSummaryItem(item)
		}
	}
}

func printIssueSummaryItem(item CheckSummaryJSON) {
	issueLabel := ui.Bold.Render(item.IssueType)
	countStr := fmt.Sprintf("(%d)", item.Count)
	if len(item.TopValues) > 0 {
		examples := strings.Join(item.TopValues, ", ")
		if item.Count > len(item.TopValues) {
			examples += ", ..."
		}
		fmt.Printf("  %s %s  %s\n", issueLabel, countStr, ui.Muted.Render("("+examples+")"))
		return
	}
	fmt.Printf("  %s %s\n", issueLabel, countStr)
}

func looksLikeWarningIssue(issueType string) bool {
	switch issueType {
	case string(check.IssueStaleIndex), string(check.IssueUnusedType), string(check.IssueUnusedTrait), string(check.IssueShortRefCouldBeFullPath):
		return true
	default:
		return false
	}
}

func handleMissingRefsInteractive(vaultPath string, s *schema.Schema, refs []*check.MissingRef, interaction checkInteraction, objectsRoot, pagesRoot, templateDir string, protectedPrefixes []string) int {
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
				if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot, templateDir, protectedPrefixes); err != nil {
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
				if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType, objectsRoot, pagesRoot, templateDir, protectedPrefixes); err != nil {
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
				created += handleNewTypeCreationInteractive(vaultPath, s, ref, response, interaction, objectsRoot, pagesRoot, templateDir, protectedPrefixes)
				continue
			}

			resolvedPath := resolvePath(ref.TargetPath, response)
			if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, response, objectsRoot, pagesRoot, templateDir, protectedPrefixes); err != nil {
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
func handleNewTypeCreationInteractive(vaultPath string, s *schema.Schema, ref *check.MissingRef, typeName string, interaction checkInteraction, objectsRoot, pagesRoot, templateDir string, protectedPrefixes []string) int {
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
	if err := checksvc.CreateMissingPage(vaultPath, s, ref.TargetPath, typeName, objectsRoot, pagesRoot, templateDir, protectedPrefixes); err != nil {
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
