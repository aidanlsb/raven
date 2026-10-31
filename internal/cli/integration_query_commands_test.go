//go:build integration

package cli_test

import (
	"database/sql"
	"os"
	"os/exec"
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

func TestIntegration_QuerySort(t *testing.T) {
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
      status:
        type: enum
        values: [active, paused, done]
      count:
        type: number
      tags:
        type: string[]
traits:
  due:
    type: date
`).
		Build()

	v.RunCLI("new", "project", "Zebra", "--field", "status=paused", "--field", "count=10").MustSucceed(t)
	v.RunCLI("new", "project", "Alpha", "--field", "status=active", "--field", "count=2").MustSucceed(t)
	v.RunCLI("new", "project", "Beta", "--field", "status=active").MustSucceed(t)

	byTitle := v.RunCLI("query", "type:project sort(.title)").MustSucceed(t)
	if got := itemIDs(t, byTitle); !equalStringSlice(got, []string{"projects/alpha", "projects/beta", "projects/zebra"}) {
		t.Fatalf("sort(.title) ids = %v", got)
	}

	byTitleDesc := v.RunCLI("query", "type:project sort(.title, desc)").MustSucceed(t)
	if got := itemIDs(t, byTitleDesc); !equalStringSlice(got, []string{"projects/zebra", "projects/beta", "projects/alpha"}) {
		t.Fatalf("sort(.title, desc) ids = %v", got)
	}

	numeric := v.RunCLI("query", "type:project sort(.count, asc)").MustSucceed(t)
	got := itemIDs(t, numeric)
	if len(got) != 3 || got[0] != "projects/alpha" || got[1] != "projects/zebra" || got[2] != "projects/beta" {
		t.Fatalf("sort(.count) should be 2, 10, then missing last, got %v", got)
	}

	numericDesc := v.RunCLI("query", "type:project sort(.count, desc)").MustSucceed(t)
	got = itemIDs(t, numericDesc)
	if got[len(got)-1] != "projects/beta" {
		t.Fatalf("missing count should sort last in desc, got %v", got)
	}

	multi := v.RunCLI("query", "type:project sort(.status, desc) sort(.title, asc)").MustSucceed(t)
	if got := itemIDs(t, multi); !equalStringSlice(got, []string{"projects/zebra", "projects/alpha", "projects/beta"}) {
		t.Fatalf("multi-field sort ids = %v", got)
	}

	page := v.RunCLI("query", "type:project sort(.title)", "--limit", "1", "--offset", "1").MustSucceed(t)
	if got := itemIDs(t, page); !equalStringSlice(got, []string{"projects/beta"}) {
		t.Fatalf("paged sort ids = %v", got)
	}
	if got := page.Data["has_more"]; got != true {
		t.Fatalf("paged has_more = %#v, want true", got)
	}

	ids := v.RunCLI("query", "type:project sort(.title, desc)", "--ids", "--limit", "2").MustSucceed(t)
	idList := stringList(t, ids, "ids")
	if !equalStringSlice(idList, []string{"projects/zebra", "projects/beta"}) {
		t.Fatalf("--ids sorted page = %v", idList)
	}

	arrayField := v.RunCLI("query", "type:project sort(.tags)")
	arrayField.MustFail(t, "QUERY_INVALID")
	arrayField.MustFailWithMessage(t, "cannot sort by array field '.tags'")

	nested := v.RunCLI("query", "type:project has(trait:due sort(.value))")
	nested.MustFail(t, "QUERY_INVALID")
	nested.MustFailWithMessage(t, "sort() is only allowed at the top level")
}

func itemIDs(t *testing.T, result *testutil.CLIResult) []string {
	t.Helper()
	items := result.DataList("items")
	ids := make([]string, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("item = %#v, want object", item)
		}
		id, _ := row["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func stringList(t *testing.T, result *testutil.CLIResult, key string) []string {
	t.Helper()
	list := result.DataList(key)
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("%s entry = %#v, want string", key, v)
		}
		out = append(out, s)
	}
	return out
}

func equalStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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

func TestIntegration_QueryAutoRefreshPicksUpHandWrittenEditsAndDeletes(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("reindex").MustSucceed(t)
	v.AssertQueryCount("type:person", 0)

	freyaPath := filepath.Join(v.Path, "people", "freya.md")
	if err := os.MkdirAll(filepath.Dir(freyaPath), 0o755); err != nil {
		t.Fatalf("mkdir people: %v", err)
	}
	if err := os.WriteFile(freyaPath, []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("write freya: %v", err)
	}

	created := v.RunCLI("query", "type:person")
	created.MustSucceed(t)
	created.AssertResultCount(t, "items", 1)

	if err := os.WriteFile(freyaPath, []byte("---\ntype: person\nname: Frigg\n---\n"), 0o644); err != nil {
		t.Fatalf("edit freya: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(freyaPath, future, future); err != nil {
		t.Fatalf("bump freya mtime: %v", err)
	}

	edited := v.RunCLI("query", "type:person")
	edited.MustSucceed(t)
	edited.AssertResultCount(t, "items", 1)
	item, ok := edited.DataList("items")[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected query item map, got %#v", edited.DataList("items")[0])
	}
	fields, ok := item["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map, got %#v", item["fields"])
	}
	if got := fields["name"]; got != "Frigg" {
		t.Fatalf("expected refreshed name Frigg, got %#v", got)
	}

	skipped := v.RunCLI("query", "type:person", "--no-refresh")
	skipped.MustSucceed(t)
	skipped.AssertResultCount(t, "items", 1)

	if err := os.Remove(freyaPath); err != nil {
		t.Fatalf("remove freya: %v", err)
	}
	stale := v.RunCLI("query", "type:person", "--no-refresh")
	stale.MustSucceed(t)
	stale.AssertResultCount(t, "items", 1)

	deleted := v.RunCLI("query", "type:person")
	deleted.MustSucceed(t)
	deleted.AssertResultCount(t, "items", 0)
}

func TestIntegration_QueryEmptyResultsHumanJSONPipeAndIDs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	jsonResult := v.RunCLI("query", "type:person")
	jsonResult.MustSucceed(t)
	jsonResult.AssertResultCount(t, "items", 0)
	if !strings.Contains(jsonResult.RawJSON, `"items"`) {
		t.Fatalf("JSON empty query should keep an items list, got %s", jsonResult.RawJSON)
	}

	binary := testutil.BuildCLI(t)
	human := exec.Command(binary, "--vault-path", v.Path, "query", "type:person", "--no-pipe")
	humanOut, err := human.CombinedOutput()
	if err != nil {
		t.Fatalf("human query failed: %v\noutput=%s", err, humanOut)
	}
	if got := strings.TrimSpace(string(humanOut)); got != "No results" {
		t.Fatalf("human empty query = %q, want %q", got, "No results")
	}

	pipe := exec.Command(binary, "--vault-path", v.Path, "query", "type:person", "--pipe")
	pipeOut, err := pipe.CombinedOutput()
	if err != nil {
		t.Fatalf("pipe query failed: %v\noutput=%s", err, pipeOut)
	}
	if got := strings.TrimSpace(string(pipeOut)); got != "" {
		t.Fatalf("pipe empty query = %q, want empty", got)
	}

	ids := exec.Command(binary, "--vault-path", v.Path, "query", "type:person", "--ids")
	idsOut, err := ids.CombinedOutput()
	if err != nil {
		t.Fatalf("ids query failed: %v\noutput=%s", err, idsOut)
	}
	if got := strings.TrimSpace(string(idsOut)); got != "" {
		t.Fatalf("ids empty query = %q, want empty", got)
	}

	search := exec.Command(binary, "--vault-path", v.Path, "search", "zzzz-no-such-term")
	searchOut, err := search.CombinedOutput()
	if err != nil {
		t.Fatalf("human search failed: %v\noutput=%s", err, searchOut)
	}
	if got := strings.TrimSpace(string(searchOut)); got != "No results" {
		t.Fatalf("human empty search = %q, want %q", got, "No results")
	}
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

	skipped := v.RunCLI("query", "type:project", "--no-refresh")
	skipped.MustFail(t, "DATABASE_VERSION_MISMATCH")
	skipped.MustFailWithMessage(t, "rvn reindex --full")

	searchSkipped := v.RunCLI("search", "Stale Index", "--no-refresh")
	searchSkipped.MustFail(t, "DATABASE_VERSION_MISMATCH")
	searchSkipped.MustFailWithMessage(t, "rvn reindex --full")

	result := v.RunCLI("query", "type:project")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)

	searchResult := v.RunCLI("search", "Stale Index")
	searchResult.MustSucceed(t)
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

func TestIntegration_QueryFTSPunctuationAndDecimals(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  note:
    default_path: notes/
    name_field: title
    fields:
      title:
        type: string
        required: true
      rating:
        type: number
      category:
        type: string
`).
		WithFile("notes/search.md", `---
type: note
title: Search Notes
rating: 4.8
category: error
---
Look at type:project and https://github.com plus -todo, foo (bar), and @todo.
`).
		WithFile("notes/other.md", `---
type: note
title: Other Notes
rating: 4.1
---
Plain prose.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	for _, q := range []string{
		`type:note content("type:project")`,
		`type:note content("https://github.com")`,
		`type:note content("-todo")`,
		`type:note content("foo (bar)")`,
		`type:note content("@todo")`,
	} {
		result := v.RunCLI("query", q)
		result.MustSucceed(t)
		result.AssertResultCount(t, "items", 1)
	}

	search := v.RunCLI("search", "@todo")
	search.MustSucceed(t)

	decimal := v.RunCLI("query", "type:note .rating > 4.5")
	decimal.MustSucceed(t)
	decimal.AssertResultCount(t, "items", 1)

	matching := v.RunCLI("query", "type:note .category == error")
	matching.MustSucceed(t)
	negated := v.RunCLI("query", "type:note !(.category == error)")
	negated.MustSucceed(t)
	all := v.RunCLI("query", "type:note")
	all.MustSucceed(t)
	if matching.Data["total"].(float64)+negated.Data["total"].(float64) != all.Data["total"].(float64) {
		t.Fatalf("P + !P != all: matching=%v negated=%v all=%v", matching.Data["total"], negated.Data["total"], all.Data["total"])
	}

	legacy := v.RunCLI("query", "section within(object:project)")
	legacy.MustFail(t, "QUERY_INVALID")
	legacy.MustFailWithMessage(t, "legacy 'object:' queries are no longer supported")

	refdLink := v.RunCLI("query", "type:note refd(link .ext==pdf)")
	refdLink.MustFail(t, "QUERY_INVALID")
	refdLink.MustFailWithMessage(t, "refd() only supports type:..., trait:..., or section subqueries")
}
