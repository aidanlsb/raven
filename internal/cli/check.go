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
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

type CheckIssueJSON = checksvc.CheckIssueJSON
type CheckSummaryJSON = checksvc.CheckSummaryJSON
type CheckScopeJSON = checksvc.CheckScopeJSON
type CheckResultJSON = checksvc.CheckResultJSON

// checkCmd is the validate-only parent. Repairs live in the `fix` and
// `create-missing` subcommands (see #91). All three are thin canonical leaves
// that delegate execution to commandimpl/checksvc; the CLI only builds args,
// prompts, and renders.
var checkCmd = newCanonicalLeafCommand("check", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	BuildArgs:    buildCheckValidateArgs,
	HandleError:  handleCheckLeafFailure,
	HandleResult: handleCheckValidateResult,
})

var checkFixCmd = newCanonicalLeafCommand("check_fix", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	BuildArgs:    buildCheckFixArgs,
	Invoke:       invokeCheckMutation,
	HandleError:  handleCheckLeafFailure,
	HandleResult: handleCheckFixResult,
})

var checkCreateMissingCmd = newCanonicalLeafCommand("check create-missing", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	BuildArgs:    buildCheckCreateMissingArgs,
	Invoke:       invokeCheckMutation,
	HandleError:  handleCheckLeafFailure,
	HandleResult: handleCheckCreateMissingResult,
})

// checkScopeArgs collects the canonical scope arguments shared by `check` and
// `check fix` from a command's own flag set, so no state is shared between the
// parent and its subcommands.
func checkScopeArgs(cmd *cobra.Command, args []string) map[string]interface{} {
	argsMap := map[string]interface{}{}
	if len(args) > 0 {
		argsMap["path"] = args[0]
	}
	if value, _ := cmd.Flags().GetString("type"); value != "" {
		argsMap["type"] = value
	}
	if value, _ := cmd.Flags().GetString("trait"); value != "" {
		argsMap["trait"] = value
	}
	if value, _ := cmd.Flags().GetString("issues"); value != "" {
		argsMap["issues"] = value
	}
	if value, _ := cmd.Flags().GetString("exclude"); value != "" {
		argsMap["exclude"] = value
	}
	if value, _ := cmd.Flags().GetBool("errors-only"); value {
		argsMap["errors-only"] = true
	}
	return argsMap
}

func buildCheckValidateArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	return checkScopeArgs(cmd, args), nil
}

func buildCheckFixArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	return checkScopeArgs(cmd, args), nil
}

