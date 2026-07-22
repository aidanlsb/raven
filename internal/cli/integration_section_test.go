//go:build integration

package cli_test

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

const sectionLifecycleFixture = `---
type: project
title: Site
status: active
---

# Project

## Alpha

Alpha body

### Alpha Child

Child body

## Beta

Beta body
`

func TestIntegration_SectionCreatePlacements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantOrder []string
	}{
		{
			name:      "after full subtree",
			args:      []string{"section", "create", "projects/site", "Inserted", "--level", "2", "--after", "projects/site#alpha"},
			wantOrder: []string{"### Alpha Child", "## Inserted", "## Beta"},
		},
		{
			name:      "before heading",
			args:      []string{"section", "create", "projects/site", "Inserted", "--level", "2", "--before", "projects/site#beta"},
			wantOrder: []string{"### Alpha Child", "## Inserted", "## Beta"},
		},
		{
			name:      "under as last child",
			args:      []string{"section", "create", "projects/site", "Inserted", "--level", "3", "--under", "projects/site#alpha"},
			wantOrder: []string{"### Alpha Child", "### Inserted", "## Beta"},
		},
		{
			name:      "end of file",
			args:      []string{"section", "create", "projects/site", "Inserted", "--level", "2"},
			wantOrder: []string{"## Beta", "## Inserted"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := newSectionLifecycleVault(t, sectionLifecycleFixture)

			result := v.RunCLI(tt.args...)
			result.MustSucceed(t)
			if got := result.DataString("section"); got != "projects/site#inserted" {
				t.Fatalf("section = %q, want projects/site#inserted", got)
			}
			assertIntegrationHeadingOrder(t, v.ReadFile("projects/site.md"), tt.wantOrder...)
		})
	}
}

func TestIntegration_SectionCreateDryRunAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()
		v := newSectionLifecycleVault(t, sectionLifecycleFixture)

		result := v.RunCLI(
			"section", "create", "projects/site", "Preview",
			"--level", "2",
			"--after", "projects/site#alpha",
			"--dry-run",
		)
		result.MustSucceed(t)
		if result.DataString("section") != "projects/site#preview" {
			t.Fatalf("unexpected section ID: %#v", result.Data)
		}
		if got := v.ReadFile("projects/site.md"); got != sectionLifecycleFixture {
			t.Fatalf("dry run changed file:\n%s", got)
		}
	})

	tests := []struct {
		name    string
		args    []string
		code    string
		message string
	}{
		{
			name:    "illegal child level",
			args:    []string{"section", "create", "projects/site", "Too Deep", "--level", "4", "--under", "projects/site#alpha"},
			code:    "INVALID_INPUT",
			message: "expected level 3",
		},
		{
			name:    "slug collision",
			args:    []string{"section", "create", "projects/site", "Beta", "--level", "2", "--before", "projects/site#beta"},
			code:    "VALIDATION_FAILED",
			message: "shift slug",
		},
		{
			name:    "markdown title rejected",
			args:    []string{"section", "create", "projects/site", "## Wrong", "--level", "2"},
			code:    "INVALID_INPUT",
			message: "plain text",
		},
		{
			name:    "level is required",
			args:    []string{"section", "create", "projects/site", "Missing Level"},
			code:    "INVALID_INPUT",
			message: "--level is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := newSectionLifecycleVault(t, sectionLifecycleFixture)

			result := v.RunCLI(tt.args...)
			result.MustFail(t, tt.code)
			result.MustFailWithMessage(t, tt.message)
			if got := v.ReadFile("projects/site.md"); got != sectionLifecycleFixture {
				t.Fatalf("failed create changed file:\n%s", got)
			}
		})
	}
}

func TestIntegration_SectionMoveReordersSubtree(t *testing.T) {
	t.Parallel()
	v := newSectionLifecycleVault(t, sectionLifecycleFixture)

	result := v.RunCLI("section", "move", "projects/site#alpha", "--after", "projects/site#beta")
	result.MustSucceed(t)
	if got := result.DataString("section"); got != "projects/site#alpha" {
		t.Fatalf("section = %q, want projects/site#alpha", got)
	}

	content := v.ReadFile("projects/site.md")
	assertIntegrationHeadingOrder(t, content, "## Beta", "## Alpha", "### Alpha Child")
	if !strings.Contains(content, "### Alpha Child\n\nChild body") {
		t.Fatalf("child subtree was not preserved:\n%s", content)
	}
}

func TestIntegration_SectionMoveReparentsSubtree(t *testing.T) {
	t.Parallel()
	const fixture = `---
type: project
title: Site
status: active
---

# Project

## Alpha

## Beta

### Gamma

#### Gamma Child

Child body
`
	v := newSectionLifecycleVault(t, fixture)

	result := v.RunCLI("section", "move", "projects/site#gamma", "--under", "projects/site#alpha")
	result.MustSucceed(t)
	content := v.ReadFile("projects/site.md")
	assertIntegrationHeadingOrder(t, content, "## Alpha", "### Gamma", "#### Gamma Child", "## Beta")
	if !strings.Contains(content, "#### Gamma Child\n\nChild body") {
		t.Fatalf("child subtree was not preserved:\n%s", content)
	}
}

func TestIntegration_SectionMoveDryRunAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()
		v := newSectionLifecycleVault(t, sectionLifecycleFixture)

		result := v.RunCLI("section", "move", "projects/site#alpha", "--after", "projects/site#beta", "--dry-run")
		result.MustSucceed(t)
		if got := v.ReadFile("projects/site.md"); got != sectionLifecycleFixture {
			t.Fatalf("dry run changed file:\n%s", got)
		}
	})

	tests := []struct {
		name    string
		args    []string
		code    string
		message string
	}{
		{
			name:    "illegal reparent level",
			args:    []string{"section", "move", "projects/site#beta", "--under", "projects/site#alpha"},
			code:    "INVALID_INPUT",
			message: "expected level 3",
		},
		{
			name:    "own descendant",
			args:    []string{"section", "move", "projects/site#alpha", "--under", "projects/site#alpha-child"},
			code:    "INVALID_INPUT",
			message: "itself or its descendant",
		},
		{
			name:    "missing anchor",
			args:    []string{"section", "move", "projects/site#beta", "--after", "projects/site#missing"},
			code:    "REF_NOT_FOUND",
			message: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := newSectionLifecycleVault(t, sectionLifecycleFixture)

			result := v.RunCLI(tt.args...)
			result.MustFail(t, tt.code)
			result.MustFailWithMessage(t, tt.message)
			if got := v.ReadFile("projects/site.md"); got != sectionLifecycleFixture {
				t.Fatalf("failed move changed file:\n%s", got)
			}
		})
	}
}

func newSectionLifecycleVault(t *testing.T, content string) *testutil.TestVault {
	t.Helper()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", content).
		Build()
	v.RunCLI("reindex").MustSucceed(t)
	return v
}

func assertIntegrationHeadingOrder(t *testing.T, content string, headings ...string) {
	t.Helper()
	previous := -1
	for _, heading := range headings {
		index := strings.Index(content, heading)
		if index < 0 {
			t.Fatalf("missing heading %q:\n%s", heading, content)
		}
		if index <= previous {
			t.Fatalf("heading %q is out of order:\n%s", heading, content)
		}
		previous = index
	}
}
