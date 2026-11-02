//go:build integration

package cli_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_JSONOutputIsCompact(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Odin").MustSucceed(t)
	result := v.RunCLI("query", "type:person").MustSucceed(t)
	if strings.Contains(result.RawJSON, "\n  ") {
		t.Fatalf("expected compact JSON, got indented output:\n%s", result.RawJSON)
	}
	if !strings.Contains(result.RawJSON, `"ok":true`) && !strings.Contains(result.RawJSON, `"ok": true`) {
		t.Fatalf("expected JSON envelope, got: %s", result.RawJSON)
	}
}

func TestIntegration_NewWithoutTTYSkipsOptionalFieldPrompts(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	human := v.RunCLIHuman("new", "person", "Freya")
	if human.ExitCode != 0 {
		t.Fatalf("expected create to succeed without a TTY, exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	if strings.Contains(human.Stderr, "email (optional") || strings.Contains(human.Stdout, "email (optional") {
		t.Fatalf("prompted for optional fields without a TTY:\nstdout=%q\nstderr=%q", human.Stdout, human.Stderr)
	}
	v.AssertFileExists("people/freya.md")
	v.AssertFileContains("people/freya.md", "name: Freya")

	jsonResult := v.RunCLI("new", "person", "Thor").MustSucceed(t)
	if jsonResult.DataString("id") == "" {
		t.Fatalf("expected JSON create to return an id, got: %s", jsonResult.RawJSON)
	}
}

func TestIntegration_NewWithoutTTYFailsOnMissingRequiredField(t *testing.T) {
	t.Parallel()
	schemaYAML := `version: 1
types:
  task:
    default_path: task/
    fields:
      status:
        type: string
        required: true
`
	v := testutil.NewTestVault(t).
		WithSchema(schemaYAML).
		Build()

	human := v.RunCLIHuman("new", "task", "Draft")
	if human.ExitCode != 1 {
		t.Fatalf("expected REQUIRED_FIELD_MISSING in human mode, exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	combined := human.Stdout + human.Stderr
	if !strings.Contains(combined, "Missing required fields") && !strings.Contains(combined, "required field") {
		t.Fatalf("expected missing-field error, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}
	if !strings.Contains(combined, "--field") {
		t.Fatalf("expected --field suggestion, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}

	jsonResult := v.RunCLI("new", "task", "Draft")
	jsonResult.MustFail(t, "REQUIRED_FIELD_MISSING")
	if jsonResult.Error == nil || !strings.Contains(jsonResult.Error.Suggestion, "--field") {
		t.Fatalf("expected --field suggestion in JSON error, got %#v", jsonResult.Error)
	}
}

func TestIntegration_DeleteWithoutTTYRequiresForce(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	human := v.RunCLIHuman("delete", "people/alice")
	if human.ExitCode != 1 {
		t.Fatalf("expected confirmation error, exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	combined := human.Stdout + human.Stderr
	if strings.Contains(combined, "Cancelled.") {
		t.Fatalf("silently cancelled without a TTY: %q", combined)
	}
	if !strings.Contains(combined, "confirmation required") && !strings.Contains(combined, "CONFIRMATION_REQUIRED") {
		t.Fatalf("expected confirmation required error, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}
	if !strings.Contains(combined, "--force") {
		t.Fatalf("expected --force suggestion, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}
	v.AssertFileExists("people/alice.md")
}

func TestIntegration_MoveMismatchWithoutTTYRequiresSkipFlag(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	v.RunCLI("new", "project", "Website", "--field", "status=active").MustSucceed(t)

	human := v.RunCLIHuman("move", "projects/website", "people/website")
	if human.ExitCode != 1 {
		t.Fatalf("expected confirmation error for type mismatch, exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	combined := human.Stdout + human.Stderr
	if strings.Contains(combined, "Cancelled.") {
		t.Fatalf("silently cancelled without a TTY: %q", combined)
	}
	if !strings.Contains(combined, "--skip-type-check") {
		t.Fatalf("expected --skip-type-check suggestion, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}
	v.AssertFileExists("projects/website.md")

	jsonResult := v.RunCLI("move", "projects/website", "people/website")
	if jsonResult.OK {
		if jsonResult.Data["needs_confirm"] != true {
			t.Fatalf("JSON mismatch should preview with needs_confirm, got: %s", jsonResult.RawJSON)
		}
		return
	}
	jsonResult.MustFail(t, "CONFIRMATION_REQUIRED")
}

func TestIntegration_UnknownFlagSuggestsClosestAndHelp(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	human := v.RunCLIHuman("search", "notes", "--limt", "5")
	if human.ExitCode != 1 {
		t.Fatalf("expected usage error, exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	combined := human.Stdout + human.Stderr
	if len(combined) > 800 {
		t.Fatalf("expected short usage error, got %d bytes:\n%s", len(combined), combined)
	}
	if strings.Contains(combined, "Usage:") && strings.Count(combined, "--") > 15 {
		t.Fatalf("expected no full usage dump, got:\n%s", combined)
	}
	if !strings.Contains(combined, "--limit") {
		t.Fatalf("expected closest-flag suggestion, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}
	if !strings.Contains(combined, "rvn search --help") {
		t.Fatalf("expected help line, got stdout=%q stderr=%q", human.Stdout, human.Stderr)
	}

	jsonResult := v.RunCLI("search", "notes", "--limt", "5")
	jsonResult.MustFail(t, "INVALID_INPUT")
	if jsonResult.Error == nil || !strings.Contains(jsonResult.Error.Suggestion, "--limit") {
		t.Fatalf("expected --limit suggestion in JSON, got %#v raw=%s", jsonResult.Error, jsonResult.RawJSON)
	}
	if strings.Contains(jsonResult.Error.Suggestion, "starts with a dash") {
		t.Fatalf("unknown flag should not get dash-terminator advice: %s", jsonResult.Error.Suggestion)
	}
}

func TestIntegration_SearchJSONIncludesTotalAndHasMore(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/one.md", "---\ntype: page\n---\n# One\nquarterly plan\n").
		WithFile("notes/two.md", "---\ntype: page\n---\n# Two\nquarterly review\n").
		WithFile("notes/three.md", "---\ntype: page\n---\n# Three\nquarterly retro\n").
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	page := v.RunCLI("search", "quarterly", "--limit", "1").MustSucceed(t)
	if page.Data["total"] == nil {
		t.Fatalf("missing total in search JSON: %s", page.RawJSON)
	}
	if page.Data["has_more"] != true {
		t.Fatalf("has_more = %#v, want true; %s", page.Data["has_more"], page.RawJSON)
	}
	if page.Data["next_offset"] == nil {
		t.Fatalf("missing next_offset when has_more: %s", page.RawJSON)
	}

	human := v.RunCLIHuman("search", "quarterly", "--limit", "1")
	if human.ExitCode != 0 {
		t.Fatalf("human search failed: exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	if !strings.Contains(human.Stdout, "of") {
		t.Fatalf("expected capped count in human search header, got: %q", human.Stdout)
	}
}

func TestIntegration_QueryPipeShowsObjectName(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	v.RunCLI("new", "person", "Odin").MustSucceed(t)

	human := v.RunCLIHuman("query", "type:person", "--pipe")
	if human.ExitCode != 0 {
		t.Fatalf("pipe query failed: exit=%d stdout=%q stderr=%q", human.ExitCode, human.Stdout, human.Stderr)
	}
	if !strings.Contains(human.Stdout, "people/odin") {
		t.Fatalf("expected ID column, got: %q", human.Stdout)
	}
	if !strings.Contains(human.Stdout, "Odin") {
		t.Fatalf("expected display name Odin in pipe content, got: %q", human.Stdout)
	}
}

func TestIntegration_SearchHelpDoesNotDedupeDefaultTwice(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "search", "--help")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --help failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "(default: 20) (default 20)") {
		t.Fatalf("duplicated default in search help:\n%s", out)
	}
}

func TestIntegration_HumanUnknownFlagDoesNotDumpUsage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", dir, "search", "--not-a-real-flag")
	cmd.Stdin = nil
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	combined := stdout.String() + stderr.String()
	if len(combined) > 800 {
		t.Fatalf("expected short error, got %d bytes:\n%s", len(combined), combined)
	}
	if strings.Contains(combined, "Available Commands:") {
		t.Fatalf("usage dump still printed:\n%s", combined)
	}
}

func TestIntegration_DeleteJSONAppliesWithoutForce(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	v.RunCLI("new", "person", "Carol").MustSucceed(t)
	v.RunCLI("delete", "people/carol").MustSucceed(t)
	v.AssertFileNotExists("people/carol.md")
}