func buildCheckCreateMissingArgs(_ *cobra.Command, _ []string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// invokeCheckMutation drives the mutating check subcommands. `check fix` honors
// --confirm directly. `check create-missing` in interactive (non-JSON) mode
// always runs a preview and then applies via the prompt flow, so --confirm is
// only meaningful for the non-interactive JSON path.
func invokeCheckMutation(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm, _ := cmd.Flags().GetBool("confirm")
	if commandID == "check create-missing" && !isJSONOutput() {
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

func handleCheckValidateResult(cmd *cobra.Command, result commandexec.Result) error {
	strict, _ := cmd.Flags().GetBool("strict")
	if jsonOutput {
		if err := outputJSON(result); err != nil {
			return err
		}
		if checkShouldExit(result, strict) {
			os.Exit(1)
		}
		return nil
	}

	byFile, _ := cmd.Flags().GetBool("by-file")
	verbose, _ := cmd.Flags().GetBool("verbose")
	printCheckScopeHeader(getVaultPath(), checkScopeFromResult(result))
	renderCanonicalCheckValidate(result, byFile, verbose)

	if checkShouldExit(result, strict) {
		os.Exit(1)
	}

	return nil
}

func handleCheckFixResult(cmd *cobra.Command, result commandexec.Result) error {
	strict, _ := cmd.Flags().GetBool("strict")
	if jsonOutput {
		if err := outputJSON(result); err != nil {
			return err
		}
		if checkShouldExit(result, strict) {
			os.Exit(1)
		}
		return nil
	}
	printCheckScopeHeader(getVaultPath(), checkScopeFromResult(result))
	renderCanonicalCheckFix(result)
	if checkShouldExit(result, strict) {
		os.Exit(1)
	}
	return nil
}

func handleCheckCreateMissingResult(cmd *cobra.Command, result commandexec.Result) error {
	strict, _ := cmd.Flags().GetBool("strict")
	if jsonOutput {
		if err := outputJSON(result); err != nil {
			return err
		}
		if checkShouldExit(result, strict) {
			os.Exit(1)
		}
		return nil
	}
	printCheckScopeHeader(getVaultPath(), checkScopeFromResult(result))
	if err := renderCanonicalCheckCreateMissing(getVaultPath(), result); err != nil {
		return err
	}
	if checkShouldExit(result, strict) {
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

func checkShouldExit(result commandexec.Result, strict bool) bool {
	data := canonicalDataMap(result)
	errorCount := intValue(data["error_count"])
	warningCount := intValue(data["warning_count"])
	if errorCount == 0 && warningCount == 0 {
		if decoded, ok := decodeCanonicalCheckJSON(result); ok {
			errorCount = decoded.ErrorCount
			warningCount = decoded.WarnCount
		}
	}
	return errorCount > 0 || (strict && warningCount > 0)
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

func renderCanonicalCheckValidate(result commandexec.Result, byFile, verbose bool) {
	decoded, ok := decodeCanonicalCheckJSON(result)
	if !ok {
		fmt.Println(ui.Warning("failed to decode check results"))
		return
	}

	if byFile {
		printIssuesByFileFromJSON(decoded.Issues)
		fmt.Println()
		if decoded.ErrorCount == 0 && decoded.WarnCount == 0 {
			fmt.Println(ui.Starf("No issues found in %d files.", decoded.FileCount))
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", decoded.ErrorCount, decoded.WarnCount, decoded.FileCount)
		}
		return
	}

	if verbose {
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
	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warning(warning.Message))
	}
}

// renderCanonicalCheckCreateMissing runs the interactive create-missing flow in
// non-JSON mode. Prompts stay here in the CLI; all schema/file mutations are
// delegated to checksvc appliers via collectMissingRefDecisions +
// checksvc.ApplyMissingRefResolutions (and the trait equivalents).
func renderCanonicalCheckCreateMissing(vaultPath string, result commandexec.Result) error {
	data := canonicalDataMap(result)
	missingRefs := decodeMissingRefs(data["missing_ref_items"])
	undefinedTraits := decodeUndefinedTraits(data["undefined_trait_items"])
	if jsonOutput {
		return outputJSON(result)
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
		created := runMissingRefsInteractive(vaultPath, s, missingRefs, interaction, vaultCfg)
		if created > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
		}
		added := 0
		if len(undefinedTraits) > 0 {
			added = runUndefinedTraitsInteractive(vaultPath, s, undefinedTraits, interaction)
		}
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
		return nil
	}
	if len(undefinedTraits) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		added := runUndefinedTraitsInteractive(vaultPath, s, undefinedTraits, interaction)
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
	}
	return nil
}

// promptCreateMissingRefsFromResult inspects a successful write result for
// missing reference targets and, in interactive (non-JSON) mode, offers to
// create the missing pages. Writes remain permissive: the object was already
// created/modified; this is purely additive UX layered on top of the completed
// write, reusing the same interactive flow as `rvn check create-missing`.
func promptCreateMissingRefsFromResult(vaultPath string, result commandexec.Result) {
	if jsonOutput || !canUseInteractiveTerminal() {
		return
	}
	data := canonicalDataMap(result)
	missingRefs := decodeMissingRefs(data["missing_ref_items"])
	if len(missingRefs) == 0 {
		return
	}

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return
	}
	s, err := schema.Load(vaultPath)
	if err != nil {
		return
	}

	interaction := newCheckInteraction(os.Stdin, os.Stdout)
	created := runMissingRefsInteractive(vaultPath, s, missingRefs, interaction, vaultCfg)
	if created > 0 {
		fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
	}
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
	case string(check.IssueStaleIndex), string(check.IssueCheckIncomplete), string(check.IssueUnusedType), string(check.IssueUnusedTrait), string(check.IssueShortRefCouldBeFullPath):
		return true
	default:
		return false
	}
}

// runMissingRefsInteractive prompts for missing-reference creation decisions and
// delegates the resulting schema/file mutations to checksvc. It returns the
// number of pages created.
func runMissingRefsInteractive(vaultPath string, s *schema.Schema, refs []*check.MissingRef, interaction checkInteraction, vaultCfg *config.VaultConfig) int {
	newTypes, resolutions := collectMissingRefDecisions(s, refs, interaction, vaultCfg)
	if len(newTypes) == 0 && len(resolutions) == 0 {
		return 0
	}
	applied := checksvc.ApplyMissingRefResolutions(vaultPath, s, newTypes, resolutions, vaultCfg)
	return renderMissingRefOutcomes(interaction, applied)
}

// collectMissingRefDecisions runs the interactive missing-reference prompts and
// returns the user's resolved decisions. It performs no mutations: new types
// the user asks to create are returned as NewTypeResolutions, and pages to
// create are returned as MissingRefResolutions.
func collectMissingRefDecisions(s *schema.Schema, refs []*check.MissingRef, interaction checkInteraction, vaultCfg *config.VaultConfig) ([]checksvc.NewTypeResolution, []checksvc.MissingRefResolution) {
	groups := checksvc.GroupMissingRefsForInteractive(refs)

	interaction.Printf("\n%s\n", ui.SectionHeader("Missing References"))

	objectsRoot := vaultCfg.GetObjectsRoot()
	pagesRoot := vaultCfg.GetPagesRoot()
	dailyDir := vaultCfg.GetDailyDirectory()
	resolvePath := func(targetPath, typeName string) string {
		return checksvc.ResolveAndSlugifyTargetPath(targetPath, typeName, s, objectsRoot, pagesRoot, dailyDir)
	}

	var newTypes []checksvc.NewTypeResolution
	var resolutions []checksvc.MissingRefResolution
	// pendingTypes lets a second unknown ref reuse a type the user already
	// chose to create in this session (mirrors the old in-place schema update).
	pendingTypes := map[string]struct{}{}

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
				resolutions = append(resolutions, checksvc.MissingRefResolution{TargetPath: ref.TargetPath, TypeName: ref.InferredType})
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
				resolutions = append(resolutions, checksvc.MissingRefResolution{TargetPath: ref.TargetPath, TypeName: ref.InferredType})
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

			// Offer to create the type when it is neither defined, built-in,
			// nor already queued for creation in this session.
			_, definedInSchema := s.Types[response]
			_, queued := pendingTypes[response]
			if !definedInSchema && !queued && !schema.IsBuiltinType(response) {
				create, defaultPath := promptNewTypeCreation(response, ref, interaction)
				if !create {
					continue
				}
				newTypes = append(newTypes, checksvc.NewTypeResolution{TypeName: response, DefaultPath: defaultPath})
				pendingTypes[response] = struct{}{}
			}

			resolutions = append(resolutions, checksvc.MissingRefResolution{TargetPath: ref.TargetPath, TypeName: response})
		}
	}

	return newTypes, resolutions
}

