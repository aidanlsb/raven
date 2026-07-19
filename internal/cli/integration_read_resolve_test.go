//go:build integration

package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_DuplicateObjectError tests that creating a duplicate object fails.
func TestIntegration_DuplicateObjectError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	// Try to create the same person again
	result := v.RunCLI("new", "person", "Alice")
	result.MustFail(t, "FILE_EXISTS")
}

// TestIntegration_ReadObject tests reading object content.
func TestIntegration_ReadObject(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/bob.md", `---
type: person
name: Bob
email: bob@example.com
---
# Bob

Bob is a software engineer.
`).
		Build()

	// Reindex
	v.RunCLI("reindex").MustSucceed(t)

	// Read the object
	result := v.RunCLI("read", "people/bob")
	result.MustSucceed(t)

	// Verify we got the content back
	content := result.DataString("content")
	if content == "" {
		t.Errorf("expected content in read result, got empty string")
	}
}

// TestIntegration_ReadSections tests the --sections outline view.
func TestIntegration_ReadSections(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", `---
type: project
title: Site
status: active
---

## Tasks

- a task

### Backlog

- later

## Notes

text
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("read", "projects/site", "--sections")
	result.MustSucceed(t)

	sections, ok := result.Data["sections"].([]interface{})
	if !ok {
		t.Fatalf("sections = %#v, want array; raw: %s", result.Data["sections"], result.RawJSON)
	}
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d; raw: %s", len(sections), result.RawJSON)
	}
	first, _ := sections[0].(map[string]interface{})
	if first["id"] != "projects/site#tasks" || first["title"] != "Tasks" {
		t.Fatalf("first section = %#v, want tasks", first)
	}
	second, _ := sections[1].(map[string]interface{})
	if second["parent_section_id"] != "projects/site#tasks" {
		t.Fatalf("second section = %#v, want backlog under tasks", second)
	}

	// Scoped outline for a section reference.
	scoped := v.RunCLI("read", "projects/site#tasks", "--sections")
	scoped.MustSucceed(t)
	scopedSections, _ := scoped.Data["sections"].([]interface{})
	if len(scopedSections) != 2 {
		t.Fatalf("expected 2 scoped sections (tasks, backlog), got %d; raw: %s", len(scopedSections), scoped.RawJSON)
	}
}

