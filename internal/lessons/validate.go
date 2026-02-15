package lessons

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationSeverity classifies validation issue severity.
type ValidationSeverity string

const (
	ValidationSeverityError   ValidationSeverity = "error"
	ValidationSeverityWarning ValidationSeverity = "warning"
)

// ValidationIssue is a single catalog validation finding.
type ValidationIssue struct {
	Code      string             `json:"code"`
	Severity  ValidationSeverity `json:"severity"`
	Message   string             `json:"message"`
	SectionID string             `json:"section_id,omitempty"`
	LessonID  string             `json:"lesson_id,omitempty"`
}

// ValidationReport summarizes lessons catalog validation findings.
type ValidationReport struct {
	Valid        bool              `json:"valid"`
	SectionCount int               `json:"section_count"`
	LessonCount  int               `json:"lesson_count"`
	ErrorCount   int               `json:"error_count"`
	WarningCount int               `json:"warning_count"`
	IssueCount   int               `json:"issue_count"`
	Issues       []ValidationIssue `json:"issues,omitempty"`
}

// ValidateDefaults validates the embedded lessons catalog.
func ValidateDefaults() ValidationReport {
	return validateFS(defaultLessonsFS)
}

func validateFS(fsys fs.FS) ValidationReport {
	report := ValidationReport{
		Issues: make([]ValidationIssue, 0),
	}

	catalog, err := loadCatalogFromFS(fsys)
	if err != nil {
		report.addIssue(ValidationIssue{
			Code:     "CATALOG_INVALID",
			Severity: ValidationSeverityError,
			Message:  err.Error(),
		})
	} else if catalog != nil {
		report.SectionCount = len(catalog.Sections)
		report.LessonCount = len(catalog.Lessons)
	}

	orphans, err := findOrphanLessonIDs(fsys)
	if err != nil {
		report.addIssue(ValidationIssue{
			Code:     "ORPHAN_CHECK_FAILED",
			Severity: ValidationSeverityError,
			Message:  fmt.Sprintf("failed to check orphan lessons: %v", err),
		})
	} else {
		for _, lessonID := range orphans {
			report.addIssue(ValidationIssue{
				Code:     "LESSON_NOT_IN_SYLLABUS",
				Severity: ValidationSeverityWarning,
				Message:  fmt.Sprintf("lesson file %q exists but is not listed in syllabus", lessonID),
				LessonID: lessonID,
			})
		}
	}

	report.Valid = report.ErrorCount == 0
	report.IssueCount = len(report.Issues)
	return report
}

func (r *ValidationReport) addIssue(issue ValidationIssue) {
	r.Issues = append(r.Issues, issue)
	switch issue.Severity {
	case ValidationSeverityWarning:
		r.WarningCount++
	default:
		r.ErrorCount++
	}
}

func findOrphanLessonIDs(fsys fs.FS) ([]string, error) {
	referencedLessonIDs, err := syllabusLessonIDs(fsys)
	if err != nil {
		return nil, err
	}

	entries, err := fs.ReadDir(fsys, defaultLessonsDir)
	if err != nil {
		return nil, fmt.Errorf("read lessons directory: %w", err)
	}

	var orphans []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".md" {
			continue
		}

		lessonID := strings.TrimSuffix(name, ".md")
		if _, ok := referencedLessonIDs[lessonID]; ok {
			continue
		}
		orphans = append(orphans, lessonID)
	}

	sort.Strings(orphans)
	return orphans, nil
}

func syllabusLessonIDs(fsys fs.FS) (map[string]struct{}, error) {
	content, err := fs.ReadFile(fsys, defaultSyllabusPath)
	if err != nil {
		return nil, fmt.Errorf("read syllabus: %w", err)
	}

	var syllabus syllabusFile
	if err := yaml.Unmarshal(content, &syllabus); err != nil {
		return nil, fmt.Errorf("parse syllabus: %w", err)
	}

	ids := make(map[string]struct{})
	for _, section := range syllabus.Sections {
		for _, rawID := range section.Lessons {
			id := strings.TrimSpace(rawID)
			if id == "" {
				continue
			}
			ids[id] = struct{}{}
		}
	}

	return ids, nil
}
