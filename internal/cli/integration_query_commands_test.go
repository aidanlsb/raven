//go:build integration

package cli_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_QueryByField tests querying objects by field values.
func TestIntegration_QueryByField(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects with different statuses
	v.RunCLI("new", "project", "Project Alpha", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project Beta", "--field", "status=paused").MustSucceed(t)
	v.RunCLI("new", "project", "Project Gamma", "--field", "status=active").MustSucceed(t)

	// Query for active projects - uses == for equality
	result := v.RunCLI("query", "type:project .status==active")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 2)

	// Query for paused projects
	result = v.RunCLI("query", "type:project .status==paused")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

// TestIntegration_QueryPaging verifies the agent-friendly paging fields
// (has_more / next_offset) in the query JSON envelope.
func TestIntegration_QueryPaging(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Project Alpha", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project Beta", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project Gamma", "--field", "status=active").MustSucceed(t)

	// Unlimited (default): full result set, no more pages.
	unlimited := v.RunCLI("query", "type:project .status==active").MustSucceed(t)
	unlimited.AssertResultCount(t, "items", 3)
	if got := unlimited.Data["total"]; got != float64(3) {
		t.Fatalf("unlimited total = %#v, want 3", got)
	}
	if got, ok := unlimited.Data["has_more"]; !ok || got != false {
		t.Fatalf("unlimited has_more = %#v (present=%v), want false", got, ok)
	}
	if _, ok := unlimited.Data["next_offset"]; ok {
		t.Fatalf("unlimited should not include next_offset, got %#v", unlimited.Data["next_offset"])
	}

	// First page with more results available.
	first := v.RunCLI("query", "type:project .status==active", "--limit", "2", "--offset", "0").MustSucceed(t)
	first.AssertResultCount(t, "items", 2)
	if got := first.Data["total"]; got != float64(3) {
		t.Fatalf("first page total = %#v, want 3", got)
	}
	if got, ok := first.Data["has_more"]; !ok || got != true {
		t.Fatalf("first page has_more = %#v (present=%v), want true", got, ok)
	}
	if got := first.Data["next_offset"]; got != float64(2) {
		t.Fatalf("first page next_offset = %#v, want 2", got)
	}

	// Last page: no more results, next_offset omitted.
	last := v.RunCLI("query", "type:project .status==active", "--limit", "2", "--offset", "2").MustSucceed(t)
	last.AssertResultCount(t, "items", 1)
	if got, ok := last.Data["has_more"]; !ok || got != false {
		t.Fatalf("last page has_more = %#v (present=%v), want false", got, ok)
	}
	if _, ok := last.Data["next_offset"]; ok {
		t.Fatalf("last page should not include next_offset, got %#v", last.Data["next_offset"])
	}

	// --ids responses carry the same paging fields.
	ids := v.RunCLI("query", "type:project .status==active", "--ids", "--limit", "2", "--offset", "0").MustSucceed(t)
	if got, ok := ids.Data["has_more"]; !ok || got != true {
		t.Fatalf("ids has_more = %#v (present=%v), want true", got, ok)
	}
	if got := ids.Data["next_offset"]; got != float64(2) {
		t.Fatalf("ids next_offset = %#v, want 2", got)
	}
}

func TestIntegration_SavedQueryIgnoresLegacyRuntimeOptions(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  active-projects:
    query: "type:project .status==active"
    options:
      apply: ["set status=done"]
      confirm: true
      ids: true
      limit: 1
`).
		Build()

	v.RunCLI("new", "project", "Project Alpha", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project Beta", "--field", "status=active").MustSucceed(t)

	// Legacy persisted runtime options are ignored: invoking the saved query
	// returns both rows and does not apply the stored mutation.
	result := v.RunCLI("query", "active-projects").MustSucceed(t)
	result.AssertResultCount(t, "items", 2)
	v.RunCLI("query", "type:project .status==done").MustSucceed(t).AssertResultCount(t, "items", 0)

	// Runtime behavior belongs on the saved-query invocation.
	v.RunCLI("query", "active-projects", "--apply", "set status=done", "--confirm").MustSucceed(t)
	v.RunCLI("query", "type:project .status==done").MustSucceed(t).AssertResultCount(t, "items", 2)
}

func TestIntegration_QueryErrorsSuggestCorrectSyntax(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	t.Run("single quoted strings", func(t *testing.T) {
		result := v.RunCLI("query", `type:project .status=='active'`)
		result.MustFail(t, "QUERY_INVALID")
		if result.Error == nil || !strings.Contains(result.Error.Suggestion, "double quotes") {
			t.Fatalf("expected double-quote suggestion, got %#v", result.Error)
		}
	})

	t.Run("bare schema type", func(t *testing.T) {
		result := v.RunCLI("query", "project")
		result.MustFail(t, "QUERY_INVALID")
		if result.Error == nil || !strings.Contains(result.Error.Suggestion, "rvn query type:project") {
			t.Fatalf("expected type query suggestion, got %#v", result.Error)
		}
		if !strings.Contains(result.Error.Suggestion, "section") {
			t.Fatalf("expected all query roots in suggestion, got %#v", result.Error)
		}
	})

	for _, tt := range []struct {
		name    string
		query   string
		message string
	}{
		{
			name:    "nonnumeric link line",
			query:   "link .line==abc",
			message: "link field '.line' requires a numeric value",
		},
		{
			name:    "nonnumeric section line",
			query:   "section .line_start==abc",
			message: "section field '.line_start' requires a numeric value",
		},
		{
			name:    "phantom section type root",
			query:   "type:section .title==Tasks",
			message: "type:section is not a valid query root",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := v.RunCLI("query", tt.query)
			result.MustFail(t, "QUERY_INVALID")
			result.MustFailWithMessage(t, tt.message)
			if result.Error != nil && strings.Contains(result.Error.Suggestion, "reindex") {
				t.Fatalf("query validation error must not suggest reindexing: %#v", result.Error)
			}
		})
	}
}

func TestIntegration_LinkQuery(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/raven.md", `---
type: project
title: Raven
status: active
---
# Raven

[Brief](docs/brief.pdf)

## Files

[Plan](../plan.pdf)
![Diagram](images/diagram.png)
`).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice

[Resume](resume.pdf)
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	pdfs := v.RunCLI("query", "link .ext==pdf within(type:project)")
	pdfs.MustSucceed(t)
	if got := pdfs.Data["query_kind"]; got != "link" {
		t.Fatalf("query_kind = %#v, want link", got)
	}
	pdfs.AssertResultCount(t, "items", 2)
	for _, raw := range pdfs.DataList("items") {
		item := raw.(map[string]interface{})
		if item["source_id"] != "projects/raven" || item["source_type"] != "project" {
			t.Fatalf("unexpected link source: %#v", item)
		}
		for _, field := range []string{
			"file_path", "line", "position_start", "position_end", "raw_target",
			"display", "is_image", "scheme", "ext", "normalized_key",
		} {
			if _, ok := item[field]; !ok {
				t.Fatalf("link item missing %q: %#v", field, item)
			}
		}
	}

	sectionPDF := v.RunCLI("query", "link .ext==pdf within(section .title==Files)")
	sectionPDF.MustSucceed(t)
	sectionPDF.AssertResultCount(t, "items", 1)
	if got := sectionPDF.DataList("items")[0].(map[string]interface{})["raw_target"]; got != "../plan.pdf" {
		t.Fatalf("section-scoped target = %#v, want ../plan.pdf", got)
	}

	image := v.RunCLI("query", `link .is_image==true includes(.display, "gram")`)
	image.MustSucceed(t)
	image.AssertResultCount(t, "items", 1)

	ids := v.RunCLI("query", "link .ext==pdf within(type:project)", "--ids")
	ids.MustSucceed(t)
	gotIDs := ids.DataList("ids")
	if len(gotIDs) != 2 || gotIDs[0] != "projects/raven" || gotIDs[1] != "projects/raven" {
		t.Fatalf("source IDs = %#v, want one projects/raven projection per edge", gotIDs)
	}

	page := v.RunCLI("query", "link .scheme==file", "--limit", "1")
	page.MustSucceed(t)
	page.AssertResultCount(t, "items", 1)
	if got := page.Data["total"]; got != float64(4) {
		t.Fatalf("total = %#v, want 4", got)
	}
	if got := page.Data["has_more"]; got != true {
		t.Fatalf("has_more = %#v, want true", got)
	}

	apply := v.RunCLI("query", "link", "--apply", "delete")
	apply.MustFail(t, "INVALID_INPUT")
	apply.MustFailWithMessage(t, "--apply is not supported for link queries")

	invalid := v.RunCLI("query", "link refs([[projects/raven]])")
	invalid.MustFail(t, "QUERY_INVALID")
	invalid.MustFailWithMessage(t, "refs() predicate is not valid for link queries")
}

func TestIntegration_QueryRefreshRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  project:
    default_path: projects/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: enum
        values: [active, done]
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("objects/projects/weekly.md", `---
type: project
title: Weekly
status: active
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	updated := `---
type: project
title: Weekly
status: done
---
`
	filePath := filepath.Join(v.Path, "objects/projects/weekly.md")
	if err := os.WriteFile(filePath, []byte(updated), 0o644); err != nil {
		t.Fatalf("failed to update project file: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filePath, future, future); err != nil {
		t.Fatalf("failed to bump project mtime: %v", err)
	}

	result := v.RunCLI("query", "type:project", "--refresh")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)

	item, ok := result.DataList("items")[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected query item map, got %#v", result.DataList("items")[0])
	}
	if got := item["id"]; got != "projects/weekly" {
		t.Fatalf("expected refreshed project ID projects/weekly, got %#v", got)
	}
	fields, ok := item["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map, got %#v", item["fields"])
	}
	if got := fields["status"]; got != "done" {
		t.Fatalf("expected refreshed status done, got %#v", got)
	}
}