func TestIntegration_ReadSupportsDynamicDateReferences(t *testing.T) {
	t.Parallel()

	today := time.Now().Format("2006-01-02")
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("today.md", "# Literal Today\n").
		WithFile("daily/"+today+".md", `---
type: page
---
# Daily Today
First line
Second line
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("read", "today")
	result.MustSucceed(t)
	if got := result.DataString("path"); got != "daily/"+today+".md" {
		t.Fatalf("path = %q, want %q", got, "daily/"+today+".md")
	}
	content := result.DataString("content")
	if !strings.Contains(content, "# Daily Today") {
		t.Fatalf("expected dynamic daily content, got:\n%s", content)
	}
	if strings.Contains(content, "# Literal Today") {
		t.Fatalf("expected read today to prefer the daily note, got:\n%s", content)
	}

	raw := v.RunCLI("read", "today", "--raw", "--start-line", "4", "--end-line", "5")
	raw.MustSucceed(t)
	if got := raw.DataString("path"); got != "daily/"+today+".md" {
		t.Fatalf("raw path = %q, want %q", got, "daily/"+today+".md")
	}
	rawContent := raw.DataString("content")
	if !strings.Contains(rawContent, "# Daily Today") {
		t.Fatalf("expected ranged read to use dynamic daily target, got:\n%s", rawContent)
	}
}

func TestIntegration_ISODateRefsResolveToDailyIdentity(t *testing.T) {
	t.Parallel()

	// A bare ISO date is unambiguous date identity: it always resolves to the
	// canonical bare-date object ID of the daily note, which lives under the
	// configured daily directory. Legacy daily-directory-prefixed references
	// resolve to the same object as a compatibility alias.
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2025-02-01.md", `# Daily ISO Note
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	resolve := v.RunCLI("resolve", "2025-02-01")
	resolve.MustSucceed(t)
	if resolve.Data["ambiguous"] == true {
		t.Fatalf("expected bare ISO date to resolve unambiguously, got %#v", resolve.Data)
	}
	if got := resolve.DataString("object_id"); got != "2025-02-01" {
		t.Fatalf("resolve object_id = %q, want %q", got, "2025-02-01")
	}

	// The legacy prefixed form is a compatibility alias for the same object.
	legacy := v.RunCLI("resolve", "daily/2025-02-01")
	legacy.MustSucceed(t)
	if got := legacy.DataString("object_id"); got != "2025-02-01" {
		t.Fatalf("legacy resolve object_id = %q, want %q", got, "2025-02-01")
	}

	read := v.RunCLI("read", "2025-02-01")
	read.MustSucceed(t)
	if !strings.Contains(read.DataString("content"), "# Daily ISO Note") {
		t.Fatalf("expected read to return the daily note, got:\n%s", read.DataString("content"))
	}
}

func TestIntegration_ReadWithoutArgSuggestsUsage(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("read")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn read <reference>")
}

func TestIntegration_OpenWithoutArgSuggestsUsage(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("open")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn open <reference>")
}

func TestIntegration_SearchWithoutArgSuggestsUsage(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("search")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn search <query>")
}

func TestIntegration_OpenAmbiguousReferenceReturnsMatches(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---
# Freya
`).
		WithFile("people/freya-2.md", `---
type: person
name: Freya
---
# Freya Two
`).
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("open", "Freya")
	result.MustFail(t, "REF_AMBIGUOUS")
	matches, ok := result.Error.Details["matches"].([]interface{})
	if !ok {
		t.Fatalf("expected ambiguous match details, got %#v", result.Error.Details["matches"])
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 ambiguous matches, got %#v", matches)
	}
}

// TestIntegration_Resolve tests the resolve command.
func TestIntegration_Resolve(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---
# Freya
`).
		WithFile("people/thor.md", `---
type: person
name: Thor
---
# Thor
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	t.Run("resolve by literal path", func(t *testing.T) {
		result := v.RunCLI("resolve", "people/freya")
		result.MustSucceed(t)

		if result.DataString("object_id") != "people/freya" {
			t.Errorf("expected object_id 'people/freya', got %q", result.DataString("object_id"))
		}
		if result.Data["resolved"] != true {
			t.Errorf("expected resolved=true")
		}
		if result.DataString("type") != "person" {
			t.Errorf("expected type 'person', got %q", result.DataString("type"))
		}
		if result.DataString("match_source") != "literal_path" {
			t.Errorf("expected match_source 'literal_path', got %q", result.DataString("match_source"))
		}
	})

	t.Run("resolve by short name", func(t *testing.T) {
		result := v.RunCLI("resolve", "thor")
		result.MustSucceed(t)

		if result.Data["resolved"] != true {
			t.Errorf("expected resolved=true")
		}
		if result.DataString("object_id") != "people/thor" {
			t.Errorf("expected object_id 'people/thor', got %q", result.DataString("object_id"))
		}
	})

	t.Run("resolve not found", func(t *testing.T) {
		result := v.RunCLI("resolve", "nonexistent")
		result.MustSucceed(t)

		if result.Data["resolved"] != false {
			t.Errorf("expected resolved=false for not-found ref")
		}
	})

	t.Run("resolve with .md extension", func(t *testing.T) {
		result := v.RunCLI("resolve", "people/freya.md")
		result.MustSucceed(t)

		if result.Data["resolved"] != true {
			t.Errorf("expected resolved=true")
		}
		if result.DataString("object_id") != "people/freya" {
			t.Errorf("expected object_id 'people/freya', got %q", result.DataString("object_id"))
		}
	})
}

// TestIntegration_BareRefPageVsTypedIsAmbiguous locks the bare-reference resolve
// policy at the CLI surface: a bare name that matches both an untyped page (a
// vault-root file with a bare object ID) and a typed object is ambiguous — the
// on-disk page is never silently preferred over the typed object. The candidate
// matches are surfaced in the error details so the interactive picker can offer
// disambiguation.
func TestIntegration_BareRefPageVsTypedIsAmbiguous(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("freya.md", `---
type: page
---
# Freya (untyped page)
`).
		WithFile("people/freya.md", `---
type: person
name: Freya
---
# Freya (person)
`).
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	t.Run("resolve reports ambiguity without a winner", func(t *testing.T) {
		result := v.RunCLI("resolve", "freya")
		result.MustSucceed(t)
		if result.Data["resolved"] != false {
			t.Fatalf("expected resolved=false, got %#v", result.Data["resolved"])
		}
		if result.Data["ambiguous"] != true {
			t.Fatalf("expected ambiguous=true, got %#v", result.Data["ambiguous"])
		}
		items := result.DataList("items")
		if len(items) != 2 {
			t.Fatalf("expected 2 matches, got %#v", items)
		}
		ids := make(map[string]bool, len(items))
		for _, raw := range items {
			match, ok := raw.(map[string]interface{})
			if !ok {
				t.Fatalf("expected match object, got %#v", raw)
			}
			id, _ := match["object_id"].(string)
			ids[id] = true
		}
		if !ids["freya"] || !ids["people/freya"] {
			t.Fatalf("expected page and typed matches, got %#v", items)
		}
	})

	t.Run("read surfaces matches for the disambiguation picker", func(t *testing.T) {
		result := v.RunCLI("read", "freya")
		result.MustFail(t, "REF_AMBIGUOUS")
		matches, ok := result.Error.Details["matches"].([]interface{})
		if !ok {
			t.Fatalf("expected match details, got %#v", result.Error.Details["matches"])
		}
		if len(matches) != 2 {
			t.Fatalf("expected 2 ambiguous matches, got %#v", matches)
		}
	})

	t.Run("qualified typed reference stays unambiguous", func(t *testing.T) {
		result := v.RunCLI("resolve", "people/freya")
		result.MustSucceed(t)
		if result.Data["resolved"] != true {
			t.Fatalf("expected resolved=true, got %#v", result.Data["resolved"])
		}
		if result.DataString("object_id") != "people/freya" {
			t.Fatalf("expected object_id 'people/freya', got %q", result.DataString("object_id"))
		}
	})
}
