//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_SetPositionalFieldValues(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    default_path: project/
    name_field: name
    fields:
      name:
        type: string
        required: true
      status:
        type: string
traits: {}
`).
		Build()

	created := v.RunCLI("new", "project", "Perch").MustSucceed(t)
	id := created.DataString("id")
	file := created.DataString("file")
	if id != "project/perch" {
		t.Fatalf("id = %q, want project/perch (file %q)", id, file)
	}

	positional := v.RunCLI("set", id, "name=Perch", "status=active")
	positional.MustSucceed(t)
	v.AssertFileContains(file, "name: Perch")
	v.AssertFileContains(file, "status: active")

	afterDash := v.RunCLI("set", "--", id, "status=paused")
	afterDash.MustSucceed(t)
	v.AssertFileContains(file, "status: paused")

	fromJSON := v.RunCLI("set", id, "--fields-json", `{"status":"active"}`)
	fromJSON.MustSucceed(t)
	v.AssertFileContains(file, "status: active")

	fromFlag := v.RunCLI("set", id, "--field", "status=done")
	fromFlag.MustSucceed(t)
	v.AssertFileContains(file, "status: done")

	invalid := v.RunCLI("set", id, "not-a-pair")
	invalid.MustFail(t, "INVALID_INPUT")
	invalid.MustFailWithMessage(t, "field=value")
	v.AssertFileContains(file, "status: done")
}

func TestIntegration_SetStdinPositionalFieldValues(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Ada").MustSucceed(t)
	v.RunCLI("new", "person", "Bea").MustSucceed(t)

	result := v.RunCLIWithStdin(
		"people/ada\npeople/bea\n",
		"set",
		"--stdin",
		"--confirm",
		"email=ada@example.com",
	)
	result.MustSucceed(t)
	v.AssertFileContains("people/ada.md", "email: ada@example.com")
	v.AssertFileContains("people/bea.md", "email: ada@example.com")
}

func TestIntegration_EditDashTerminatorIsNotAnArg(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("pages/dash.md", "---\ntype: page\n---\n\n- old task\n").
		Build()

	// RunCLI places --json before the command, which is the required order:
	// flags, then "--", then positional strings that start with a dash.
	result := v.RunCLI("edit", "pages/dash.md", "--", "- old task", "- new task")
	result.MustSucceed(t)
	v.AssertFileContains("pages/dash.md", "- new task")
	v.AssertFileNotContains("pages/dash.md", "- old task")
}

func TestIntegration_UnsetMultipleFieldsAfterDash(t *testing.T) {
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

	result := v.RunCLI("unset", "docs/cleanup", "--", "link", "date").MustSucceed(t)
	removed, ok := result.Data["removed_fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("removed_fields = %#v", result.Data["removed_fields"])
	}
	if _, exists := removed["--"]; exists {
		t.Fatalf("dash terminator was counted as a field: %#v", removed)
	}
	if _, exists := removed["link"]; !exists {
		t.Fatalf("link was not removed: %#v", removed)
	}
	if _, exists := removed["date"]; !exists {
		t.Fatalf("date was not removed: %#v", removed)
	}
	missing, _ := result.Data["missing_fields"].([]interface{})
	for _, field := range missing {
		if field == "--" {
			t.Fatalf("dash terminator was counted as a missing field: %#v", missing)
		}
	}
	v.AssertFileNotContains("docs/cleanup.md", "link:")
	v.AssertFileNotContains("docs/cleanup.md", "date:")
	v.AssertFileContains("docs/cleanup.md", "title: Cleanup")
}
