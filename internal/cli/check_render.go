package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checkfixsvc"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

type CheckIssueJSON = checksvc.CheckIssueJSON
type CheckSummaryJSON = checksvc.CheckSummaryJSON
type CheckScopeJSON = checksvc.CheckScopeJSON
type CheckResultJSON = checksvc.CheckResultJSON

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
	if scope, ok := commandpayload.CheckResultScope(result.Data); ok {
		return scope
	}
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
		printCheckIssueTotals(decoded)
		return
	}
	if verbose {
		printIssuesVerboseFromJSON(decoded.Issues)
		fmt.Println()
		printCheckIssueTotals(decoded)
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

func printCheckIssueTotals(result CheckResultJSON) {
	fmt.Println()
	if result.ErrorCount == 0 && result.WarnCount == 0 {
		fmt.Println(ui.Starf("No issues found in %d files.", result.FileCount))
		return
	}
	fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", result.ErrorCount, result.WarnCount, result.FileCount)
}

func renderCanonicalCheckFix(result commandexec.Result) {
	switch data := result.Data.(type) {
	case commandpayload.CheckFixPreviewResult:
		if data.FixableIssues == 0 {
			fmt.Println(ui.Hint("\nNo auto-fixable issues found."))
			return
		}
		fmt.Printf("\n%s\n", ui.SectionHeader("Auto-fixable Issues"))
		fmt.Println(ui.Hint("Use --confirm to apply these fixes."))
		fmt.Println()
		printCheckFileFixes(data.Files, data.FixableIssues)
	case commandpayload.CheckFixResult:
		if data.FixableIssues == 0 {
			fmt.Println(ui.Hint("\nNo auto-fixable issues found."))
			return
		}
		fmt.Printf("\n%s\n", ui.Checkf("Fixed %d issue(s) in %d file(s).", data.FixedIssues, data.FixedFiles))
		for _, warning := range result.Warnings {
			fmt.Printf("  %s\n", ui.Warning(warning.Message))
		}
	default:
		fmt.Println(ui.Hint("\nNo auto-fixable issues found."))
	}
}

func printCheckFileFixes(grouped []checkfixsvc.FileFixes, total int) {
	for _, file := range grouped {
		fmt.Printf("%s %s\n", ui.FilePath(file.FilePath), ui.Muted.Render(fmt.Sprintf("(%d fix%s)", len(file.Fixes), pluralize(len(file.Fixes)))))
		for _, fix := range file.Fixes {
			fmt.Printf("  %s %s\n", ui.Muted.Render(fmt.Sprintf("L%d", fix.Line)), fix.Description)
		}
	}
	fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Total: %d fixable issue(s) in %d file(s)", total, len(grouped))))
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

	filePaths := make([]string, 0, len(issuesByFile))
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

// pluralize returns "es" for counts != 1.
func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}
