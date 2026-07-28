//go:build integration

package cli_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_CheckValidation tests the check command for validation.
func TestIntegration_CheckValidation(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("notes/orphan.md", `---
type: page
---
# Note with missing ref

See [[nonexistent/page]] for details.
`).
		Build()

	// Reindex
	v.RunCLI("reindex").MustSucceed(t)

	// Run check - the check command has its own format (not the standard ok/data envelope)
	// Just verify the command runs and produces output
	result := v.RunCLI("check")

	// Check command output is structured differently - look at raw JSON
	if result.RawJSON == "" {
		t.Error("expected check to produce output")
	}

	// The raw JSON should contain issues for missing reference
	if !strings.Contains(result.RawJSON, "missing_reference") {
		t.Errorf("expected check output to include 'missing_reference' issue\nRaw: %s", result.RawJSON)
	}
}

func TestIntegration_CheckBrokenFileLinksSkipsURLs(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/links.md", strings.Join([]string{
			"[existing](../files/existing.txt)",
			"[missing](../files/missing.txt)",
			"[url](https://example.com/files/missing.txt)",
			"",
		}, "\n")).
		WithFile("files/existing.txt", "present\n").
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("check")
	if !strings.Contains(result.RawJSON, "broken_file_link") {
		t.Fatalf("expected broken_file_link issue, got: %s", result.RawJSON)
	}
	if !strings.Contains(result.RawJSON, "../files/missing.txt") {
		t.Fatalf("expected missing file target in issue, got: %s", result.RawJSON)
	}
	if strings.Contains(result.RawJSON, "https://example.com") {
		t.Fatalf("URL target must not be checked for existence: %s", result.RawJSON)
	}
}

func TestIntegration_CheckFixSubcommandAppliesShortRefFixes(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---`).
		WithFile("projects/roadmap.md", `---
type: project
title: Roadmap
owner: "[[freya]]"
---`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	preview := v.RunCLI("check", "fix")
	preview.MustSucceed(t)
	if got, ok := preview.Data["preview"].(bool); !ok || !got {
		t.Fatalf("expected preview=true, got %#v", preview.Data["preview"])
	}
	if got, ok := preview.Data["fixable_issues"].(float64); !ok || int(got) < 1 {
		t.Fatalf("expected at least 1 fixable issue, got %#v", preview.Data["fixable_issues"])
	}

	apply := v.RunCLI("check", "fix", "--confirm")
	apply.MustSucceed(t)
	if got, ok := apply.Data["preview"].(bool); !ok || got {
		t.Fatalf("expected preview=false after apply, got %#v", apply.Data["preview"])
	}
	if got, ok := apply.Data["fixed_issues"].(float64); !ok || int(got) < 1 {
		t.Fatalf("expected at least 1 fixed issue, got %#v", apply.Data["fixed_issues"])
	}

	v.AssertFileContains("projects/roadmap.md", "owner: \"[[people/freya]]\"")
}

// TestIntegration_CheckFixCanonicalPathMovesFiles verifies that check fix
// detects files outside the configured directory roots and migrates them via
// real file moves, with reference updates following the move.
func TestIntegration_CheckFixCanonicalPathMovesFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    default_path: person/
    name_field: name
    fields:
      name:
        type: string
        required: true
`).
		WithRavenYAML(`directories:
  type: type/
  page: page/
`).
		WithFile("objects/person/john.md", `---
type: person
name: John
---
`).
		WithFile("page/notes/today.md", `---
type: page
---
Mentioned [[type/person/john]] today.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	preview := v.RunCLI("check", "fix")
	preview.MustSucceed(t)
	if got, ok := preview.Data["fixable_issues"].(float64); !ok || int(got) < 2 {
		t.Fatalf("expected at least 2 fixable issues (move + ref), got %#v", preview.Data["fixable_issues"])
	}

	apply := v.RunCLI("check", "fix", "--confirm")
	apply.MustSucceed(t)

	v.AssertFileExists("type/person/john.md")
	v.AssertFileNotExists("objects/person/john.md")

	v.AssertFileContains("page/notes/today.md", "[[person/john]]")
	v.AssertFileNotContains("page/notes/today.md", "[[type/person/john]]")
}

// TestIntegration_CheckFixCanonicalPathSkipsCollisions verifies that when a
// non_canonical_path move would collide with an existing file at the
// canonical destination, check fix skips that move and continues past it.
func TestIntegration_CheckFixCanonicalPathSkipsCollisions(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    default_path: person/
    name_field: name
    fields:
      name:
        type: string
        required: true
`).
		WithRavenYAML(`directories:
  type: type/
  page: page/
`).
		WithFile("objects/person/john.md", `---
type: person
name: John (old)
---
`).
		WithFile("type/person/john.md", `---
type: person
name: John (new)
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	apply := v.RunCLI("check", "fix", "--confirm")
	apply.MustSucceed(t)

	if got, ok := apply.Data["skipped_issues"].(float64); !ok || int(got) < 1 {
		t.Fatalf("expected at least 1 skipped fix for collision, got %#v", apply.Data["skipped_issues"])
	}

	v.AssertFileExists("objects/person/john.md")
	v.AssertFileExists("type/person/john.md")
}

func TestIntegration_CheckCreateMissingSubcommandJSONConfirmRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  meeting:
    default_path: meeting/
  project:
    default_path: projects/
    fields:
      meeting:
        type: ref
        target: meeting
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/weekly.md", `---
type: project
meeting: "[[meeting/all-hands]]"
---
# Weekly
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// check create-missing may still exit non-zero due pre-existing validation issues;
	// validate side effects through file creation.
	_ = v.RunCLI("check", "create-missing", "--confirm")

	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
}

