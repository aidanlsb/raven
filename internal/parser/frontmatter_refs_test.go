package parser

import (
	"testing"
)

// TestFrontmatterRefParsing tests all permutations of how refs can be written
// in YAML frontmatter. This is critical for ensuring robust reference handling.
//
// Permutations to support:
// - Quoted vs unquoted
// - With [[]] vs without (bare)
// - Full paths (people/freya)
// - Short names (freya)
// - Names with spaces ("Freya Goddess")
//
// NOTE: Resolution of short names, aliases, and name_field values happens
// at index/resolve time, not at parse time. This test focuses on parsing.
func TestFrontmatterRefParsing(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		fieldName  string
		wantType   string        // "ref", "string", "array", or "null"
		wantTarget string        // for ref type, the expected target
		wantString string        // for string type, the expected value
		wantArray  []interface{} // for array, expected elements (simplified)
	}{
		// === WIKILINK SYNTAX (with [[brackets]]) ===
		{
			name:       "quoted wikilink with full path",
			yaml:       `company: "[[companies/cursor]]"`,
			fieldName:  "company",
			wantType:   "ref",
			wantTarget: "companies/cursor",
		},
		{
			name:       "quoted wikilink with short name",
			yaml:       `company: "[[cursor]]"`,
			fieldName:  "company",
			wantType:   "ref",
			wantTarget: "cursor",
		},
		{
			name:       "quoted wikilink with display text",
			yaml:       `company: "[[companies/cursor|Cursor Inc]]"`,
			fieldName:  "company",
			wantType:   "ref",
			wantTarget: "companies/cursor",
		},
		{
			name:       "quoted wikilink with partial path",
			yaml:       `owner: "[[people/freya]]"`,
			fieldName:  "owner",
			wantType:   "ref",
			wantTarget: "people/freya",
		},
		{
			name:      "unquoted wikilink - YAML parses as array",
			yaml:      `company: [[cursor]]`,
			fieldName: "company",
			wantType:  "array", // YAML interprets [[x]] as nested array
		},

		// === BARE STRING SYNTAX (without [[brackets]]) ===
		{
			name:       "quoted bare short name",
			yaml:       `company: "cursor"`,
			fieldName:  "company",
			wantType:   "string",
			wantString: "cursor",
		},
		{
			name:       "unquoted bare short name",
			yaml:       `company: cursor`,
			fieldName:  "company",
			wantType:   "string",
			wantString: "cursor",
		},
		{
			name:       "quoted bare full path",
			yaml:       `company: "companies/cursor"`,
			fieldName:  "company",
			wantType:   "string",
			wantString: "companies/cursor",
		},
		{
			name:       "unquoted bare full path",
			yaml:       `company: companies/cursor`,
			fieldName:  "company",
			wantType:   "string",
			wantString: "companies/cursor",
		},
		{
			name:       "quoted bare partial path",
			yaml:       `company: "partial/path/object"`,
			fieldName:  "company",
			wantType:   "string",
			wantString: "partial/path/object",
		},
		{
			name:       "quoted name with spaces (potential alias or name_field)",
			yaml:       `book: "The Prose Edda"`,
			fieldName:  "book",
			wantType:   "string",
			wantString: "The Prose Edda",
		},

		// === EDGE CASES ===
		{
			name:       "empty wikilink",
			yaml:       `company: "[[]]"`,
			fieldName:  "company",
			wantType:   "string", // Empty target = not a valid ref
			wantString: "[[]]",
		},
		{
			name:       "wikilink with only spaces",
			yaml:       `company: "[[  ]]"`,
			fieldName:  "company",
			wantType:   "string",
			wantString: "[[  ]]",
		},
		{
			name:       "nested path in wikilink",
			yaml:       `doc: "[[docs/specs/api-v2]]"`,
			fieldName:  "doc",
			wantType:   "ref",
			wantTarget: "docs/specs/api-v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "---\n" + tt.yaml + "\n---\n"
			fm, err := ParseFrontmatter(content)
			if err != nil {
				t.Fatalf("ParseFrontmatter error: %v", err)
			}
			if fm == nil {
				t.Fatal("expected non-nil frontmatter")
			}

			field, ok := fm.Fields[tt.fieldName]
			if !ok {
				t.Fatalf("field %q not found in frontmatter", tt.fieldName)
			}

			switch tt.wantType {
			case "ref":
				target, ok := field.AsRef()
				if !ok {
					t.Errorf("expected ref type, got %T", field)
					return
				}
				if target != tt.wantTarget {
					t.Errorf("ref target = %q, want %q", target, tt.wantTarget)
				}
			case "string":
				s, ok := field.AsString()
				if !ok {
					t.Errorf("expected string type, got %T", field)
					return
				}
				if s != tt.wantString {
					t.Errorf("string value = %q, want %q", s, tt.wantString)
				}
			case "array":
				_, ok := field.AsArray()
				if !ok {
					t.Errorf("expected array type, got %T", field)
				}
			case "null":
				if !field.IsNull() {
					t.Errorf("expected null, got %v", field)
				}
			default:
				t.Fatalf("unknown wantType: %s", tt.wantType)
			}
		})
	}
}

