package sectionsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

const lifecycleOutline = `---
type: project
title: Site
status: active
---

# Project

Intro

## Alpha

Alpha body

### Alpha Child

Child body

## Beta

Beta body
`

func TestCreatePlacements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		level     int
		placement Placement
		assert    func(*testing.T, string)
	}{
		{
			name:      "after full subtree",
			level:     2,
			placement: Placement{After: "projects/site#alpha"},
			assert: func(t *testing.T, content string) {
				t.Helper()
				assertHeadingOrder(t, content, "### Alpha Child", "## Inserted", "## Beta")
			},
		},
		{
			name:      "before heading",
			level:     2,
			placement: Placement{Before: "projects/site#beta"},
			assert: func(t *testing.T, content string) {
				t.Helper()
				assertHeadingOrder(t, content, "### Alpha Child", "## Inserted", "## Beta")
			},
		},
		{
			name:      "under as last child",
			level:     3,
			placement: Placement{Under: "projects/site#alpha"},
			assert: func(t *testing.T, content string) {
				t.Helper()
				assertHeadingOrder(t, content, "### Alpha Child", "### Inserted", "## Beta")
			},
		},
		{
			name:  "end of file",
			level: 2,
			assert: func(t *testing.T, content string) {
				t.Helper()
				assertHeadingOrder(t, content, "## Beta", "## Inserted")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("projects/site.md", lifecycleOutline).
				Build()
			rt := testRuntime(t, v.Path)
			indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

			result, err := Create(rt, CreateRequest{
				FileReference:  "projects/site",
				Title:          "Inserted",
				Level:          tt.level,
				Placement:      tt.placement,
				FailOnIndexErr: true,
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if result.SectionID != "projects/site#inserted" {
				t.Fatalf("SectionID = %q, want projects/site#inserted", result.SectionID)
			}
			tt.assert(t, v.ReadFile("projects/site.md"))
		})
	}
}

const flushLifecycleOutline = `---
type: project
title: Site
status: active
---

# Project
Intro
## Alpha
Alpha body
### Alpha Child
Child body
## Beta
Beta body
`

func TestShouldInsertBlankLineBeforeHeading(t *testing.T) {
	t.Parallel()

	bodyDoc := []trackedLine{
		{text: "## Alpha"},
		{text: "Alpha body"},
		{text: ""},
		{text: "   "},
		{text: "## Beta"},
		{text: "Beta body"},
	}

	tests := []struct {
		name        string
		lines       []trackedLine
		insertIndex int
		want        bool
	}{
		{name: "no previous line", lines: bodyDoc, insertIndex: 0, want: false},
		{name: "empty document", insertIndex: 0, want: false},
		{name: "previous body text", lines: bodyDoc, insertIndex: 2, want: true},
		{name: "eof after last-section body", lines: bodyDoc, insertIndex: 6, want: true},
		{name: "previous already blank", lines: bodyDoc, insertIndex: 3, want: false},
		{name: "previous whitespace only", lines: bodyDoc, insertIndex: 4, want: false},
		{name: "previous heading", lines: bodyDoc, insertIndex: 1, want: true},
		{name: "clamped past end uses last line", lines: bodyDoc, insertIndex: 99, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldInsertBlankLineBeforeHeading(tt.lines, tt.insertIndex); got != tt.want {
				t.Fatalf("shouldInsertBlankLineBeforeHeading(%d) = %v, want %v", tt.insertIndex, got, tt.want)
			}
		})
	}
}

func TestCreateInsertsBlankLineBeforeHeadingWhenPreviousIsBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		level     int
		placement Placement
		want      string
	}{
		{
			name:  "eof after last-section body",
			level: 2,
			want:  "Beta body\n\n## Inserted\n",
		},
		{
			name:      "after subtree ending in body",
			level:     2,
			placement: Placement{After: "projects/site#alpha"},
			want:      "Child body\n\n## Inserted\n## Beta\n",
		},
		{
			name:      "before heading after body",
			level:     2,
			placement: Placement{Before: "projects/site#beta"},
			want:      "Child body\n\n## Inserted\n## Beta\n",
		},
		{
			name:      "under last child ending in body",
			level:     3,
			placement: Placement{Under: "projects/site#alpha"},
			want:      "Child body\n\n### Inserted\n## Beta\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("projects/site.md", flushLifecycleOutline).
				Build()
			rt := testRuntime(t, v.Path)
			indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

			result, err := Create(rt, CreateRequest{
				FileReference:  "projects/site",
				Title:          "Inserted",
				Level:          tt.level,
				Placement:      tt.placement,
				FailOnIndexErr: true,
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if result.SectionID != "projects/site#inserted" {
				t.Fatalf("SectionID = %q, want projects/site#inserted", result.SectionID)
			}

			content := v.ReadFile("projects/site.md")
			if !strings.Contains(content, tt.want) {
				t.Fatalf("missing blank line before heading %q:\n%s", tt.want, content)
			}
			heading := strings.Repeat("#", tt.level) + " Inserted"
			if strings.Contains(content, heading+"\n\n") {
				t.Fatalf("inserted a blank line after the heading:\n%s", content)
			}
		})
	}
}

