//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func hasWarningCode(r *testutil.CLIResult, code string) bool {
	for _, w := range r.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
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
// write) but surfaces the missing target via a REF_NOT_FOUND warning plus
// missing_ref data.
func TestIntegration_NewSurfacesMissingRefTarget(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("new", "project", "Website", "--field", "owner=people/ghost")
	result.MustSucceed(t)
	v.AssertFileExists("projects/website.md")

	if !hasWarningCode(result, "REF_NOT_FOUND") {
		t.Fatalf("expected REF_NOT_FOUND warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
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

	if hasWarningCode(result, "REF_NOT_FOUND") {
		t.Fatalf("did not expect REF_NOT_FOUND warning, got %#v", result.Warnings)
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

	result := v.RunCLI("set", "projects/website", "owner=people/ghost")
	result.MustSucceed(t)

	if !hasWarningCode(result, "REF_NOT_FOUND") {
		t.Fatalf("expected REF_NOT_FOUND warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
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

	if !hasWarningCode(result, "REF_NOT_FOUND") {
		t.Fatalf("expected REF_NOT_FOUND warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
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

	result := v.RunCLI("edit", "projects/edit-target", "Status line", "Status [[projects/ghost-project]]", "--confirm")
	result.MustSucceed(t)

	if !hasWarningCode(result, "REF_NOT_FOUND") {
		t.Fatalf("expected REF_NOT_FOUND warning, got %#v\nraw: %s", result.Warnings, result.RawJSON)
	}
	if got := missingRefCount(t, result); got != 1 {
		t.Fatalf("missing_refs = %d, want 1\nraw: %s", got, result.RawJSON)
	}
}
