//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func hasWarningCode(r *testutil.CLIResult, code string) bool {
	return warningByCode(r, code) != nil
}

func warningByCode(r *testutil.CLIResult, code string) *testutil.CLIWarning {
	for i := range r.Warnings {
		if r.Warnings[i].Code == code {
			return &r.Warnings[i]
		}
	}
	return nil
}

func missingRefCount(t *testing.T, r *testutil.CLIResult) int {
	t.Helper()
	raw, ok := r.Data["missing_refs"]
	if !ok {
		return 0
	}
	count, ok := raw.(float64)
	if !ok {
		t.Fatalf("missing_refs = %#v, want a number", raw)
	}
	return int(count)
}

// TestIntegration_NewSurfacesMissingRefTarget verifies that creating an object
// with a ref field pointing at a non-existent target still succeeds (permissive
// write) but surfaces the missing target via a REF_TARGET_MISSING warning plus
// missing_ref data.
func TestIntegration_NewSurfacesMissingRefTarget(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("new", "project", "Website", "--field", "owner=people/ghost")
	result.MustSucceed(t)
	v.AssertFileExists("projects/website.md")

	if !hasWarningCode(result, "REF_TARGET_MISSING") {
		t.Fatalf("expected REF_TARGET_MISSING warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
	}
	if got := missingRefCount(t, result); got != 1 {
		t.Fatalf("missing_refs = %d, want 1\nraw: %s", got, result.RawJSON)
	}
	items := result.DataList("missing_ref_items")
	if len(items) != 1 {
		t.Fatalf("missing_ref_items = %#v, want 1 item", items)
	}
	item, _ := items[0].(map[string]interface{})
	if item["InferredType"] != "person" {
		t.Fatalf("inferred type = %#v, want person", item["InferredType"])
	}

	// The warning must carry structured remediation so agents can invoke the
	// creation without shell-parsing the create_command string.
	w := warningByCode(result, "REF_TARGET_MISSING")
	if w == nil {
		t.Fatalf("missing REF_TARGET_MISSING warning\nraw: %s", result.RawJSON)
	}
	if w.SuggestedType != "person" {
		t.Fatalf("suggested_type = %q, want person\nraw: %s", w.SuggestedType, result.RawJSON)
	}
	if w.CreateCommand == "" {
		t.Fatalf("expected create_command hint, got empty\nraw: %s", result.RawJSON)
	}
	if w.CreateInvoke == nil {
		t.Fatalf("expected structured create_invoke, got nil\nraw: %s", result.RawJSON)
	}
	if w.CreateInvoke.Command != "new" {
		t.Fatalf("create_invoke.command = %q, want new\nraw: %s", w.CreateInvoke.Command, result.RawJSON)
	}
	if got := w.CreateInvoke.Args["type"]; got != "person" {
		t.Fatalf("create_invoke.args.type = %#v, want person\nraw: %s", got, result.RawJSON)
	}
	if got := w.CreateInvoke.Args["path"]; got != "people/ghost" {
		t.Fatalf("create_invoke.args.path = %#v, want people/ghost\nraw: %s", got, result.RawJSON)
	}
	if got := w.CreateInvoke.Args["title"]; got != "ghost" {
		t.Fatalf("create_invoke.args.title = %#v, want ghost\nraw: %s", got, result.RawJSON)
	}
}

// TestIntegration_NewExistingRefTargetNoWarning verifies the inverse: when the
// ref target exists, no missing-ref warning or data is emitted.
func TestIntegration_NewExistingRefTargetNoWarning(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Freya").MustSucceed(t)

	result := v.RunCLI("new", "project", "Website", "--field", "owner=people/freya")
	result.MustSucceed(t)

	if hasWarningCode(result, "REF_TARGET_MISSING") {
		t.Fatalf("did not expect REF_TARGET_MISSING warning, got %#v", result.Warnings)
	}
	if got := missingRefCount(t, result); got != 0 {
		t.Fatalf("missing_refs = %d, want 0", got)
	}
}

// TestIntegration_SetSurfacesMissingRefTarget verifies set on a ref field
// surfaces a missing target.
func TestIntegration_SetSurfacesMissingRefTarget(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Website").MustSucceed(t)

	result := v.RunCLI("set", "projects/website", "--field", "owner=people/ghost")
	result.MustSucceed(t)

	if !hasWarningCode(result, "REF_TARGET_MISSING") {
		t.Fatalf("expected REF_TARGET_MISSING warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
	}
	if got := missingRefCount(t, result); got != 1 {
		t.Fatalf("missing_refs = %d, want 1\nraw: %s", got, result.RawJSON)
	}
}

// TestIntegration_AddSurfacesMissingRefTarget verifies appending body content
// with a [[ref]] to a missing target surfaces it.
func TestIntegration_AddSurfacesMissingRefTarget(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Website").MustSucceed(t)

	result := v.RunCLI("add", "See [[projects/ghost-project]] for details", "--to", "projects/website")
	result.MustSucceed(t)

	if !hasWarningCode(result, "REF_TARGET_MISSING") {
		t.Fatalf("expected REF_TARGET_MISSING warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
	}
	if got := missingRefCount(t, result); got != 1 {
		t.Fatalf("missing_refs = %d, want 1\nraw: %s", got, result.RawJSON)
	}
}

// TestIntegration_EditSurfacesMissingRefTarget verifies an applied edit that
// introduces a [[ref]] to a missing target surfaces it.
func TestIntegration_EditSurfacesMissingRefTarget(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("upsert", "project", "Edit Target", "--content", "Status line").MustSucceed(t)

	result := v.RunCLI("edit", "projects/edit-target", "Status line", "Status [[projects/ghost-project]]")
	result.MustSucceed(t)

	if !hasWarningCode(result, "REF_TARGET_MISSING") {
		t.Fatalf("expected REF_TARGET_MISSING warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
	}
	if got := missingRefCount(t, result); got != 1 {
		t.Fatalf("missing_refs = %d, want 1\nraw: %s", got, result.RawJSON)
	}
}
