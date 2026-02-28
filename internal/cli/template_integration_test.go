//go:build integration

package cli_test

import "testing"

import "github.com/aidanlsb/raven/internal/testutil"

func TestIntegration_TemplateLifecycle(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	writeCreated := v.RunCLI("template", "write", "meeting.md", "--content", "# {{title}}\n")
	writeCreated.MustSucceed(t)
	if got := writeCreated.DataString("status"); got != "created" {
		t.Fatalf("expected write status 'created', got %q", got)
	}
	v.AssertFileExists("templates/meeting.md")
	v.AssertFileContains("templates/meeting.md", "# {{title}}")

	writeUnchanged := v.RunCLI("template", "write", "meeting.md", "--content", "# {{title}}\n")
	writeUnchanged.MustSucceed(t)
	if got := writeUnchanged.DataString("status"); got != "unchanged" {
		t.Fatalf("expected write status 'unchanged', got %q", got)
	}

	writeUpdated := v.RunCLI("template", "write", "meeting.md", "--content", "# {{title}}\n\n## Notes\n")
	writeUpdated.MustSucceed(t)
	if got := writeUpdated.DataString("status"); got != "updated" {
		t.Fatalf("expected write status 'updated', got %q", got)
	}
	v.AssertFileContains("templates/meeting.md", "## Notes")

	listResult := v.RunCLI("template", "list")
	listResult.MustSucceed(t)
	templates := listResult.DataList("templates")
	if len(templates) != 1 {
		t.Fatalf("expected exactly one template file, got %d (raw: %s)", len(templates), listResult.RawJSON)
	}

	v.RunCLI("schema", "template", "set", "meeting_standard", "--file", "templates/meeting.md").MustSucceed(t)

	// Deletion should be blocked while schema template definitions still reference this file.
	blockedDelete := v.RunCLI("template", "delete", "meeting.md")
	blockedDelete.MustFail(t, "VALIDATION_FAILED")

	v.RunCLI("schema", "template", "remove", "meeting_standard").MustSucceed(t)

	deleteResult := v.RunCLI("template", "delete", "meeting.md")
	deleteResult.MustSucceed(t)
	v.AssertFileNotExists("templates/meeting.md")

	trashPath := deleteResult.DataString("trash_path")
	if trashPath == "" {
		t.Fatalf("expected trash_path in response, got raw: %s", deleteResult.RawJSON)
	}
	v.AssertFileExists(trashPath)
}