// renderMissingRefOutcomes prints the results of applying missing-ref decisions
// and returns the number of pages created.
func renderMissingRefOutcomes(interaction checkInteraction, applied checksvc.MissingRefApplyResult) int {
	for _, typeOutcome := range applied.Types {
		if typeOutcome.Err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to create type '%s': %v", typeOutcome.TypeName, typeOutcome.Err))
			continue
		}
		interaction.Printf("  %s\n", ui.Checkf("Created type '%s' in schema.yaml", typeOutcome.TypeName))
		if typeOutcome.DefaultPath != "" {
			interaction.Printf("    %s\n", ui.Muted.Render("default_path: "+typeOutcome.DefaultPath))
		}
	}

	created := 0
	for _, page := range applied.Pages {
		if page.Err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", page.ResolvedPath, page.Err))
			continue
		}
		interaction.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", page.ResolvedPath, page.TypeName))
		created++
	}
	return created
}

// runUndefinedTraitsInteractive prompts for undefined-trait decisions and
// delegates the schema mutations to checksvc. It returns the number of traits
// added.
func runUndefinedTraitsInteractive(vaultPath string, s *schema.Schema, traits []*check.UndefinedTrait, interaction checkInteraction) int {
	resolutions := collectTraitDecisions(traits, interaction)
	if len(resolutions) == 0 {
		return 0
	}
	outcomes := checksvc.ApplyTraitResolutions(vaultPath, s, resolutions)
	return renderTraitOutcomes(interaction, outcomes)
}

// collectTraitDecisions prompts the user about undefined traits and returns the
// resolved decisions. It performs no mutations.
func collectTraitDecisions(traits []*check.UndefinedTrait, interaction checkInteraction) []checksvc.TraitResolution {
	if len(traits) == 0 {
		return nil
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

	interaction.Println("\nWould you like to add these traits to the schema?")

	var resolutions []checksvc.TraitResolution
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

		resolutions = append(resolutions, checksvc.TraitResolution{
			TraitName:    trait.TraitName,
			TraitType:    traitType,
			EnumValues:   enumValues,
			DefaultValue: defaultValue,
		})
	}

	return resolutions
}

// renderTraitOutcomes prints the results of applying trait decisions and returns
// the number of traits added.
func renderTraitOutcomes(interaction checkInteraction, outcomes []checksvc.TraitOutcome) int {
	added := 0
	for _, outcome := range outcomes {
		if outcome.Err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to add @%s: %v", outcome.TraitName, outcome.Err))
			continue
		}
		interaction.Printf("  %s\n", ui.Checkf("Added trait '@%s' (type: %s) to schema.yaml", outcome.TraitName, outcome.TraitType))
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

// promptNewTypeCreation asks the user whether to create a type that does not yet
// exist and, if so, for its default path. It performs no mutations; the caller
// records the decision and checksvc applies it. Returns create=false when the
// user declines (the referencing page is then skipped).
func promptNewTypeCreation(typeName string, ref *check.MissingRef, interaction checkInteraction) (create bool, defaultPath string) {
	interaction.Printf("\n  Type %s doesn't exist. Would you like to create it? %s ",
		ui.Bold.Render("'"+typeName+"'"),
		ui.Muted.Render("[y/N]"))
	response := readTrimmedLowerLine(interaction)

	if response != "y" && response != "yes" {
		interaction.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
		return false, ""
	}

	// Prompt for default_path (optional)
	interaction.Printf("  Default path for '%s' files %s: ", typeName, ui.Muted.Render(fmt.Sprintf("(e.g., '%s/', or leave empty)", typeName+"s")))
	defaultPath = readTrimmedLine(interaction)
	return true, defaultPath
}

// pluralize returns "es" for counts != 1
func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

func init() {
	checkCmd.AddCommand(checkFixCmd)
	checkCmd.AddCommand(checkCreateMissingCmd)
	rootCmd.AddCommand(checkCmd)
}