// TestIntegration_CheckCreateMissingRespectsDirectoryRoots verifies that
// `check create-missing` creates typed objects under configured directory roots.
func TestIntegration_CheckCreateMissingRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  meeting:
    default_path: meeting/
  project:
    default_path: projects/
    fields:
      meeting:
        type: ref
        target: meeting
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/kickoff.md", `---
type: project
meeting: "[[meeting/all-hands]]"
---
# Kickoff
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// check create-missing is interactive and non-JSON; accept default "yes"
	// for "Certain (from typed fields)" prompts by sending an empty line.
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "check", "create-missing")
	cmd.Stdin = strings.NewReader("\n")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Ensure the missing-reference creation flow ran.
	if !strings.Contains(outputStr, "Missing References") {
		t.Fatalf("expected check output to include missing reference prompt, got:\n%s", outputStr)
	}

	// Regression assertion: created file must be nested under objects root.
	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
	v.AssertFileContains("objects/meeting/all-hands.md", "type: meeting")
}

// TestIntegration_CheckCreateMissingUnknownTypeRespectsDirectoryRoots verifies
// the unknown-type interactive flow:
// 1) user provides a new type name
// 2) check creates the type in schema.yaml
// 3) missing page is created under configured objects root
func TestIntegration_CheckCreateMissingUnknownTypeRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  project:
    default_path: projects/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/launch.md", `---
type: project
---
# Launch

See [[meeting/all-hands]] for notes.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// Interactive inputs:
	// - Type for meeting/all-hands: meeting
	// - Create new type meeting?: y
	// - Default path for meeting: meeting/
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "check", "create-missing")
	cmd.Stdin = strings.NewReader("meeting\ny\nmeeting/\n")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Ensure the unknown-type flow ran.
	if !strings.Contains(outputStr, "Unknown type (please specify)") {
		t.Fatalf("expected check output to include unknown type prompt, got:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "Created type 'meeting' in schema.yaml") {
		t.Fatalf("expected check output to include type creation message, got:\n%s", outputStr)
	}

	// Regression assertion: created page must be nested under objects root.
	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
	v.AssertFileContains("objects/meeting/all-hands.md", "type: meeting")

	// Verify schema was updated with the new type/default_path.
	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meeting/")
}

// TestIntegration_CheckCreateMissingJSONConfirmRespectsDirectoryRoots verifies
// non-interactive create-missing in JSON mode (agent-style invocation).
func TestIntegration_CheckCreateMissingJSONConfirmRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  meeting:
    default_path: meeting/
  project:
    default_path: projects/
    fields:
      meeting:
        type: ref
        target: meeting
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/weekly.md", `---
type: project
meeting: "[[meeting/all-hands]]"
---
# Weekly
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// Agent-style call: JSON mode + create-missing + confirm.
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "--json", "check", "create-missing", "--confirm")
	_, _ = cmd.CombinedOutput() // check may exit non-zero due validation issues; side effects are what we validate.

	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
}
