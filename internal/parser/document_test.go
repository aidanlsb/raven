package parser

import (
	"testing"
)

func TestParseDocument(t *testing.T) {
	t.Parallel()
	t.Run("simple document", func(t *testing.T) {
		content := `---
type: person
name: Freya
---

# Freya

Some content about Freya.
`
		doc, err := ParseDocument(content, "/vault/people/freya.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if doc.FilePath != "people/freya.md" {
			t.Errorf("FilePath = %q, want %q", doc.FilePath, "people/freya.md")
		}

		// Should have file-level object + section for "# Freya"
		if len(doc.Objects) != 1 {
			t.Errorf("got %d objects, want 1", len(doc.Objects))
		}

		if doc.Objects[0].ID != "people/freya" {
			t.Errorf("first object ID = %q, want %q", doc.Objects[0].ID, "people/freya")
		}

		if doc.Objects[0].ObjectType != "person" {
			t.Errorf("first object type = %q, want %q", doc.Objects[0].ObjectType, "person")
		}

		if len(doc.Sections) != 1 {
			t.Fatalf("got %d sections, want 1", len(doc.Sections))
		}
		if doc.Sections[0].ID != "people/freya#freya" {
			t.Errorf("section ID = %q, want people/freya#freya", doc.Sections[0].ID)
		}
	})

	t.Run("document with sections", func(t *testing.T) {
		content := `# Introduction

Some intro text.

## Background

More text here.

## Methods

Even more text.
`
		doc, err := ParseDocument(content, "/vault/doc.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// File + 3 sections
		if len(doc.Objects) != 1 {
			t.Errorf("got %d objects, want 1", len(doc.Objects))
		}

		if doc.Objects[0].ObjectType != "page" {
			t.Errorf("file object type = %q, want page", doc.Objects[0].ObjectType)
		}

		if len(doc.Sections) != 3 {
			t.Fatalf("got %d sections, want 3", len(doc.Sections))
		}
		if doc.Sections[0].ID != "doc#introduction" {
			t.Errorf("first section ID = %q, want doc#introduction", doc.Sections[0].ID)
		}
		if doc.Sections[1].ID != "doc#background" {
			t.Errorf("second section ID = %q, want doc#background", doc.Sections[1].ID)
		}
		if doc.Sections[2].ID != "doc#methods" {
			t.Errorf("third section ID = %q, want doc#methods", doc.Sections[2].ID)
		}
	})

	t.Run("legacy explicit type with explicit id is plain text", func(t *testing.T) {
		content := `# Weekly Standup
::meeting(id=standup, time=09:00)

Discussion notes here.
`
		doc, err := ParseDocument(content, "/vault/meetings.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(doc.Objects) != 1 {
			t.Errorf("got %d objects, want 1", len(doc.Objects))
		}
		if len(doc.Sections) != 1 || doc.Sections[0].ID != "meetings#weekly-standup" {
			t.Fatalf("sections = %+v, want meetings#weekly-standup", doc.Sections)
		}
	})

	t.Run("legacy explicit type with id derived from heading is plain text", func(t *testing.T) {
		content := `# Weekly Standup
::meeting(time=09:00)

Discussion notes here.
`
		doc, err := ParseDocument(content, "/vault/meetings.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(doc.Objects) != 1 {
			t.Errorf("got %d objects, want 1", len(doc.Objects))
		}
		if len(doc.Sections) != 1 || doc.Sections[0].ID != "meetings#weekly-standup" {
			t.Fatalf("sections = %+v, want meetings#weekly-standup", doc.Sections)
		}
	})

	t.Run("duplicate headings with derived ids", func(t *testing.T) {
		content := `# Notes

## Team Sync
::meeting(time=09:00)

First meeting.

## Team Sync
::meeting(time=14:00)

Second meeting with same heading.
`
		doc, err := ParseDocument(content, "/vault/daily.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// File + Notes section + 2 Team Sync sections. ::meeting lines are plain text.
		if len(doc.Objects) != 1 {
			t.Errorf("got %d objects, want 1", len(doc.Objects))
		}
		if len(doc.Sections) != 3 {
			t.Fatalf("got %d sections, want 3", len(doc.Sections))
		}
		if doc.Sections[1].ID != "daily#team-sync" {
			t.Errorf("first team sync ID = %q, want daily#team-sync", doc.Sections[1].ID)
		}
		if doc.Sections[2].ID != "daily#team-sync-2" {
			t.Errorf("second team sync ID = %q, want daily#team-sync-2", doc.Sections[2].ID)
		}
	})

	t.Run("legacy explicit id does not override heading slug", func(t *testing.T) {
		content := `# Very Long Meeting Title That Would Make A Bad ID
::meeting(id=standup, time=09:00)

Discussion notes here.
`
		doc, err := ParseDocument(content, "/vault/meetings.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(doc.Sections) != 1 || doc.Sections[0].ID != "meetings#very-long-meeting-title-that-would-make-a-bad-id" {
			t.Fatalf("sections = %+v, want heading-derived section ID", doc.Sections)
		}
	})

	t.Run("duplicate heading IDs", func(t *testing.T) {
		content := `# Notes

## Ideas

First ideas section.

## Ideas

Second ideas section with same heading.
`
		doc, err := ParseDocument(content, "/vault/doc.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have unique section IDs
		if len(doc.Objects) != 1 {
			t.Errorf("got %d objects, want 1", len(doc.Objects))
		}
		if len(doc.Sections) != 3 {
			t.Fatalf("got %d sections, want 3", len(doc.Sections))
		}
		if doc.Sections[1].ID != "doc#ideas" {
			t.Errorf("first ideas ID = %q, want doc#ideas", doc.Sections[1].ID)
		}
		if doc.Sections[2].ID != "doc#ideas-2" {
			t.Errorf("second ideas ID = %q, want doc#ideas-2", doc.Sections[2].ID)
		}
	})

	t.Run("natural slug does not collide with generated duplicate slug", func(t *testing.T) {
		content := `# Notes

## Foo

First section.

## Foo

Duplicate section.

## Foo 2

Natural foo-2 section.
`
		doc, err := ParseDocument(content, "/vault/doc.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(doc.Objects) != 1 {
			t.Fatalf("got %d objects, want 1", len(doc.Objects))
		}
		if len(doc.Sections) != 4 {
			t.Fatalf("got %d sections, want 4", len(doc.Sections))
		}

		if doc.Sections[1].ID != "doc#foo" {
			t.Errorf("first foo ID = %q, want doc#foo", doc.Sections[1].ID)
		}
		if doc.Sections[2].ID != "doc#foo-2" {
			t.Errorf("second foo ID = %q, want doc#foo-2", doc.Sections[2].ID)
		}
		if doc.Sections[3].ID != "doc#foo-2-2" {
			t.Errorf("foo 2 ID = %q, want doc#foo-2-2", doc.Sections[3].ID)
		}
	})

	t.Run("trait parented to section", func(t *testing.T) {
		content := `# Project

## Tasks

- @task(due=2024-01-15) Do the thing
`
		doc, err := ParseDocument(content, "/vault/project.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(doc.Traits) != 1 {
			t.Errorf("got %d traits, want 1", len(doc.Traits))
			return
		}

		// Task should be parented to the "Tasks" section
		if doc.Traits[0].ParentObjectID != "project#tasks" {
			t.Errorf("trait parent = %q, want project#tasks", doc.Traits[0].ParentObjectID)
		}
	})

	t.Run("traits in fenced code block ignored", func(t *testing.T) {
		content := `# Notes

- @todo Real task

` + "```python" + `
@decorator
def my_function():
    pass
` + "```" + `

- @done Another real task
`
		doc, err := ParseDocument(content, "/vault/notes.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have 2 traits: @todo and @done
		// The @decorator inside the code block should be ignored
		if len(doc.Traits) != 2 {
			t.Errorf("got %d traits, want 2", len(doc.Traits))
			for _, tr := range doc.Traits {
				t.Logf("  trait: %s", tr.TraitType)
			}
		}
	})

	t.Run("refs in fenced code block ignored", func(t *testing.T) {
		content := `# Notes

See [[real-ref]] for details.

` + "```markdown" + `
This is example code with [[fake-ref]]
` + "```" + `

Also see [[another-real-ref]].
`
		doc, err := ParseDocument(content, "/vault/notes.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have 2 refs: real-ref and another-real-ref
		// The fake-ref inside the code block should be ignored
		if len(doc.Refs) != 2 {
			t.Errorf("got %d refs, want 2", len(doc.Refs))
			for _, ref := range doc.Refs {
				t.Logf("  ref: %s", ref.TargetRaw)
			}
		}
	})

	t.Run("inline code filtering", func(t *testing.T) {
		content := `# Notes

Use ` + "`@decorator`" + ` for Python decorators.

- @todo This is a real task
`
		doc, err := ParseDocument(content, "/vault/notes.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have 1 trait: @todo
		// The @decorator inside backticks should be ignored
		if len(doc.Traits) != 1 {
			t.Errorf("got %d traits, want 1", len(doc.Traits))
			for _, tr := range doc.Traits {
				t.Logf("  trait: %s", tr.TraitType)
			}
		}

		if len(doc.Traits) > 0 && doc.Traits[0].TraitType != "todo" {
			t.Errorf("trait type = %q, want %q", doc.Traits[0].TraitType, "todo")
		}
	})

	t.Run("traits in fenced code block inside list ignored", func(t *testing.T) {
		// Fenced code blocks can appear inside list items with the list marker
		// prefix (e.g., "- ```"). These should still be detected as code blocks.
		content := `# Notes

- @todo Real task

- ` + "```python" + `
  @decorator
  def my_function():
      pass
  ` + "```" + `

- @done Another real task
`
		doc, err := ParseDocument(content, "/vault/notes.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have 2 traits: @todo and @done
		// The @decorator inside the code block (even with list marker) should be ignored
		if len(doc.Traits) != 2 {
			t.Errorf("got %d traits, want 2", len(doc.Traits))
			for _, tr := range doc.Traits {
				t.Logf("  trait: %s", tr.TraitType)
			}
		}
	})

	t.Run("with directory roots", func(t *testing.T) {
		content := `---
type: person
name: Freya
---

# Freya
`
		opts := &ParseOptions{
			ObjectsRoot: "objects/",
			PagesRoot:   "pages/",
		}

		doc, err := ParseDocumentWithOptions(content, "/vault/objects/people/freya.md", "/vault", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Object ID should strip the objects/ prefix
		if doc.Objects[0].ID != "people/freya" {
			t.Errorf("object ID = %q, want %q", doc.Objects[0].ID, "people/freya")
		}
	})

	t.Run("pages root stripping", func(t *testing.T) {
		content := `# My Note
Some content.
`
		opts := &ParseOptions{
			ObjectsRoot: "objects/",
			PagesRoot:   "pages/",
		}

		doc, err := ParseDocumentWithOptions(content, "/vault/pages/my-note.md", "/vault", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Object ID should strip the pages/ prefix
		if doc.Objects[0].ID != "my-note" {
			t.Errorf("object ID = %q, want %q", doc.Objects[0].ID, "my-note")
		}
	})

	t.Run("files outside roots", func(t *testing.T) {
		content := `---
type: date
---
# Daily Note
`
		opts := &ParseOptions{
			ObjectsRoot: "objects/",
			PagesRoot:   "pages/",
		}

		// Daily notes are not under objects/ or pages/
		doc, err := ParseDocumentWithOptions(content, "/vault/daily/2025-01-01.md", "/vault", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Object ID should remain unchanged (no prefix to strip)
		if doc.Objects[0].ID != "daily/2025-01-01" {
			t.Errorf("object ID = %q, want %q", doc.Objects[0].ID, "daily/2025-01-01")
		}
	})

	t.Run("legacy type declaration refs are ordinary body refs", func(t *testing.T) {
		content := `# Daily Note

## Weekly Standup
::meeting(series=[[meetings/standup]])

Discussed project status.
`
		doc, err := ParseDocument(content, "/vault/daily/2025-01-15.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Legacy ::type(...) text is ordinary markdown, so refs on that line are scoped to the section.
		found := false
		for _, ref := range doc.Refs {
			if ref.TargetRaw == "meetings/standup" && ref.SourceID == "daily/2025-01-15#weekly-standup" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ref to meetings/standup from legacy declaration line, got refs: %v", doc.Refs)
		}
	})

	t.Run("refs extracted from legacy type declaration lines", func(t *testing.T) {
		content := `# Daily Note

## Team Sync
::meeting(attendees=[[people/freya]] [[people/thor]])

Team discussion.
`
		doc, err := ParseDocument(content, "/vault/daily/2025-01-15.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have refs from the ordinary markdown line.
		foundFreya := false
		foundThor := false
		for _, ref := range doc.Refs {
			if ref.SourceID == "daily/2025-01-15#team-sync" {
				if ref.TargetRaw == "people/freya" {
					foundFreya = true
				}
				if ref.TargetRaw == "people/thor" {
					foundThor = true
				}
			}
		}
		if !foundFreya {
			t.Errorf("expected ref to people/freya from legacy declaration line")
		}
		if !foundThor {
			t.Errorf("expected ref to people/thor from legacy declaration line")
		}
	})

	t.Run("multiple refs in legacy type declaration line", func(t *testing.T) {
		content := `# Daily Note

## Client Meeting
::meeting(series=[[meetings/acme-weekly]], client=[[companies/acme]])

Discussed roadmap.
`
		doc, err := ParseDocument(content, "/vault/daily/2025-01-15.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have refs from the ordinary markdown line.
		foundSeries := false
		foundClient := false
		for _, ref := range doc.Refs {
			if ref.SourceID == "daily/2025-01-15#client-meeting" {
				if ref.TargetRaw == "meetings/acme-weekly" {
					foundSeries = true
				}
				if ref.TargetRaw == "companies/acme" {
					foundClient = true
				}
			}
		}
		if !foundSeries {
			t.Errorf("expected ref to meetings/acme-weekly from legacy declaration line")
		}
		if !foundClient {
			t.Errorf("expected ref to companies/acme from legacy declaration line")
		}
	})

	t.Run("legacy declaration refs distinct from body refs", func(t *testing.T) {
		content := `# Daily Note

## Standup
::meeting(series=[[meetings/standup]])

Discussed [[projects/website]] progress.
`
		doc, err := ParseDocument(content, "/vault/daily/2025-01-15.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have both the declaration-line ref and body ref.
		foundFieldRef := false
		foundBodyRef := false
		for _, ref := range doc.Refs {
			if ref.TargetRaw == "meetings/standup" {
				foundFieldRef = true
			}
			if ref.TargetRaw == "projects/website" {
				foundBodyRef = true
			}
		}
		if !foundFieldRef {
			t.Errorf("expected ref to meetings/standup from legacy declaration line")
		}
		if !foundBodyRef {
			t.Errorf("expected ref to projects/website from body")
		}
	})
}

func TestParseDocument_OnlyFrontmatterNoBody(t *testing.T) {
	t.Parallel()

	content := `---
type: person
name: Freya
email: freya@asgard.realm
---`

	doc, err := ParseDocument(content, "/vault/people/freya.md", "/vault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(doc.Objects))
	}
	if doc.Objects[0].ID != "people/freya" {
		t.Fatalf("object ID = %q, want %q", doc.Objects[0].ID, "people/freya")
	}
	if len(doc.Traits) != 0 {
		t.Fatalf("got %d traits, want 0", len(doc.Traits))
	}
	if len(doc.Refs) != 0 {
		t.Fatalf("got %d refs, want 0", len(doc.Refs))
	}
}

func TestParseDocument_EmptyHeadingIgnored(t *testing.T) {
	t.Parallel()

	content := `# Title

##

Body text.
`

	doc, err := ParseDocument(content, "/vault/doc.md", "/vault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(doc.Objects))
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(doc.Sections))
	}
	if doc.Sections[0].ID != "doc#title" {
		t.Fatalf("section ID = %q, want %q", doc.Sections[0].ID, "doc#title")
	}
}

func TestParseDocument_UnicodeHeadingSlugs(t *testing.T) {
	t.Parallel()

	content := `# Über Alles

## 日本語
`

	doc, err := ParseDocument(content, "/vault/doc.md", "/vault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(doc.Objects))
	}
	if len(doc.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(doc.Sections))
	}
	if doc.Sections[0].ID != "doc#über-alles" {
		t.Fatalf("first section ID = %q, want %q", doc.Sections[0].ID, "doc#über-alles")
	}
	if doc.Sections[1].ID != "doc#日本語" {
		t.Fatalf("second section ID = %q, want %q", doc.Sections[1].ID, "doc#日本語")
	}
}

func TestParseDocument_DeepHeadingHierarchy(t *testing.T) {
	t.Parallel()

	content := `# L1

## L2

### L3

#### L4

##### L5

###### L6
`

	doc, err := ParseDocument(content, "/vault/doc.md", "/vault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(doc.Objects))
	}
	if len(doc.Sections) != 6 {
		t.Fatalf("got %d sections, want 6", len(doc.Sections))
	}

	for i := 1; i < len(doc.Sections); i++ {
		parent := doc.Sections[i].ParentSectionID
		if parent == nil {
			t.Fatalf("section %q missing parent", doc.Sections[i].ID)
		}
		if *parent != doc.Sections[i-1].ID {
			t.Fatalf("section %q parent = %q, want %q", doc.Sections[i].ID, *parent, doc.Sections[i-1].ID)
		}
	}
}

func TestFindScopeForLine(t *testing.T) {
	t.Parallel()

	sections := []*ParsedSection{
		{ID: "doc#intro", LineStart: 5},
		{ID: "doc#details", LineStart: 12},
	}

	tests := []struct {
		name string
		line int
		want string
	}{
		{name: "before first heading stays on file object", line: 1, want: "doc"},
		{name: "between headings uses nearest parent", line: 8, want: "doc#intro"},
		{name: "exact heading line matches that object", line: 12, want: "doc#details"},
		{name: "after last heading uses last object", line: 20, want: "doc#details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := findScopeForLine("doc", sections, tt.line); got != tt.want {
				t.Fatalf("findScopeForLine(..., %d) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
