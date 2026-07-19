//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_NewWithExplicitPath(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLI("new", "note", "Raven Friction", "--path", "note/raven-logo-brief")
	result.MustSucceed(t)

	v.AssertFileExists("objects/note/raven-logo-brief.md")
	v.AssertFileNotExists("objects/note/raven-friction.md")
}

func TestIntegration_NewSlugifiesTitleWithPathSeparator(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
    name_field: title
    fields:
      title:
        type: string
        required: true
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	title := "config.VaultConfig duplicates internal/paths"
	result := v.RunCLI("new", "note", title)
	result.MustSucceed(t)

	// The "/" in the title is slugified into a single filename component rather
	// than creating a nested directory.
	relFile := "objects/note/config-vaultconfig-duplicates-internal-paths.md"
	v.AssertFileExists(relFile)
	v.AssertFileNotExists("objects/note/internal/paths.md")

	// The display title is persisted verbatim in frontmatter.
	v.AssertFileContains(relFile, "title: "+title)

	// The object ID is path-derived from the slugified path (roots stripped).
	if got := result.DataString("id"); got != "note/config-vaultconfig-duplicates-internal-paths" {
		t.Fatalf("unexpected object id: %q", got)
	}
	if got := result.DataString("file"); got != relFile {
		t.Fatalf("unexpected file path: %q", got)
	}
}

// TestIntegration_SchemaValidationErrors tests that schema validation errors are properly reported.
func TestIntegration_SchemaValidationErrors(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Try to create a person without providing the required fields via title
	// Create the file manually without a required field and verify check finds the issue
	v.RunCLI("new", "person", "TestPerson").MustSucceed(t)

	// Unknown fields should fail fast.
	result := v.RunCLI("set", "people/testperson", "status=invalid_value")
	result.MustFail(t, "UNKNOWN_FIELD")
	if result.Error == nil || result.Error.Details == nil {
		t.Fatalf("expected unknown field details in error, got: %#v", result.Error)
	}
	unknownFieldsRaw, ok := result.Error.Details["unknown_fields"].([]interface{})
	if !ok || len(unknownFieldsRaw) == 0 {
		t.Fatalf("expected unknown_fields in details, got: %#v", result.Error.Details)
	}
	if unknownFieldsRaw[0] != "status" {
		t.Fatalf("expected unknown field 'status', got: %#v", unknownFieldsRaw)
	}
	result.MustFailWithMessage(t, "schema type person")
}

func TestIntegration_SetBulkFailsOnSchemaLoadError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Schema Broken").MustSucceed(t)

	schemaPath := filepath.Join(v.Path, "schema.yaml")
	if err := os.WriteFile(schemaPath, []byte("version: ["), 0o644); err != nil {
		t.Fatalf("failed to corrupt schema for test: %v", err)
	}

	result := v.RunCLIWithStdin("people/schema-broken\n", "set", "--stdin", "email=broken@example.com", "--confirm")
	result.MustFail(t, "SCHEMA_INVALID")
	result.MustFailWithMessage(t, "Fix schema.yaml and try again")
}

func TestIntegration_SetResolvesObjectIDsAndWikiLinks(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	v.RunCLI("new", "person", "Dana").MustSucceed(t)

	v.RunCLI("set", "people/dana", "email=dana@example.com").MustSucceed(t)
	v.RunCLI("set", "[[people/dana]]", "email=dana+wiki@example.com").MustSucceed(t)

	v.AssertFileContains("objects/people/dana.md", "email: dana+wiki@example.com")
}

func TestIntegration_SetValidatesTypedValuesAtWriteTime(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Dana").MustSucceed(t)

	// email is a string field; unquoted true should fail type validation.
	result := v.RunCLI("set", "people/dana", "email=true")
	result.MustFail(t, "VALIDATION_FAILED")

	// Confirm no invalid bool value was written into frontmatter.
	v.AssertFileNotContains("people/dana.md", "email: true")
}