// TestFrontmatterRefArrayParsing tests arrays of refs in frontmatter.
func TestFrontmatterRefArrayParsing(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		fieldName string
		wantRefs  []string // expected ref targets
		wantMixed bool     // true if array contains mixed types
	}{
		{
			name: "array of wikilinks",
			yaml: `owners:
  - "[[people/freya]]"
  - "[[people/thor]]"`,
			fieldName: "owners",
			wantRefs:  []string{"people/freya", "people/thor"},
		},
		{
			name:      "inline array of wikilinks",
			yaml:      `owners: ["[[people/freya]]", "[[people/thor]]"]`,
			fieldName: "owners",
			wantRefs:  []string{"people/freya", "people/thor"},
		},
		{
			name: "array of bare strings",
			yaml: `owners:
  - people/freya
  - people/thor`,
			fieldName: "owners",
			wantRefs:  nil, // These are strings, not refs
			wantMixed: true,
		},
		{
			name: "mixed array - wikilinks and strings",
			yaml: `owners:
  - "[[people/freya]]"
  - people/thor`,
			fieldName: "owners",
			wantRefs:  []string{"people/freya"}, // Only first is a ref
			wantMixed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "---\n" + tt.yaml + "\n---\n"
			fm, err := ParseFrontmatter(content)
			if err != nil {
				t.Fatalf("ParseFrontmatter error: %v", err)
			}
			if fm == nil {
				t.Fatal("expected non-nil frontmatter")
			}

			field, ok := fm.Fields[tt.fieldName]
			if !ok {
				t.Fatalf("field %q not found", tt.fieldName)
			}

			arr, ok := field.AsArray()
			if !ok {
				t.Fatalf("expected array, got %T", field)
			}

			var foundRefs []string
			for _, item := range arr {
				if target, ok := item.AsRef(); ok {
					foundRefs = append(foundRefs, target)
				}
			}

			if len(tt.wantRefs) > 0 {
				if len(foundRefs) != len(tt.wantRefs) {
					t.Errorf("found %d refs, want %d", len(foundRefs), len(tt.wantRefs))
				}
				for i, want := range tt.wantRefs {
					if i < len(foundRefs) && foundRefs[i] != want {
						t.Errorf("ref[%d] = %q, want %q", i, foundRefs[i], want)
					}
				}
			}
		})
	}
}

// TestExtractRefsFromFrontmatter tests that refs are correctly extracted
// from frontmatter for indexing (used by backlinks and query refs:[[target]]).
func TestExtractRefsFromFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantRefs []string // expected TargetRaw values
	}{
		{
			name: "wikilink in frontmatter is extracted",
			content: `---
company: "[[companies/cursor]]"
---

Content`,
			wantRefs: []string{"companies/cursor"},
		},
		{
			name: "multiple wikilinks in frontmatter",
			content: `---
company: "[[companies/cursor]]"
owner: "[[people/freya]]"
---

Content`,
			wantRefs: []string{"companies/cursor", "people/freya"},
		},
		{
			name: "wikilink with display text",
			content: `---
client: "[[clients/midgard|Midgard Corp]]"
---

Content`,
			wantRefs: []string{"clients/midgard"},
		},
		{
			name: "bare string is NOT extracted as ref",
			content: `---
company: companies/cursor
---

Content`,
			wantRefs: nil, // No [[]] = no ref extracted
		},
		{
			name: "quoted bare string is NOT extracted as ref",
			content: `---
company: "companies/cursor"
---

Content`,
			wantRefs: nil, // No [[]] = no ref extracted
		},
		{
			name: "array of wikilinks",
			content: `---
owners:
  - "[[people/freya]]"
  - "[[people/thor]]"
---

Content`,
			wantRefs: []string{"people/freya", "people/thor"},
		},
		{
			name: "wikilink in body is also extracted",
			content: `---
type: project
---

Met with [[people/odin]] about this.`,
			wantRefs: []string{"people/odin"},
		},
		{
			name: "refs in both frontmatter and body",
			content: `---
owner: "[[people/freya]]"
---

Discussed with [[people/thor]].`,
			wantRefs: []string{"people/freya", "people/thor"},
		},
		{
			name: "wikilink embedded in string field is extracted",
			content: `---
type: note
title: My Note
description: "See [[people/freya]] for more info"
---

Content`,
			wantRefs: []string{"people/freya"},
		},
		{
			name: "multiple wikilinks in string field",
			content: `---
type: note
summary: "Meeting with [[people/freya]] and [[people/thor]]"
---

Content`,
			wantRefs: []string{"people/freya", "people/thor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseDocument(tt.content, "test.md", "/vault")
			if err != nil {
				t.Fatalf("ParseDocument error: %v", err)
			}

			var foundRefs []string
			for _, ref := range doc.Refs {
				foundRefs = append(foundRefs, ref.TargetRaw)
			}

			if len(foundRefs) != len(tt.wantRefs) {
				t.Errorf("found %d refs %v, want %d %v", len(foundRefs), foundRefs, len(tt.wantRefs), tt.wantRefs)
				return
			}

			// Check that each expected ref is present (order may vary)
			for _, want := range tt.wantRefs {
				found := false
				for _, got := range foundRefs {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing expected ref %q in %v", want, foundRefs)
				}
			}
		})
	}
}

