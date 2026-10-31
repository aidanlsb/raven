package readsvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
)

// RefreshWarning is a non-fatal issue observed while refreshing the index
// before an index-backed read.
type RefreshWarning struct {
	Code    codes.WarningCode
	Message string
	Ref     string
}

// RefreshWarnings converts a SmartReindex report into stable warning codes
// shared by query and other index-reading commands.
func RefreshWarnings(report SmartReindexReport) []RefreshWarning {
	warnings := refreshFailureWarnings(report)
	warnings = append(warnings, refreshUnknownFieldWarnings(report)...)
	return append(warnings, refreshReferenceResolutionWarnings(report)...)
}

func refreshFailureWarnings(report SmartReindexReport) []RefreshWarning {
	if len(report.Failures) == 0 {
		return nil
	}
	const maxListed = 5
	listed := make([]string, 0, maxListed)
	for i, failure := range report.Failures {
		if i >= maxListed {
			break
		}
		listed = append(listed, fmt.Sprintf("%s (%s: %s)", failure.Path, failure.Stage, failure.ErrMsg))
	}
	message := fmt.Sprintf("refresh skipped %d file(s); index may be incomplete", len(report.Failures))
	if len(listed) > 0 {
		message += ": " + strings.Join(listed, "; ")
	}
	if len(report.Failures) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.Failures)-maxListed)
	}
	return []RefreshWarning{{
		Code:    codes.WarnIndexUpdateFailed,
		Message: message,
		Ref:     "Run 'rvn reindex' or 'rvn check' to inspect failed files",
	}}
}

func refreshUnknownFieldWarnings(report SmartReindexReport) []RefreshWarning {
	if len(report.Warnings) == 0 {
		return nil
	}
	const maxListed = 5
	listed := make([]string, 0, maxListed)
	for i, warning := range report.Warnings {
		if i >= maxListed {
			break
		}
		listed = append(listed, warning.Message)
	}
	message := fmt.Sprintf("refresh found %d unknown frontmatter key(s)", len(report.Warnings))
	if len(listed) > 0 {
		message += ": " + strings.Join(listed, "; ")
	}
	if len(report.Warnings) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.Warnings)-maxListed)
	}
	return []RefreshWarning{{
		Code:    codes.WarnUnknownField,
		Message: message,
		Ref:     "Add the field to schema.yaml or remove it; run 'rvn check' for details",
	}}
}

func refreshReferenceResolutionWarnings(report SmartReindexReport) []RefreshWarning {
	if len(report.ReferenceResolutionWarnings) == 0 {
		return nil
	}
	const maxListed = 5
	listed := make([]string, 0, maxListed)
	for i, warning := range report.ReferenceResolutionWarnings {
		if i >= maxListed {
			break
		}
		listed = append(listed, warning.Message)
	}
	message := fmt.Sprintf(
		"refresh indexed %d file(s), but reference resolution did not complete",
		len(report.ReferenceResolutionWarnings),
	)
	if len(listed) > 0 {
		message += ": " + strings.Join(listed, "; ")
	}
	if len(report.ReferenceResolutionWarnings) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.ReferenceResolutionWarnings)-maxListed)
	}
	return []RefreshWarning{{
		Code:    codes.WarnRefResolutionIncomplete,
		Message: message,
		Ref:     "The files were indexed successfully, but backlinks may be stale. Run 'rvn reindex' to retry reference resolution.",
	}}
}
