package lessons

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadCatalogDefaults(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	if len(catalog.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(catalog.Sections))
	}

	section := catalog.Sections[0]
	if section.ID != "foundations" {
		t.Fatalf("expected section id foundations, got %q", section.ID)
	}
	if len(section.Lessons) != 2 {
		t.Fatalf("expected 2 lessons in section, got %d", len(section.Lessons))
	}

	if _, ok := catalog.LessonByID("objects"); !ok {
		t.Fatalf("expected objects lesson to exist")
	}
	if _, ok := catalog.LessonByID("refs"); !ok {
		t.Fatalf("expected refs lesson to exist")
	}
}

func TestNextSuggested(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	progress := NewProgress()
	next, ok := catalog.NextSuggested(progress)
	if !ok {
		t.Fatalf("expected next lesson for empty progress")
	}
	if next.ID != "objects" {
		t.Fatalf("expected next lesson objects, got %q", next.ID)
	}

	progress.MarkCompleted("objects", "2026-02-15")
	next, ok = catalog.NextSuggested(progress)
	if !ok {
		t.Fatalf("expected next lesson after completing objects")
	}
	if next.ID != "refs" {
		t.Fatalf("expected next lesson refs, got %q", next.ID)
	}

	progress.MarkCompleted("refs", "2026-02-16")
	if _, ok := catalog.NextSuggested(progress); ok {
		t.Fatalf("expected no next lesson after completing all lessons")
	}
}

func TestLoadCatalogRejectsUnknownPrereq(t *testing.T) {
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
prereqs:
  - missing
---

# Objects
`),
		},
	}

	_, err := loadCatalogFromFS(fsys)
	if err == nil {
		t.Fatalf("expected error for unknown prerequisite")
	}
	if !strings.Contains(err.Error(), "unknown prerequisite") {
		t.Fatalf("expected unknown prerequisite error, got: %v", err)
	}
}

func TestLoadCatalogParsesAndNormalizesDocsFrontmatter(t *testing.T) {
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
docs:
  - docs/getting-started/core-concepts.md#files-are-objects
  - " docs/types-and-traits/file-format.md#frontmatter "
  - docs/getting-started/core-concepts.md#files-are-objects
  - ""
---

# Objects
`),
		},
	}

	catalog, err := loadCatalogFromFS(fsys)
	if err != nil {
		t.Fatalf("loadCatalogFromFS() error = %v", err)
	}

	lesson, ok := catalog.LessonByID("objects")
	if !ok {
		t.Fatalf("expected objects lesson")
	}

	if len(lesson.Docs) != 2 {
		t.Fatalf("expected 2 unique docs, got %d: %#v", len(lesson.Docs), lesson.Docs)
	}
	if lesson.Docs[0] != "docs/getting-started/core-concepts.md#files-are-objects" {
		t.Fatalf("unexpected first docs link: %q", lesson.Docs[0])
	}
	if lesson.Docs[1] != "docs/types-and-traits/file-format.md#frontmatter" {
		t.Fatalf("unexpected second docs link: %q", lesson.Docs[1])
	}
}