// TestFrontmatterRefFieldValues tests that ref-typed fields store the correct
// FieldValue type that can be retrieved with AsRef().
func TestFrontmatterRefFieldValues(t *testing.T) {
	content := `---
type: project
company: "[[companies/cursor]]"
owner: "[[people/freya]]"
status: active
priority: high
---

Content`

	fm, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("ParseFrontmatter error: %v", err)
	}

	// company should be a ref
	company := fm.Fields["company"]
	if target, ok := company.AsRef(); !ok || target != "companies/cursor" {
		t.Errorf("company: expected ref 'companies/cursor', got %v (isRef=%v)", company, ok)
	}

	// owner should be a ref
	owner := fm.Fields["owner"]
	if target, ok := owner.AsRef(); !ok || target != "people/freya" {
		t.Errorf("owner: expected ref 'people/freya', got %v (isRef=%v)", owner, ok)
	}

	// status should be a string
	status := fm.Fields["status"]
	if s, ok := status.AsString(); !ok || s != "active" {
		t.Errorf("status: expected string 'active', got %v", status)
	}

	// priority should be a string
	priority := fm.Fields["priority"]
	if s, ok := priority.AsString(); !ok || s != "high" {
		t.Errorf("priority: expected string 'high', got %v", priority)
	}
}

// TestBareStringRefFieldsAtParseTime documents the parser behavior:
// Bare strings (without [[brackets]]) are stored as strings at parse time.
//
// However, at INDEX time (in internal/index/database.go), the schema is used
// to identify ref-typed fields, and bare strings are then treated as refs.
// This means `company: cursor` DOES work when the schema declares `company: ref`.
//
// See TestFrontmatterRefResolution in internal/index/refs_resolution_test.go
// for the complete end-to-end tests including schema-aware ref extraction.
func TestBareStringRefFieldsAtParseTime(t *testing.T) {
	// This tests PARSE behavior - bare strings are stored as strings
	// (schema-aware extraction happens at index time, not parse time)
	tests := []struct {
		yaml      string
		fieldName string
		isRef     bool // at parse time
		value     string
	}{
		// Wikilink syntax → stored as ref at parse time
		{`company: "[[cursor]]"`, "company", true, "cursor"},
		{`company: "[[companies/cursor]]"`, "company", true, "companies/cursor"},

		// Bare string syntax → stored as string at parse time
		// (but extracted as ref at index time when schema says it's a ref field)
		{`company: cursor`, "company", false, "cursor"},
		{`company: "cursor"`, "company", false, "cursor"},
		{`company: companies/cursor`, "company", false, "companies/cursor"},
		{`company: "companies/cursor"`, "company", false, "companies/cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.yaml, func(t *testing.T) {
			content := "---\n" + tt.yaml + "\n---\n"
			fm, err := ParseFrontmatter(content)
			if err != nil {
				t.Fatalf("ParseFrontmatter error: %v", err)
			}

			field := fm.Fields[tt.fieldName]

			if tt.isRef {
				target, ok := field.AsRef()
				if !ok {
					t.Errorf("expected ref, got %T", field)
				} else if target != tt.value {
					t.Errorf("ref target = %q, want %q", target, tt.value)
				}
			} else {
				s, ok := field.AsString()
				if !ok {
					t.Errorf("expected string, got %T", field)
				} else if s != tt.value {
					t.Errorf("string = %q, want %q", s, tt.value)
				}
			}
		})
	}
}