func TestIntegration_SetNormalizesDateAndDateTargetRefFields(t *testing.T) {
	t.Parallel()
	today := time.Now().Format("2006-01-02")

	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  brief:
    default_path: brief/
    name_field: title
    fields:
      title:
        type: string
        required: true
      due:
        type: date
      date:
        type: ref
        target: date
`).
		WithFile("daily/"+today+".md", `---
type: date
---
# Today
`).
		Build()

	v.RunCLI("new", "brief", "Daily Brief", "--field", "due=today", "--field", "date=today").MustSucceed(t)
	v.AssertFileContains("brief/daily-brief.md", `due: "`+today+`"`)
	v.AssertFileContains("brief/daily-brief.md", `date: "`+today+`"`)

	v.RunCLI("reindex").MustSucceed(t)
	result := v.RunCLI("query", "type:brief .date==today")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

func TestIntegration_UpsertValidatesTypedValuesAtWriteTime(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Website", "--field", "status=active").MustSucceed(t)

	// status is enum(active|paused|done); invalid enum should fail.
	result := v.RunCLI("upsert", "project", "Website", "--field", "status=not-a-valid-status")
	result.MustFail(t, "VALIDATION_FAILED")

	// Existing value should remain unchanged.
	v.AssertFileContains("projects/website.md", "status: active")
}

func TestIntegration_UpsertUnknownFieldFailsFast(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("upsert", "person", "Unknown Field User", "--field", "favorite_color=blue")
	result.MustFail(t, "UNKNOWN_FIELD")
	result.MustFailWithMessage(t, "schema type person")

	if result.Error == nil || result.Error.Details == nil {
		t.Fatalf("expected unknown field details in error, got: %#v", result.Error)
	}
	unknownFieldsRaw, ok := result.Error.Details["unknown_fields"].([]interface{})
	if !ok || len(unknownFieldsRaw) == 0 {
		t.Fatalf("expected unknown_fields in details, got: %#v", result.Error.Details)
	}
	if unknownFieldsRaw[0] != "favorite_color" {
		t.Fatalf("expected unknown field 'favorite_color', got: %#v", unknownFieldsRaw)
	}

	v.AssertFileNotExists("people/unknown-field-user.md")
}

func TestIntegration_SetFieldsJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Erin").MustSucceed(t)

	// email is a string field; JSON string "true" should stay a string.
	result := v.RunCLI("set", "people/erin", "--fields-json", `{"email":"true"}`)
	result.MustSucceed(t)
	v.AssertFileContains("people/erin.md", `email: "true"`)
}

func TestIntegration_UnsetRemovesUnknownAndKnownFrontmatterFields(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  doc:
    default_path: docs/
    fields:
      title:
        type: string
        required: true
      link:
        type: string
traits: {}
`).
		WithFile("docs/cleanup.md", "---\ntype: doc\ntitle: Cleanup\nlink: https://example.com\ndate: 2026-06-07\n---\n# Cleanup\n").
		Build()

	v.RunCLI("reindex").MustSucceed(t)
	before := v.RunCLI("query", `type:doc .link=="https://example.com"`).MustSucceed(t)
	if before.Meta == nil || before.Meta.Count != 1 {
		t.Fatalf("expected indexed link before unset, got meta=%#v raw=%s", before.Meta, before.RawJSON)
	}

	result := v.RunCLI("unset", "docs/cleanup", "link", "date").MustSucceed(t)
	if modified, ok := result.Data["modified"].(bool); !ok || !modified {
		t.Fatalf("expected modified=true, got data=%#v", result.Data)
	}
	v.AssertFileNotContains("docs/cleanup.md", "link:")
	v.AssertFileNotContains("docs/cleanup.md", "date:")
	v.AssertFileContains("docs/cleanup.md", "title: Cleanup")
	v.AssertFileContains("docs/cleanup.md", "# Cleanup")

	after := v.RunCLI("query", `type:doc .link=="https://example.com"`).MustSucceed(t)
	if after.Meta != nil && after.Meta.Count != 0 {
		t.Fatalf("expected index to drop removed link, got meta=%#v raw=%s", after.Meta, after.RawJSON)
	}
}

func TestIntegration_SetBulkFieldsJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Bulk Erin One").MustSucceed(t)
	v.RunCLI("new", "person", "Bulk Erin Two").MustSucceed(t)

	result := v.RunCLIWithStdin(
		"people/bulk-erin-one\npeople/bulk-erin-two\n",
		"set",
		"--stdin",
		"--confirm",
		"--fields-json",
		`{"email":"true"}`,
	)
	result.MustSucceed(t)
	v.AssertFileContains("people/bulk-erin-one.md", `email: "true"`)
	v.AssertFileContains("people/bulk-erin-two.md", `email: "true"`)
}

func TestIntegration_UpsertFieldsJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("upsert", "person", "Fields Json User", "--fields-json", `{"email":"true"}`)
	result.MustSucceed(t)
	v.AssertFileContains("people/fields-json-user.md", `email: "true"`)
}

func TestIntegration_NewFieldsJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("new", "person", "New Fields Json User", "--fields-json", `{"email":"true"}`)
	result.MustSucceed(t)
	v.AssertFileContains("people/new-fields-json-user.md", `email: "true"`)
}

func TestIntegration_UpsertWithExplicitPath(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLI("upsert", "note", "Raven Friction", "--path", "note/raven-logo-brief", "--content", "# V1")
	result.MustSucceed(t)
	v.AssertFileExists("objects/note/raven-logo-brief.md")
	v.AssertFileContains("objects/note/raven-logo-brief.md", "# V1")

	result = v.RunCLI("upsert", "note", "Raven Friction", "--path", "note/raven-logo-brief", "--content", "# V2")
	result.MustSucceed(t)
	v.AssertFileContains("objects/note/raven-logo-brief.md", "# V2")
}

func TestIntegration_UpsertWithContentFile(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	contentFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(contentFile, []byte("# From File\n\n[[note/ref]]\n"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}

	result := v.RunCLI("upsert", "note", "File Body", "--content-file", contentFile)
	result.MustSucceed(t)
	v.AssertFileExists("objects/note/file-body.md")
	v.AssertFileContains("objects/note/file-body.md", "# From File")
	v.AssertFileContains("objects/note/file-body.md", "[[note/ref]]")
}

func TestIntegration_UpsertWithContentFileStdin(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLIWithStdin("# From Stdin\n\nBody\n", "upsert", "note", "Stdin Body", "--content-file", "-")
	result.MustSucceed(t)
	v.AssertFileExists("objects/note/stdin-body.md")
	v.AssertFileContains("objects/note/stdin-body.md", "# From Stdin")
	v.AssertFileContains("objects/note/stdin-body.md", "Body")
}

func TestIntegration_UpsertRejectsContentAndContentFileTogether(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	contentFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(contentFile, []byte("# From File\n"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}

	result := v.RunCLI("upsert", "project", "Conflict Body", "--content", "# Inline", "--content-file", contentFile)
	result.MustFail(t, "INVALID_INPUT")
	result.MustFailWithMessage(t, "mutually exclusive")
	v.AssertFileNotExists("projects/conflict-body.md")
}