func TestCreateLeavesExistingBlankLineBeforeHeading(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Create(rt, CreateRequest{
		FileReference:  "projects/site",
		Title:          "Inserted",
		Level:          2,
		Placement:      Placement{Before: "projects/site#beta"},
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	content := v.ReadFile("projects/site.md")
	if !strings.Contains(content, "Child body\n\n## Inserted\n## Beta\n") {
		t.Fatalf("expected one existing blank line before heading, not an extra blank:\n%s", content)
	}
}

func TestCreateRejectsSectionFileReference(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Create(rt, CreateRequest{
		FileReference:  "projects/site#alpha",
		Title:          "Inserted",
		Level:          2,
		FailOnIndexErr: true,
	})
	if err == nil || !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("Create() error = %v, want file-only rejection", err)
	}
	assertServiceCode(t, err, "INVALID_INPUT")
	if got := v.ReadFile("projects/site.md"); got != lifecycleOutline {
		t.Fatalf("rejected create changed file:\n%s", got)
	}
}

func TestCreateDryRunDoesNotWrite(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	result, err := Create(rt, CreateRequest{
		FileReference:  "projects/site",
		Title:          "Preview",
		Level:          2,
		Placement:      Placement{After: "projects/site#alpha"},
		Preview:        true,
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.SectionID != "projects/site#preview" {
		t.Fatalf("SectionID = %q, want projects/site#preview", result.SectionID)
	}
	if got := v.ReadFile("projects/site.md"); got != lifecycleOutline {
		t.Fatalf("dry run changed file:\n%s", got)
	}
}

func TestCreateRejectsIllegalDepthAndSlugCollision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		title     string
		level     int
		placement Placement
		wantError string
	}{
		{
			name:      "under depth mismatch",
			title:     "Too Deep",
			level:     4,
			placement: Placement{Under: "projects/site#alpha"},
			wantError: "expected level 3",
		},
		{
			name:      "after depth mismatch",
			title:     "Wrong Sibling",
			level:     3,
			placement: Placement{After: "projects/site#alpha"},
			wantError: "expected level 2",
		},
		{
			name:      "slug collision",
			title:     "Beta",
			level:     2,
			placement: Placement{Before: "projects/site#beta"},
			wantError: "shift slug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("projects/site.md", lifecycleOutline).
				Build()
			rt := testRuntime(t, v.Path)
			indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

			_, err := Create(rt, CreateRequest{
				FileReference:  "projects/site",
				Title:          tt.title,
				Level:          tt.level,
				Placement:      tt.placement,
				FailOnIndexErr: true,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Create() error = %v, want substring %q", err, tt.wantError)
			}
			if got := v.ReadFile("projects/site.md"); got != lifecycleOutline {
				t.Fatalf("failed create changed file:\n%s", got)
			}
		})
	}
}

func TestMoveRejectsFileReference(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Move(rt, MoveRequest{
		Reference:      "projects/site",
		Placement:      Placement{After: "projects/site#beta"},
		FailOnIndexErr: true,
	})
	if err == nil || !strings.Contains(err.Error(), "section reference required") {
		t.Fatalf("Move() error = %v, want section-only rejection", err)
	}
	assertServiceCode(t, err, "INVALID_INPUT")
}

func TestMoveReordersEntireSubtree(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	result, err := Move(rt, MoveRequest{
		Reference:      "projects/site#alpha",
		Placement:      Placement{After: "projects/site#beta"},
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if result.SectionID != "projects/site#alpha" {
		t.Fatalf("SectionID = %q, want projects/site#alpha", result.SectionID)
	}
	content := v.ReadFile("projects/site.md")
	assertHeadingOrder(t, content, "## Beta", "## Alpha", "### Alpha Child")
	if !strings.Contains(content, "### Alpha Child\n\nChild body") {
		t.Fatalf("child content did not move with subtree:\n%s", content)
	}
}

func TestMoveBeforeSibling(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Move(rt, MoveRequest{
		Reference:      "projects/site#beta",
		Placement:      Placement{Before: "projects/site#alpha"},
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	assertHeadingOrder(t, v.ReadFile("projects/site.md"), "## Beta", "## Alpha", "### Alpha Child")
}

func TestMoveToEOF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		wantOrder []string
	}{
		{
			name:      "moves earlier subtree to EOF",
			reference: "projects/site#alpha",
			wantOrder: []string{"## Beta", "## Alpha", "### Alpha Child"},
		},
		{
			name:      "last subtree is a no-op",
			reference: "projects/site#beta",
			wantOrder: []string{"## Alpha", "### Alpha Child", "## Beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("projects/site.md", lifecycleOutline).
				Build()
			rt := testRuntime(t, v.Path)
			indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

			_, err := Move(rt, MoveRequest{
				Reference:      tt.reference,
				FailOnIndexErr: true,
			})
			if err != nil {
				t.Fatalf("Move() error = %v", err)
			}
			assertHeadingOrder(t, v.ReadFile("projects/site.md"), tt.wantOrder...)
		})
	}
}

func TestMoveReparentsSubtree(t *testing.T) {
	t.Parallel()

	content := `---
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
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", content).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Move(rt, MoveRequest{
		Reference:      "projects/site#gamma",
		Placement:      Placement{Under: "projects/site#alpha"},
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	updated := v.ReadFile("projects/site.md")
	assertHeadingOrder(t, updated, "## Alpha", "### Gamma", "#### Gamma Child", "## Beta")
	if !strings.Contains(updated, "#### Gamma Child\n\nChild body") {
		t.Fatalf("child content did not move with reparented subtree:\n%s", updated)
	}
}

func TestMoveDryRunAndInvalidPlacements(t *testing.T) {
	t.Parallel()

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()

		v := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/site.md", lifecycleOutline).
			Build()
		rt := testRuntime(t, v.Path)
		indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

		_, err := Move(rt, MoveRequest{
			Reference:      "projects/site#alpha",
			Placement:      Placement{After: "projects/site#beta"},
			Preview:        true,
			FailOnIndexErr: true,
		})
		if err != nil {
			t.Fatalf("Move() error = %v", err)
		}
		if got := v.ReadFile("projects/site.md"); got != lifecycleOutline {
			t.Fatalf("dry run changed file:\n%s", got)
		}
	})

	tests := []struct {
		name      string
		reference string
		placement Placement
		wantError string
	}{
		{
			name:      "illegal reparent depth",
			reference: "projects/site#beta",
			placement: Placement{Under: "projects/site#alpha"},
			wantError: "expected level 3",
		},
		{
			name:      "own descendant",
			reference: "projects/site#alpha",
			placement: Placement{Under: "projects/site#alpha-child"},
			wantError: "itself or its descendant",
		},
		{
			name:      "mutually exclusive anchors",
			reference: "projects/site#beta",
			placement: Placement{After: "projects/site#alpha", Before: "projects/site#alpha"},
			wantError: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("projects/site.md", lifecycleOutline).
				Build()
			rt := testRuntime(t, v.Path)
			indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

			_, err := Move(rt, MoveRequest{
				Reference:      tt.reference,
				Placement:      tt.placement,
				FailOnIndexErr: true,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Move() error = %v, want substring %q", err, tt.wantError)
			}
			if got := v.ReadFile("projects/site.md"); got != lifecycleOutline {
				t.Fatalf("failed move changed file:\n%s", got)
			}
		})
	}
}

func TestMoveRejectsSlugShifts(t *testing.T) {
	t.Parallel()

	content := `---
type: project
title: Site
status: active
---

## Repeat

First

## Other

## Repeat

Second
`
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", content).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Move(rt, MoveRequest{
		Reference:      "projects/site#repeat-2",
		Placement:      Placement{Before: "projects/site#repeat"},
		FailOnIndexErr: true,
	})
	if err == nil || !strings.Contains(err.Error(), "shift slug") {
		t.Fatalf("Move() error = %v, want slug-shift rejection", err)
	}
	if got := v.ReadFile("projects/site.md"); got != content {
		t.Fatalf("failed move changed file:\n%s", got)
	}
}

func assertHeadingOrder(t *testing.T, content string, headings ...string) {
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
