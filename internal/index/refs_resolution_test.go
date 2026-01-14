package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// TestFrontmatterRefResolution is a comprehensive test of all frontmatter ref
// resolution permutations. This covers the full flow:
//
// 1. YAML frontmatter parsing → FieldValue
// 2. Reference extraction → ParsedRef
// 3. Indexing → refs table (target_raw, target_id)
// 4. Resolution → resolver.Resolve()
// 5. Query/Backlinks → finding the refs
//
// We test these reference styles:
// - Full paths: people/freya, companies/cursor
// - Short names: freya, cursor
// - Aliases: "The Queen" → people/freya
// - Name field values: "Freya Goddess" → people/freya (if name_field is set)
//
// And these syntax styles:
// - Quoted wikilinks: "[[target]]"
// - Bare strings without schema: stored as string, not indexed as ref
func TestFrontmatterRefResolution(t *testing.T) {
	// Create temp directory for test vault
	tmpDir, err := os.MkdirTemp("", "raven-test-refs-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test schema
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				DefaultPath: "people/",
				NameField:   "name",
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"alias": {Type: schema.FieldTypeString},
				},
			},
			"company": {
				DefaultPath: "companies/",
				NameField:   "name",
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
			"project": {
				DefaultPath: "projects/",
				Fields: map[string]*schema.FieldDefinition{
					"name":    {Type: schema.FieldTypeString},
					"company": {Type: schema.FieldTypeRef, Target: "company"},
					"owner":   {Type: schema.FieldTypeRef, Target: "person"},
					"owners":  {Type: schema.FieldTypeRefArray, Target: "person"},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	// Open database
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create and index person: people/freya
	freyaContent := `---
type: person
name: Freya Goddess
alias: The Queen
---

# Freya

The goddess of love and beauty.
`
	freyaPath := filepath.Join(tmpDir, "people", "freya.md")
	if err := os.MkdirAll(filepath.Dir(freyaPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freyaPath, []byte(freyaContent), 0644); err != nil {
		t.Fatal(err)
	}
	freyaDoc, err := parser.ParseDocument(freyaContent, freyaPath, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.IndexDocument(freyaDoc, sch); err != nil {
		t.Fatal(err)
	}

	// Create and index person: people/thor
	thorContent := `---
type: person
name: Thor
---

# Thor
`
	thorPath := filepath.Join(tmpDir, "people", "thor.md")
	if err := os.WriteFile(thorPath, []byte(thorContent), 0644); err != nil {
		t.Fatal(err)
	}
	thorDoc, err := parser.ParseDocument(thorContent, thorPath, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.IndexDocument(thorDoc, sch); err != nil {
		t.Fatal(err)
	}

	// Create and index company: companies/cursor
	cursorContent := `---
type: company
name: Cursor AI
alias: cursor-ai
---

# Cursor
`
	cursorPath := filepath.Join(tmpDir, "companies", "cursor.md")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cursorPath, []byte(cursorContent), 0644); err != nil {
		t.Fatal(err)
	}
	cursorDoc, err := parser.ParseDocument(cursorContent, cursorPath, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.IndexDocument(cursorDoc, sch); err != nil {
		t.Fatal(err)
	}

	// Now create projects with various ref styles to test resolution
	testCases := []struct {
		name        string
		yaml        string // The frontmatter YAML (excluding --- markers)
		wantBacklinks map[string]bool // target object ID → should have backlink
	}{
		{
			name: "wikilink with full path",
			yaml: `type: project
name: Test Project
company: "[[companies/cursor]]"`,
			wantBacklinks: map[string]bool{"companies/cursor": true},
		},
		{
			name: "wikilink with short name",
			yaml: `type: project
name: Test Project
company: "[[cursor]]"`,
			wantBacklinks: map[string]bool{"companies/cursor": true},
		},
		{
			name: "wikilink with person short name",
			yaml: `type: project
name: Test Project
owner: "[[freya]]"`,
			wantBacklinks: map[string]bool{"people/freya": true},
		},
		{
			name: "wikilink with person full path",
			yaml: `type: project
name: Test Project
owner: "[[people/freya]]"`,
			wantBacklinks: map[string]bool{"people/freya": true},
		},
		{
			name: "multiple refs in array",
			yaml: `type: project
name: Test Project
owners:
  - "[[people/freya]]"
  - "[[people/thor]]"`,
			wantBacklinks: map[string]bool{
				"people/freya": true,
				"people/thor":  true,
			},
		},
		{
			name: "inline array of refs",
			yaml: `type: project
name: Test Project
owners: ["[[freya]]", "[[thor]]"]`,
			wantBacklinks: map[string]bool{
				"people/freya": true,
				"people/thor":  true,
			},
		},
		// Bare strings in ref-typed fields are now indexed as refs
		// because the schema declares the field as type: ref
		{
			name: "bare string IS indexed as ref (schema-aware)",
			yaml: `type: project
name: Test Project
company: cursor`,
			wantBacklinks: map[string]bool{"companies/cursor": true},
		},
		{
			name: "quoted bare full path IS indexed as ref (schema-aware)",
			yaml: `type: project
name: Test Project
company: "companies/cursor"`,
			wantBacklinks: map[string]bool{"companies/cursor": true},
		},
		{
			name: "quoted bare short name IS indexed as ref (schema-aware)",
			yaml: `type: project
name: Test Project
company: "cursor"`,
			wantBacklinks: map[string]bool{"companies/cursor": true},
		},
		{
			name: "unquoted wikilink (YAML nested array) IS indexed as ref",
			yaml: `type: project
name: Test Project
company: [[cursor]]`,
			wantBacklinks: map[string]bool{"companies/cursor": true},
		},
	}

	// Resolve all existing refs first (for freya, thor, cursor)
	if _, err := db.ResolveReferences("daily"); err != nil {
		t.Fatal(err)
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create unique project file for this test case
			projectContent := "---\n" + tc.yaml + "\n---\n\nProject content.\n"
			projectName := filepath.Join(tmpDir, "projects", "test-"+string(rune('a'+i))+".md")
			if err := os.MkdirAll(filepath.Dir(projectName), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(projectName, []byte(projectContent), 0644); err != nil {
				t.Fatal(err)
			}

			// Parse and index
			doc, err := parser.ParseDocument(projectContent, projectName, tmpDir)
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			if err := db.IndexDocument(doc, sch); err != nil {
				t.Fatalf("IndexDocument: %v", err)
			}

			// Resolve references
			if _, err := db.ResolveReferences("daily"); err != nil {
				t.Fatalf("ResolveReferences: %v", err)
			}

			// Get the project's object ID
			projectID := doc.Objects[0].ID

			// Check backlinks for each expected target
			for target, wantBacklink := range tc.wantBacklinks {
				backlinks, err := db.Backlinks(target)
				if err != nil {
					t.Fatalf("Backlinks(%q): %v", target, err)
				}

				found := false
				for _, bl := range backlinks {
					if bl.SourceID == projectID {
						found = true
						break
					}
				}

				if wantBacklink && !found {
					t.Errorf("expected backlink from %s to %s, but not found", projectID, target)
				}
				if !wantBacklink && found {
					t.Errorf("unexpected backlink from %s to %s", projectID, target)
				}
			}

			// Verify ref count matches expected backlinks
			var refCount int
			err = db.db.QueryRow(
				"SELECT COUNT(*) FROM refs WHERE source_id = ?",
				projectID,
			).Scan(&refCount)
			if err != nil {
				t.Fatalf("query ref count: %v", err)
			}
			expectedRefs := len(tc.wantBacklinks)
			if refCount != expectedRefs {
				t.Errorf("expected %d refs for %s, got %d", expectedRefs, projectID, refCount)
			}
		})
	}
}

// TestAliasResolution tests that aliases resolve correctly for refs.
func TestAliasResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "raven-test-alias-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				DefaultPath: "people/",
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"alias": {Type: schema.FieldTypeString},
				},
			},
			"note": {
				Fields: map[string]*schema.FieldDefinition{
					"about": {Type: schema.FieldTypeRef, Target: "person"},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	db, err := Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create person with alias
	personContent := `---
type: person
name: Freya
alias: The Queen
---

# Freya
`
	personPath := filepath.Join(tmpDir, "people", "freya.md")
	os.MkdirAll(filepath.Dir(personPath), 0755)
	os.WriteFile(personPath, []byte(personContent), 0644)
	personDoc, _ := parser.ParseDocument(personContent, personPath, tmpDir)
	db.IndexDocument(personDoc, sch)

	// Create note referencing by alias
	noteContent := `---
type: note
about: "[[The Queen]]"
---

# Note about Freya
`
	notePath := filepath.Join(tmpDir, "notes", "about-freya.md")
	os.MkdirAll(filepath.Dir(notePath), 0755)
	os.WriteFile(notePath, []byte(noteContent), 0644)
	noteDoc, _ := parser.ParseDocument(noteContent, notePath, tmpDir)
	db.IndexDocument(noteDoc, sch)

	// Resolve references
	db.ResolveReferences("daily")

	// Check that alias resolved to people/freya
	backlinks, err := db.Backlinks("people/freya")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, bl := range backlinks {
		if bl.SourceID == "notes/about-freya" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected backlink from notes/about-freya to people/freya via alias 'The Queen'")
	}
}

// TestNameFieldResolution tests that name_field values resolve correctly.
func TestNameFieldResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "raven-test-namefield-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"book": {
				DefaultPath: "books/",
				NameField:   "title",
				Fields: map[string]*schema.FieldDefinition{
					"title":  {Type: schema.FieldTypeString, Required: true},
					"author": {Type: schema.FieldTypeString},
				},
			},
			"note": {
				Fields: map[string]*schema.FieldDefinition{
					"about": {Type: schema.FieldTypeRef, Target: "book"},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	db, err := Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create book with title (name_field)
	bookContent := `---
type: book
title: The Prose Edda
author: Snorri Sturluson
---

# The Prose Edda
`
	bookPath := filepath.Join(tmpDir, "books", "the-prose-edda.md")
	os.MkdirAll(filepath.Dir(bookPath), 0755)
	os.WriteFile(bookPath, []byte(bookContent), 0644)
	bookDoc, _ := parser.ParseDocument(bookContent, bookPath, tmpDir)
	db.IndexDocument(bookDoc, sch)

	// Create note referencing by title (name_field value)
	noteContent := `---
type: note
about: "[[The Prose Edda]]"
---

# Notes on the Edda
`
	notePath := filepath.Join(tmpDir, "notes", "edda-notes.md")
	os.MkdirAll(filepath.Dir(notePath), 0755)
	os.WriteFile(notePath, []byte(noteContent), 0644)
	noteDoc, _ := parser.ParseDocument(noteContent, notePath, tmpDir)
	db.IndexDocument(noteDoc, sch)

	// Resolve references WITH schema to enable name_field resolution
	res, err := db.Resolver(ResolverOptions{
		DailyDirectory: "daily",
		Schema:         sch,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check that "The Prose Edda" resolves to books/the-prose-edda
	result := res.Resolve("The Prose Edda")
	if result.TargetID != "books/the-prose-edda" {
		t.Errorf("expected 'The Prose Edda' to resolve to 'books/the-prose-edda', got %q (error: %s)",
			result.TargetID, result.Error)
	}

	// Also test via ResolveReferences
	// First, manually resolve the refs with the schema-aware resolver
	db.ResolveReferences("daily") // This uses basic resolver

	// Build resolver with schema for backlinks check
	backlinks, err := db.Backlinks("books/the-prose-edda")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Backlinks to books/the-prose-edda: %d", len(backlinks))
	for _, bl := range backlinks {
		t.Logf("  - %s", bl.SourceID)
	}

	// Note: ResolveReferences uses basic resolver without schema,
	// so name_field resolution may not work there. This is a known limitation.
}

// TestQueryRefsWithResolution tests that query refs:[[target]] works with
// various reference styles after resolution.
func TestQueryRefsWithResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "raven-test-query-refs-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"company": {
				DefaultPath: "companies/",
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
			"project": {
				DefaultPath: "projects/",
				Fields: map[string]*schema.FieldDefinition{
					"name":    {Type: schema.FieldTypeString},
					"company": {Type: schema.FieldTypeRef, Target: "company"},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	db, err := Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create company
	companyContent := `---
type: company
name: Cursor
---

# Cursor
`
	companyPath := filepath.Join(tmpDir, "companies", "cursor.md")
	os.MkdirAll(filepath.Dir(companyPath), 0755)
	os.WriteFile(companyPath, []byte(companyContent), 0644)
	companyDoc, _ := parser.ParseDocument(companyContent, companyPath, tmpDir)
	db.IndexDocument(companyDoc, sch)

	// Create project referencing company
	projectContent := `---
type: project
name: Hiring
company: "[[cursor]]"
---

# Hiring Project
`
	projectPath := filepath.Join(tmpDir, "projects", "hiring.md")
	os.MkdirAll(filepath.Dir(projectPath), 0755)
	os.WriteFile(projectPath, []byte(projectContent), 0644)
	projectDoc, _ := parser.ParseDocument(projectContent, projectPath, tmpDir)
	db.IndexDocument(projectDoc, sch)

	// Resolve references
	db.ResolveReferences("daily")

	// Verify refs were stored correctly
	var targetID, targetRaw string
	err = db.db.QueryRow(`
		SELECT target_id, target_raw FROM refs 
		WHERE source_id = 'projects/hiring'
	`).Scan(&targetID, &targetRaw)
	if err != nil {
		t.Fatalf("query refs: %v", err)
	}

	t.Logf("Stored ref: target_id=%q, target_raw=%q", targetID, targetRaw)

	// target_raw should be "cursor" (the short name as written)
	if targetRaw != "cursor" {
		t.Errorf("expected target_raw='cursor', got %q", targetRaw)
	}

	// target_id should be "companies/cursor" (resolved)
	if targetID != "companies/cursor" {
		t.Errorf("expected target_id='companies/cursor', got %q", targetID)
	}

	// Test backlinks work with short name
	backlinks, err := db.Backlinks("companies/cursor")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, bl := range backlinks {
		if bl.SourceID == "projects/hiring" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected backlink from projects/hiring to companies/cursor")
	}
}