func TestIntegration_QueryRefreshRemovesDeletedFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)
	v.AssertQueryCount("type:person", 1)

	if err := os.Remove(filepath.Join(v.Path, "people/alice.md")); err != nil {
		t.Fatalf("failed to remove person file: %v", err)
	}

	result := v.RunCLI("query", "type:person", "--refresh")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 0)
}

func TestIntegration_QueryFailsOnSchemaLoadError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Schema Query").MustSucceed(t)

	schemaPath := filepath.Join(v.Path, "schema.yaml")
	if err := os.WriteFile(schemaPath, []byte("version: ["), 0o644); err != nil {
		t.Fatalf("failed to corrupt schema for test: %v", err)
	}

	result := v.RunCLI("query", "type:project")
	result.MustFail(t, "SCHEMA_INVALID")
	result.MustFailWithMessage(t, "Fix schema.yaml and try again")
}

func TestIntegration_IndexReadsFailOnStaleIndexSchema(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Stale Index").MustSucceed(t)
	v.RunCLI("reindex").MustSucceed(t)

	db, err := sql.Open("sqlite", filepath.Join(v.Path, ".raven", "index.db"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if _, err := db.Exec(`UPDATE meta SET value = ? WHERE key = 'version'`, strconv.Itoa(indexschema.CurrentDBVersion-1)); err != nil {
		db.Close()
		t.Fatalf("downgrade index version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	result := v.RunCLI("query", "type:project")
	result.MustFail(t, "DATABASE_VERSION_MISMATCH")
	result.MustFailWithMessage(t, "rvn reindex --full")

	searchResult := v.RunCLI("search", "Stale Index")
	searchResult.MustFail(t, "DATABASE_VERSION_MISMATCH")
	searchResult.MustFailWithMessage(t, "rvn reindex --full")
}

func TestIntegration_QueryAmbiguousReferenceReturnsQueryInvalid(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Alex").MustSucceed(t)
	v.RunCLI("new", "person", "Alex").MustSucceed(t)

	result := v.RunCLI("query", "type:project refs([[alex]])")
	result.MustFail(t, "QUERY_INVALID")
	result.MustFailWithMessage(t, "Use a full object ID/path to disambiguate")
}
