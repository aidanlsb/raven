package lessons

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/parser"
)

const (
	defaultSyllabusPath = "defaults/syllabus.yaml"
	defaultLessonsDir   = "defaults/lessons"
)

//go:embed defaults/syllabus.yaml defaults/lessons/*.md
var defaultLessonsFS embed.FS

// Catalog contains the sectioned syllabus and lesson content.
type Catalog struct {
	Sections []Section
	Lessons  map[string]Lesson
}

// Section describes an ordered group of lesson IDs.
type Section struct {
	ID      string
	Title   string
	Lessons []string
}

// Lesson is a single teachable unit.
type Lesson struct {
	ID      string
	Title   string
	Prereqs []string
	Docs    []string
	Content string
}

type syllabusFile struct {
	Sections []syllabusSection `yaml:"sections"`
}

type syllabusSection struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Lessons []string `yaml:"lessons"`
}

type lessonFrontmatter struct {
	Title   string   `yaml:"title"`
	Prereqs []string `yaml:"prereqs"`
	Docs    []string `yaml:"docs"`
}

// LoadCatalog loads the embedded lessons catalog.
func LoadCatalog() (*Catalog, error) {
	return loadCatalogFromFS(defaultLessonsFS)
}

func loadCatalogFromFS(fsys fs.FS) (*Catalog, error) {
	syllabusRaw, err := fs.ReadFile(fsys, defaultSyllabusPath)
	if err != nil {
		return nil, fmt.Errorf("read syllabus: %w", err)
	}

	var syllabus syllabusFile
	if err := yaml.Unmarshal(syllabusRaw, &syllabus); err != nil {
		return nil, fmt.Errorf("parse syllabus: %w", err)
	}
	if len(syllabus.Sections) == 0 {
		return nil, fmt.Errorf("syllabus has no sections")
	}

	catalog := &Catalog{
		Sections: make([]Section, 0, len(syllabus.Sections)),
		Lessons:  make(map[string]Lesson),
	}

	seenSectionIDs := make(map[string]bool, len(syllabus.Sections))
	seenLessonIDs := map[string]string{}
	orderedLessonIDs := make([]string, 0)

	for i, rawSection := range syllabus.Sections {
		sectionID := strings.TrimSpace(rawSection.ID)
		sectionTitle := strings.TrimSpace(rawSection.Title)
		if sectionID == "" {
			return nil, fmt.Errorf("section %d is missing id", i)
		}
		if sectionTitle == "" {
			return nil, fmt.Errorf("section %q is missing title", sectionID)
		}
		if seenSectionIDs[sectionID] {
			return nil, fmt.Errorf("duplicate section id: %q", sectionID)
		}
		seenSectionIDs[sectionID] = true

		if len(rawSection.Lessons) == 0 {
			return nil, fmt.Errorf("section %q has no lessons", sectionID)
		}

		sectionLessonIDs := make([]string, 0, len(rawSection.Lessons))
		seenInSection := map[string]bool{}

		for _, rawLessonID := range rawSection.Lessons {
			lessonID := strings.TrimSpace(rawLessonID)
			if lessonID == "" {
				return nil, fmt.Errorf("section %q contains an empty lesson id", sectionID)
			}
			if seenInSection[lessonID] {
				return nil, fmt.Errorf("section %q contains duplicate lesson id %q", sectionID, lessonID)
			}
			if priorSection, exists := seenLessonIDs[lessonID]; exists {
				return nil, fmt.Errorf("lesson id %q appears in multiple sections (%q, %q)", lessonID, priorSection, sectionID)
			}

			seenInSection[lessonID] = true
			seenLessonIDs[lessonID] = sectionID
			sectionLessonIDs = append(sectionLessonIDs, lessonID)
			orderedLessonIDs = append(orderedLessonIDs, lessonID)
		}

		catalog.Sections = append(catalog.Sections, Section{
			ID:      sectionID,
			Title:   sectionTitle,
			Lessons: sectionLessonIDs,
		})
	}

	for _, lessonID := range orderedLessonIDs {
		lessonPath := path.Join(defaultLessonsDir, lessonID+".md")
		lessonRaw, err := fs.ReadFile(fsys, lessonPath)
		if err != nil {
			return nil, fmt.Errorf("read lesson %q: %w", lessonID, err)
		}

		lesson, err := parseLessonMarkdown(lessonID, string(lessonRaw))
		if err != nil {
			return nil, err
		}
		catalog.Lessons[lessonID] = lesson
	}

	for _, lesson := range catalog.Lessons {
		for _, prereq := range lesson.Prereqs {
			if prereq == lesson.ID {
				return nil, fmt.Errorf("lesson %q cannot list itself as a prerequisite", lesson.ID)
			}
			if _, ok := catalog.Lessons[prereq]; !ok {
				return nil, fmt.Errorf("lesson %q has unknown prerequisite %q", lesson.ID, prereq)
			}
		}
	}

	if err := validateNoPrereqCycles(catalog); err != nil {
		return nil, err
	}

	return catalog, nil
}

