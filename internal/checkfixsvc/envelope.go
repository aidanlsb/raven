package checkfixsvc

import (
	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
)

// CheckScope is the stable scope object shared by check mutation payloads.
type CheckScope struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// FixPreviewResult is the success payload for `check fix` before confirm.
type FixPreviewResult struct {
	Preview       bool        `json:"preview"`
	FixableIssues int         `json:"fixable_issues"`
	Files         []FileFixes `json:"files"`
	Scope         CheckScope  `json:"scope"`
	FileCount     int         `json:"file_count"`
	ErrorCount    int         `json:"error_count"`
	WarningCount  int         `json:"warning_count"`
}

// FixAppliedResult is the success payload for an applied `check fix`.
type FixAppliedResult struct {
	Preview       bool         `json:"preview"`
	OK            bool         `json:"ok"`
	FixableIssues int          `json:"fixable_issues"`
	FixedIssues   int          `json:"fixed_issues"`
	FixedFiles    int          `json:"fixed_files"`
	SkippedIssues int          `json:"skipped_issues"`
	SkippedItems  []SkippedFix `json:"skipped_items"`
	Scope         CheckScope   `json:"scope"`
	FileCount     int          `json:"file_count"`
	ErrorCount    int          `json:"error_count"`
	WarningCount  int          `json:"warning_count"`
	check.MissingReferences
}

// CreateMissingResult is the success payload for `check create-missing`.
type CreateMissingResult struct {
	Preview             bool                    `json:"preview"`
	MissingRefs         int                     `json:"missing_refs"`
	UndefinedTraits     int                     `json:"undefined_traits"`
	RequiresConfirm     bool                    `json:"requires_confirm"`
	NonInteractiveOnly  bool                    `json:"non_interactive_only"`
	Scope               CheckScope              `json:"scope"`
	MissingRefItems     []*check.MissingRef     `json:"missing_ref_items"`
	UndefinedTraitItems []*check.UndefinedTrait `json:"undefined_trait_items"`
	FileCount           int                     `json:"file_count"`
	ErrorCount          int                     `json:"error_count"`
	WarningCount        int                     `json:"warning_count"`
	OK                  *bool                   `json:"ok,omitempty"`
	CreatedPages        *int                    `json:"created_pages,omitempty"`
	FailedPages         *int                    `json:"failed_pages,omitempty"`
	FailedPageItems     *[]CreateMissingFailure `json:"failed_page_items,omitempty"`
	UndefinedTraitsNote string                  `json:"undefined_traits_note,omitempty"`
}

// ResultScope returns the scope of any typed check payload.
func ResultScope(data any) (CheckScope, bool) {
	switch payload := data.(type) {
	case FixPreviewResult:
		return payload.Scope, true
	case FixAppliedResult:
		return payload.Scope, true
	case CreateMissingResult:
		return payload.Scope, true
	case checksvc.CheckResultJSON:
		if payload.Scope == nil {
			return CheckScope{Type: "full"}, true
		}
		return CheckScope{Type: payload.Scope.Type, Value: payload.Scope.Value}, true
	default:
		return CheckScope{}, false
	}
}

func ScopeOf(result *checksvc.RunResult) CheckScope {
	if result == nil {
		return CheckScope{}
	}
	return CheckScope{Type: result.Scope.Type, Value: result.Scope.Value}
}

func PreviewFixes(result *checksvc.RunResult, fixes []FixableIssue) FixPreviewResult {
	return FixPreviewResult{
		Preview:       true,
		FixableIssues: len(fixes),
		Files:         GroupFixesByFile(fixes),
		Scope:         ScopeOf(result),
		FileCount:     result.FileCount,
		ErrorCount:    result.ErrorCount,
		WarningCount:  result.WarningCount,
	}
}

func AppliedFixes(result *checksvc.RunResult, fixes []FixableIssue, applied FixResult, refs check.MissingReferences) FixAppliedResult {
	return FixAppliedResult{
		Preview:           false,
		OK:                len(applied.Skipped) == 0,
		FixableIssues:     len(fixes),
		FixedIssues:       applied.IssueCount,
		FixedFiles:        applied.FileCount,
		SkippedIssues:     len(applied.Skipped),
		SkippedItems:      applied.Skipped,
		Scope:             ScopeOf(result),
		FileCount:         result.FileCount,
		ErrorCount:        result.ErrorCount,
		WarningCount:      result.WarningCount,
		MissingReferences: refs,
	}
}

func PreviewCreateMissing(result *checksvc.RunResult) CreateMissingResult {
	return CreateMissingResult{
		Preview:             true,
		MissingRefs:         len(result.MissingRefs),
		UndefinedTraits:     len(result.UndefinedTraits),
		RequiresConfirm:     true,
		NonInteractiveOnly:  true,
		Scope:               ScopeOf(result),
		MissingRefItems:     result.MissingRefs,
		UndefinedTraitItems: result.UndefinedTraits,
		FileCount:           result.FileCount,
		ErrorCount:          result.ErrorCount,
		WarningCount:        result.WarningCount,
	}
}

func AppliedCreateMissing(result *checksvc.RunResult, created CreateMissingApplyResult) CreateMissingResult {
	ok := len(created.Failures) == 0
	failedPages := len(created.Failures)
	data := PreviewCreateMissing(result)
	data.Preview = false
	data.OK = &ok
	data.CreatedPages = &created.Created
	data.FailedPages = &failedPages
	data.FailedPageItems = &created.Failures
	data.UndefinedTraitsNote = "undefined traits are interactive-only and were not changed in JSON mode"
	return data
}
