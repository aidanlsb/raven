package commandpayload

import (
	"sort"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
)

// CheckIssueJSON is one issue in the `check` validate payload.
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

// CheckSummaryJSON is one aggregated row in the `check` validate summary.
type CheckSummaryJSON struct {
	IssueType    string   `json:"issue_type"`
	Count        int      `json:"count"`
	UniqueValues int      `json:"unique_values,omitempty"`
	FixCommand   string   `json:"fix_command,omitempty"`
	FixHint      string   `json:"fix_hint,omitempty"`
	TopValues    []string `json:"top_values,omitempty"`
}

// CheckScopeJSON is the optional scope object on a `check` validate payload.
// Value is omitted when empty; mutation payloads use CheckScope instead, which
// always emits value.
type CheckScopeJSON struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

// CheckResultJSON is the success payload for read-only `check`.
type CheckResultJSON struct {
	VaultPath  string             `json:"vault_path"`
	Scope      *CheckScopeJSON    `json:"scope,omitempty"`
	FileCount  int                `json:"file_count"`
	ErrorCount int                `json:"error_count"`
	WarnCount  int                `json:"warning_count"`
	Issues     []CheckIssueJSON   `json:"issues"`
	Summary    []CheckSummaryJSON `json:"summary"`
}

// BuildJSON maps a check service result onto the stable validate payload.
func BuildJSON(vaultPath string, result *checksvc.RunResult) CheckResultJSON {
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
		jsonResult.Issues = append(jsonResult.Issues, checkIssueJSON(issue))
	}
	for _, issue := range result.SchemaIssues {
		jsonResult.Issues = append(jsonResult.Issues, schemaIssueJSON(issue))
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

func checkIssueJSON(issue check.Issue) CheckIssueJSON {
	return CheckIssueJSON{
		Type:       string(issue.Type),
		Level:      issue.Level.String(),
		FilePath:   issue.FilePath,
		Line:       issue.Line,
		Message:    issue.Message,
		Value:      issue.Value,
		FixCommand: issue.FixCommand,
		FixHint:    issue.FixHint,
	}
}

func schemaIssueJSON(issue check.SchemaIssue) CheckIssueJSON {
	return CheckIssueJSON{
		Type:       string(issue.Type),
		Level:      issue.Level.String(),
		FilePath:   "schema.yaml",
		Line:       0,
		Message:    issue.Message,
		Value:      issue.Value,
		FixCommand: issue.FixCommand,
		FixHint:    issue.FixHint,
	}
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
