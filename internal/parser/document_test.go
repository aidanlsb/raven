package parser

import (
	"testing"
)

func TestParseDocument(t *testing.T) {
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
		if len(doc.Objects) != 2 {
			t.Errorf("got %d objects, want 2", len(doc.Objects))
		}

		if doc.Objects[0].ID != "people/freya" {
			t.Errorf("first object ID = %q, want %q", doc.Objects[0].ID, "people/freya")
		}

		if doc.Objects[0].ObjectType != "person" {
			t.Errorf("first object type = %q, want %q", doc.Objects[0].ObjectType, "person")
		}

		if doc.Objects[1].ObjectType != "section" {
			t.Errorf("second object type = %q, want %q", doc.Objects[1].ObjectType, "section")
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
		if len(doc.Objects) != 4 {
			t.Errorf("got %d objects, want 4", len(doc.Objects))
		}

		if doc.Objects[0].ObjectType != "page" {
			t.Errorf("file object type = %q, want page", doc.Objects[0].ObjectType)
		}

		if doc.Objects[1].ID != "doc#introduction" {
			t.Errorf("first section ID = %q, want doc#introduction", doc.Objects[1].ID)
		}

		if doc.Objects[2].ID != "doc#background" {
			t.Errorf("second section ID = %q, want doc#background", doc.Objects[2].ID)
		}

		if doc.Objects[3].ID != "doc#methods" {
			t.Errorf("third section ID = %q, want doc#methods", doc.Objects[3].ID)
		}
	})

	t.Run("explicit type with explicit id", func(t *testing.T) {
		content := `# Weekly Standup
::meeting(id=standup, time=09:00)

Discussion notes here.
`
		doc, err := ParseDocument(content, "/vault/meetings.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// File + 1 meeting (not section)
		if len(doc.Objects) != 2 {
			t.Errorf("got %d objects, want 2", len(doc.Objects))
		}

		if doc.Objects[1].ObjectType != "meeting" {
			t.Errorf("second object type = %q, want meeting", doc.Objects[1].ObjectType)
		}

		if doc.Objects[1].ID != "meetings#standup" {
			t.Errorf("meeting ID = %q, want meetings#standup", doc.Objects[1].ID)
		}
	})

	t.Run("explicit type with id derived from heading", func(t *testing.T) {
		content := `# Weekly Standup
::meeting(time=09:00)

Discussion notes here.
`
		doc, err := ParseDocument(content, "/vault/meetings.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// File + 1 meeting (not section)
		if len(doc.Objects) != 2 {
			t.Errorf("got %d objects, want 2", len(doc.Objects))
		}

		if doc.Objects[1].ObjectType != "meeting" {
			t.Errorf("second object type = %q, want meeting", doc.Objects[1].ObjectType)
		}

		// ID should be derived from slugified heading
		if doc.Objects[1].ID != "meetings#weekly-standup" {
			t.Errorf("meeting ID = %q, want meetings#weekly-standup", doc.Objects[1].ID)
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

		// File + Notes section + 2 meetings
		if len(doc.Objects) != 4 {
			t.Errorf("got %d objects, want 4", len(doc.Objects))
		}

		// First meeting gets the base slug
		if doc.Objects[2].ID != "daily#team-sync" {
			t.Errorf("first meeting ID = %q, want daily#team-sync", doc.Objects[2].ID)
		}

		// Second meeting gets -2 suffix
		if doc.Objects[3].ID != "daily#team-sync-2" {
			t.Errorf("second meeting ID = %q, want daily#team-sync-2", doc.Objects[3].ID)
		}
	})

	t.Run("explicit id overrides heading slug", func(t *testing.T) {
		content := `# Very Long Meeting Title That Would Make A Bad ID
::meeting(id=standup, time=09:00)

Discussion notes here.
`
		doc, err := ParseDocument(content, "/vault/meetings.md", "/vault")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Explicit id=standup should be used, not the slugified heading
		if doc.Objects[1].ID != "meetings#standup" {
			t.Errorf("meeting ID = %q, want meetings#standup", doc.Objects[1].ID)
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

		// Should have unique IDs
		if len(doc.Objects) != 4 {
			t.Errorf("got %d objects, want 4", len(doc.Objects))
		}

		if doc.Objects[2].ID != "doc#ideas" {
			t.Errorf("first ideas ID = %q, want doc#ideas", doc.Objects[2].ID)
		}

		if doc.Objects[3].ID != "doc#ideas-2" {
			t.Errorf("second ideas ID = %q, want doc#ideas-2", doc.Objects[3].ID)
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
}