func parseLessonMarkdown(lessonID, raw string) (Lesson, error) {
	lines := strings.Split(raw, "\n")
	_, endLine, hasFrontmatter := parser.FrontmatterBounds(lines)
	if !hasFrontmatter || endLine == -1 {
		return Lesson{}, fmt.Errorf("lesson %q is missing a closed YAML frontmatter block", lessonID)
	}

	frontmatterRaw := strings.Join(lines[1:endLine], "\n")
	var fm lessonFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &fm); err != nil {
		return Lesson{}, fmt.Errorf("parse frontmatter for lesson %q: %w", lessonID, err)
	}

	title := strings.TrimSpace(fm.Title)
	if title == "" {
		return Lesson{}, fmt.Errorf("lesson %q is missing required frontmatter field 'title'", lessonID)
	}

	body := ""
	if endLine+1 < len(lines) {
		body = strings.Join(lines[endLine+1:], "\n")
	}
	body = strings.TrimLeft(body, "\n")

	return Lesson{
		ID:      lessonID,
		Title:   title,
		Prereqs: normalizeLessonIDs(fm.Prereqs),
		Docs:    normalizeLessonDocLinks(fm.Docs),
		Content: body,
	}, nil
}

func normalizeLessonIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func normalizeLessonDocLinks(docs []string) []string {
	out := make([]string, 0, len(docs))
	seen := map[string]bool{}
	for _, raw := range docs {
		doc := strings.TrimSpace(raw)
		if doc == "" || seen[doc] {
			continue
		}
		seen[doc] = true
		out = append(out, doc)
	}
	return out
}

func validateNoPrereqCycles(catalog *Catalog) error {
	visiting := map[string]bool{}
	visited := map[string]bool{}

	var walk func(id string) error
	walk = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("prerequisite cycle detected at lesson %q", id)
		}

		visiting[id] = true
		lesson := catalog.Lessons[id]
		for _, prereqID := range lesson.Prereqs {
			if err := walk(prereqID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}

	for _, section := range catalog.Sections {
		for _, lessonID := range section.Lessons {
			if err := walk(lessonID); err != nil {
				return err
			}
		}
	}

	return nil
}

// LessonByID returns a lesson by its ID.
func (c *Catalog) LessonByID(id string) (Lesson, bool) {
	lesson, ok := c.Lessons[id]
	return lesson, ok
}

// NextSuggested returns the first incomplete lesson in linear syllabus order.
func (c *Catalog) NextSuggested(progress *Progress) (Lesson, bool) {
	if c == nil {
		return Lesson{}, false
	}
	for _, section := range c.Sections {
		for _, lessonID := range section.Lessons {
			if progress != nil && progress.IsCompleted(lessonID) {
				continue
			}
			lesson, ok := c.Lessons[lessonID]
			if !ok {
				continue
			}
			return lesson, true
		}
	}
	return Lesson{}, false
}
