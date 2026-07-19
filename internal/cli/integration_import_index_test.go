//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_PrefixedDailyRefResolvesAndDailyCreatesUnderDailyDir(t *testing.T) {
	t.Parallel()
	// A legacy daily-directory-prefixed reference resolves to the bare-date object
	// ID as a compatibility alias, so it is never reported as a missing reference.
	// The canonical way to materialize a daily note remains `rvn daily`, which
	// creates the file under the configured daily directory.
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  type: objects/
  daily: journal/
`).
		WithFile("notes/mention.md", `See [[journal/2026-06-30]] and [[2026-06-30]] for details.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// Both the prefixed compat form and the bare form resolve to the bare-date ID.
	for _, ref := range []string{"journal/2026-06-30", "2026-06-30"} {
		resolve := v.RunCLI("resolve", ref)
		resolve.MustSucceed(t)
		if got := resolve.DataString("object_id"); got != "2026-06-30" {
			t.Fatalf("resolve %q object_id = %q, want %q", ref, got, "2026-06-30")
		}
	}

	// Creating the daily note places the file under the configured daily directory
	// while its object ID stays the bare date.
	v.RunCLI("daily", "2026-06-30").MustSucceed(t)
	v.AssertFileExists("journal/2026-06-30.md")
	v.AssertFileNotExists("objects/journal/2026-06-30.md")
	v.AssertFileContains("journal/2026-06-30.md", "type: date")
}

// TestIntegration_ImportRespectsDirectoryRootsOnCreate verifies that imports
// create new objects through the canonical path resolution logic, including
// configured directory roots.
func TestIntegration_ImportRespectsDirectoryRootsOnCreate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLIWithStdin(`[{"name":"Freya"}]`, "import", "person")
	result.MustSucceed(t)

	v.AssertFileExists("objects/people/freya.md")
	v.AssertFileNotExists("people/freya.md")
	v.AssertFileNotExists("objects/objects/people/freya.md")
	v.AssertFileContains("objects/people/freya.md", "type: person")
	v.AssertFileContains("objects/people/freya.md", "name: Freya")
}

func TestIntegration_ImportUnknownFieldReturnsStructuredItemError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLIWithStdin(`[{"name":"Freya","favorite_color":"green"}]`, "import", "person")
	result.MustSucceed(t)

	if got, ok := result.Data["errors"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected errors=1, got: %#v", result.Data["errors"])
	}

	results := result.DataList("results")
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 import result item, got %d", len(results))
	}
	item, ok := results[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected import result object, got: %#v", results[0])
	}
	if item["action"] != "error" {
		t.Fatalf("expected import action=error, got: %#v", item["action"])
	}
	if item["code"] != "UNKNOWN_FIELD" {
		t.Fatalf("expected import error code UNKNOWN_FIELD, got: %#v", item["code"])
	}
	details, ok := item["details"].(map[string]interface{})
	if !ok || details == nil {
		t.Fatalf("expected structured details for import item error, got: %#v", item["details"])
	}
	unknownFields, ok := details["unknown_fields"].([]interface{})
	if !ok || len(unknownFields) == 0 {
		t.Fatalf("expected unknown_fields detail, got: %#v", details)
	}
	if unknownFields[0] != "favorite_color" {
		t.Fatalf("expected unknown field favorite_color, got: %#v", unknownFields)
	}

	v.AssertFileNotExists("people/freya.md")
}

// TestIntegration_ImportRespectsProtectedPathsOnUpdate verifies that importing an
// update into a protected path is rejected per item and leaves the existing
// object untouched, matching objectsvc's content-mutation guardrails.
func TestIntegration_ImportRespectsProtectedPathsOnUpdate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("protected_prefixes:\n  - people/\n").
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n\n# Freya\n").
		Build()

	result := v.RunCLIWithStdin(`[{"name":"Freya","email":"freya@example.com"}]`, "import", "person")
	result.MustSucceed(t)

	if got, ok := result.Data["errors"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected errors=1, got: %#v", result.Data["errors"])
	}

	results := result.DataList("results")
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 import result item, got %d", len(results))
	}
	item, ok := results[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected import result object, got: %#v", results[0])
	}
	if item["action"] != "error" {
		t.Fatalf("expected import action=error, got: %#v", item["action"])
	}
	if item["code"] != "VALIDATION_FAILED" {
		t.Fatalf("expected import error code VALIDATION_FAILED, got: %#v", item["code"])
	}

	// The protected object must be left untouched.
	v.AssertFileNotContains("people/freya.md", "email: freya@example.com")
}

func TestIntegration_AutoReindexDatabaseFailuresSurfaceStructuredWarnings(t *testing.T) {
	t.Parallel()

	breakIndex := func(t *testing.T, v *testutil.TestVault) {
		t.Helper()
		ravenDir := filepath.Join(v.Path, ".raven")
		if err := os.RemoveAll(ravenDir); err != nil {
			t.Fatalf("remove .raven: %v", err)
		}
		if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("write .raven file: %v", err)
		}
	}

	assertIndexWarning := func(t *testing.T, result *testutil.CLIResult) {
		t.Helper()
		result.AssertHasWarning(t, "INDEX_UPDATE_FAILED")
		for _, warning := range result.Warnings {
			if warning.Code == "INDEX_UPDATE_FAILED" && strings.Contains(warning.Message, "failed to open index database") {
				return
			}
		}
		t.Fatalf("expected index warning mentioning database open failure, got warnings: %+v", result.Warnings)
	}

	tests := []struct {
		name   string
		run    func(v *testutil.TestVault) *testutil.CLIResult
		assert func(t *testing.T, v *testutil.TestVault)
	}{
		{
			name: "new",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("new", "person", "Freya")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("people/freya.md")
			},
		},
		{
			name: "upsert",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("upsert", "person", "Frigg", "--field", "email=frigg@example.com")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("people/frigg.md")
				v.AssertFileContains("people/frigg.md", "email: frigg@example.com")
			},
		},
		{
			name: "set",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("set", "people/alice", "email=alice@newdomain.com")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileContains("people/alice.md", "email: alice@newdomain.com")
			},
		},
		{
			name: "add",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("add", "Follow up note", "--to", "people/alice")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileContains("people/alice.md", "Follow up note")
			},
		},
		{
			name: "edit",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("edit", "people/alice", "Body", "Updated body")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileContains("people/alice.md", "Updated body")
			},
		},
		{
			name: "import",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLIWithStdin(`[{"name":"Thor"}]`, "import", "person")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("people/thor.md")
			},
		},
		{
			name: "template write",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("template", "write", "meeting.md", "--content", "# {{title}}\n")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("templates/meeting.md")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("people/alice.md", `---
type: person
name: Alice
---

Body
`).
				Build()

			breakIndex(t, v)
			result := tc.run(v)
			result.MustSucceed(t)
			assertIndexWarning(t, result)
			tc.assert(t, v)
		})
	}
}

func TestIntegration_EditSurfacesAutoReindexParseWarnings(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---

Body
`).
		Build()

	result := v.RunCLI("edit", "people/alice", "name: Alice", "name: [")
	result.MustSucceed(t)
	result.AssertHasWarning(t, "INDEX_UPDATE_FAILED")

	foundParseWarning := false
	for _, warning := range result.Warnings {
		if warning.Code == "INDEX_UPDATE_FAILED" && strings.Contains(warning.Message, "failed to parse file") {
			foundParseWarning = true
			break
		}
	}
	if !foundParseWarning {
		t.Fatalf("expected parse warning, got warnings: %+v", result.Warnings)
	}

	v.AssertFileContains("people/alice.md", "name: [")
}
