//go:build integration

package cli_test

import (
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_ReferencesAndBacklinks tests reference resolution and backlinks.
func TestIntegration_ReferencesAndBacklinks(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	// Create a project owned by Alice
	v.RunCLI("new", "project", "Website Redesign", "--field", "status=active", "--field", "owner=[[people/alice]]").MustSucceed(t)

	// Check that Alice has a backlink from the project
	v.AssertBacklinks("people/alice", 1)

	// Query for projects that reference Alice - results are in "items" field
	result := v.RunCLI("query", "type:project refs([[people/alice]])")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

func TestIntegration_RefdIncludesReferencesFromSourceSections(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice
`).
		WithFile("projects/site.md", `---
type: project
title: Site
status: active
---
# Site

## Tasks

Follow up with [[people/alice]].
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	backlinks := v.RunCLI("backlinks", "people/alice").MustSucceed(t)
	backlinks.AssertResultCount(t, "items", 1)

	for _, query := range []string{
		"type:person refd([[projects/site]])",
		"type:person refd(type:project)",
	} {
		t.Run(query, func(t *testing.T) {
			result := v.RunCLI("query", query).MustSucceed(t)
			if got, want := len(result.DataList("items")), len(backlinks.DataList("items")); got != want {
				t.Fatalf("refd result count = %d, backlinks count = %d\nQuery: %s\nRaw: %s", got, want, query, result.RawJSON)
			}
		})
	}
}

func TestIntegration_QueryLinksPredicate(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    default_path: projects/
    name_field: title
    fields:
      title:
        type: string
        required: true
  meeting:
    default_path: meetings/
    name_field: title
    fields:
      title:
        type: string
        required: true
traits:
  todo:
    type: string
`).
		WithFile("projects/site.md", `---
type: project
title: Site
---
# Site

## Resources

- Review [spec](../assets/spec.pdf) @todo(open)
- Visit [vendor](https://example.com)
`).
		WithFile("meetings/sync.md", `---
type: meeting
title: Sync
---
# Sync

![board](../assets/board.png)
`).
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	queries := []string{
		"type:project links(.ext==pdf)",
		"type:meeting links(.is_image==true)",
		"trait:todo links(.ext==pdf)",
		"section .title==Resources links(.scheme==url)",
	}
	for _, queryStr := range queries {
		t.Run(queryStr, func(t *testing.T) {
			result := v.RunCLI("query", queryStr)
			result.MustSucceed(t)
			result.AssertResultCount(t, "items", 1)
		})
	}
}

func TestIntegration_BacklinksOutlinksStdinGroupedOutput(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice
`).
		WithFile("people/bob.md", `---
type: person
name: Bob
---
# Bob
`).
		WithFile("projects/site.md", `---
type: project
title: Site
status: active
owner: people/alice
---
# Site

Owner [[people/alice]] and reviewer [[people/bob]].
`).
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	backlinks := v.RunCLIWithStdin("people/alice\nmissing-person\n", "backlinks", "--stdin")
	backlinks.MustSucceed(t)
	backlinkGroups := backlinks.DataList("items_by_target")
	if len(backlinkGroups) != 1 {
		t.Fatalf("backlink groups = %#v, want 1 group\nRaw: %s", backlinkGroups, backlinks.RawJSON)
	}
	aliceGroup := backlinkGroups[0].(map[string]interface{})
	if aliceGroup["input"] != "people/alice" || aliceGroup["target"] != "people/alice" {
		t.Fatalf("unexpected backlink group: %#v", aliceGroup)
	}
	if items, ok := aliceGroup["items"].([]interface{}); !ok || len(items) != 2 {
		t.Fatalf("backlink items = %#v, want 2", aliceGroup["items"])
	}
	if errors := backlinks.DataList("errors"); len(errors) != 1 {
		t.Fatalf("backlink errors = %#v, want 1", errors)
	}

	outlinks := v.RunCLIWithStdin("projects/site\nmissing-project\n", "outlinks", "--stdin")
	outlinks.MustSucceed(t)
	outlinkGroups := outlinks.DataList("items_by_source")
	if len(outlinkGroups) != 1 {
		t.Fatalf("outlink groups = %#v, want 1 group\nRaw: %s", outlinkGroups, outlinks.RawJSON)
	}
	siteGroup := outlinkGroups[0].(map[string]interface{})
	if siteGroup["input"] != "projects/site" || siteGroup["source"] != "projects/site" {
		t.Fatalf("unexpected outlink group: %#v", siteGroup)
	}
	if items, ok := siteGroup["items"].([]interface{}); !ok || len(items) != 3 {
		t.Fatalf("outlink items = %#v, want 3", siteGroup["items"])
	}
	if errors := outlinks.DataList("errors"); len(errors) != 1 {
		t.Fatalf("outlink errors = %#v, want 1", errors)
	}

	emptyBacklinks := v.RunCLI("backlinks", "projects/site")
	emptyBacklinks.MustSucceed(t)
	if items, ok := emptyBacklinks.Data["items"].([]interface{}); !ok || len(items) != 0 {
		t.Fatalf("empty backlinks items = %#v, want []", emptyBacklinks.Data["items"])
	}

	emptyOutlinks := v.RunCLI("outlinks", "people/alice")
	emptyOutlinks.MustSucceed(t)
	if items, ok := emptyOutlinks.Data["items"].([]interface{}); !ok || len(items) != 0 {
		t.Fatalf("empty outlinks items = %#v, want []", emptyOutlinks.Data["items"])
	}
}

// TestIntegration_BacklinksOutlinksDynamicDates tests that backlinks and outlinks
// resolve dynamic date keywords like "today" and "yesterday".
func TestIntegration_BacklinksOutlinksDynamicDates(t *testing.T) {
	t.Parallel()
	today := time.Now().Format("2006-01-02")

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("directories:\n  daily: daily/\n").
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice
`).
		WithFile("daily/"+today+".md", `---
type: page
---
# Daily Note

Met with [[people/alice]] today.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	t.Run("backlinks with dynamic date today", func(t *testing.T) {
		// "today" should resolve to daily/<today> and alice should have a backlink from it
		result := v.RunCLI("backlinks", "alice")
		result.MustSucceed(t)
		result.AssertResultCount(t, "items", 1)

		// Now test that "today" resolves as a target for backlinks
		result = v.RunCLI("backlinks", "today")
		result.MustSucceed(t)

		if result.DataString("target") != today {
			t.Errorf("expected target %q, got %q", today, result.DataString("target"))
		}
	})

	t.Run("outlinks with dynamic date today", func(t *testing.T) {
		result := v.RunCLI("outlinks", "today")
		result.MustSucceed(t)

		if result.DataString("source") != today {
			t.Errorf("expected source %q, got %q", today, result.DataString("source"))
		}
		result.AssertResultCount(t, "items", 1)
	})
}
