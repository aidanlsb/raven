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

	t.Run("explicit type overrides section", func(t *testing.T) {
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
