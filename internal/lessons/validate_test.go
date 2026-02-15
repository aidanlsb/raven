package lessons

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestValidateDefaults(t *testing.T) {
	report := ValidateDefaults()
	if !report.Valid {
		t.Fatalf("expected embedded defaults to validate, got: %+v", report)
	}
	if report.ErrorCount != 0 {
		t.Fatalf("expected no validation errors, got %d", report.ErrorCount)
	}
}

func TestValidateFSOrphanLessonWarning(t *testing.T) {
	fsys := fstest.MapFS{
		"defaults/syllabus.yaml": {
			Data: []byte(`sections:
  - id: foundations
    title: Foundations
    lessons:
      - objects
`),
		},
		"defaults/lessons/objects.md": {
			Data: []byte(`---
title: Objects
---

# Objects
`),
		},
		"defaults/lessons/orphan.md": {
			Data: []byte(`---
title: Orphan
---

# Orphan
`),
		},
	}

	report := validateFS(fsys)
	if !report.Valid {
		t.Fatalf("expected report to be valid with warning-only findings, got: %+v", report)
	}
	if report.WarningCount != 1 {
		t.Fatalf("expected 1 warning, got %d", report.WarningCount)
	}
	if report.Issues[0].Code != "LESSON_NOT_IN_SYLLABUS" {
		t.Fatalf("expected orphan lesson warning, got %+v", report.Issues[0])
	}
}

func TestValidateFSInvalidCatalog(t *testing.T) {
	fsys := fstest.MapFS{
		"defaults/syllabus.yaml": {
			Data: []byte(`sections:
  - id: foundations
    title: Foundations
    lessons:
      - objects
      - refs
`),
		},
		"defaults/lessons/objects.md": {
			Data: []byte(`---
title: Objects
prereqs:
  - refs
---

# Objects
`),
		},
		"defaults/lessons/refs.md": {
			Data: []byte(`---
title: Refs
prereqs:
  - objects
---

# Refs
`),
		},
	}

	report := validateFS(fsys)
	if report.Valid {
		t.Fatalf("expected invalid report due to prereq cycle")
	}
	if report.ErrorCount == 0 {
		t.Fatalf("expected at least one validation error")
	}
	if report.Issues[0].Code != "CATALOG_INVALID" {
		t.Fatalf("expected catalog invalid issue, got %+v", report.Issues[0])
	}
	if !strings.Contains(report.Issues[0].Message, "cycle") {
		t.Fatalf("expected cycle-related message, got %q", report.Issues[0].Message)
	}
}
