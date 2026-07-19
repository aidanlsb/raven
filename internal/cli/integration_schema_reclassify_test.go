//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_SchemaTemplateLifecycle tests schema template lifecycle commands.
func TestIntegration_SchemaTemplateLifecycle(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	t.Run("schema template set/get/remove", func(t *testing.T) {
		v.WriteFile("templates/person.md", "# Person Profile\n")

		result := v.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md")
		result.MustSucceed(t)

		result = v.RunCLI("schema", "template", "get", "person_profile")
		result.MustSucceed(t)
		if result.DataString("id") != "person_profile" {
			t.Errorf("expected id=person_profile, got %q", result.DataString("id"))
		}
		if result.DataString("file") != "templates/person.md" {
			t.Errorf("expected file 'templates/person.md', got %q", result.DataString("file"))
		}

		v.RunCLI("schema", "template", "bind", "person_profile", "--type", "person").MustSucceed(t)
		v.RunCLI("schema", "template", "default", "person_profile", "--type", "person").MustSucceed(t)
		v.RunCLI("new", "person", "Alice").MustSucceed(t)
		v.AssertFileContains("people/alice.md", "# Person Profile")

		result = v.RunCLI("schema", "template", "unbind", "person_profile", "--type", "person")
		result.MustFailWithMessage(t, "--clear-default")

		result = v.RunCLI("schema", "template", "unbind", "person_profile", "--type", "person", "--clear-default")
		result.MustSucceed(t)
		if result.Data["removed"] != true {
			t.Errorf("expected removed=true")
		}

		result = v.RunCLI("schema", "template", "remove", "person_profile")
		result.MustSucceed(t)
		if result.Data["removed"] != true {
			t.Errorf("expected schema template remove=true")
		}
	})

	t.Run("daily lifecycle via date type templates", func(t *testing.T) {
		v.WriteFile("templates/daily.md", "# {{weekday}}, {{date}}\n\n## Notes\n")
		v.WriteFile("templates/daily-brief.md", "# {{date}}\n\n## Brief\n")

		result := v.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md")
		result.MustSucceed(t)
		if result.DataString("file") != "templates/daily.md" {
			t.Errorf("expected daily file binding to templates/daily.md, got %q", result.DataString("file"))
		}
		v.RunCLI("schema", "template", "set", "daily_brief", "--file", "templates/daily-brief.md").MustSucceed(t)

		result = v.RunCLI("schema", "template", "bind", "daily_default", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("bind core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "bind", "daily_brief", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("bind core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "default", "daily_default", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("default core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "list", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("list core = %q, want %q", got, "date")
		}

		v.RunCLI("daily", "2026-02-03").MustSucceed(t)
		v.AssertFileContains("daily/2026-02-03.md", "## Notes")
		v.RunCLI("daily", "2026-02-05", "--template", "daily_brief").MustSucceed(t)
		v.AssertFileContains("daily/2026-02-05.md", "## Brief")

		result = v.RunCLI("schema", "template", "default", "--core", "date", "--clear")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("clear default core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "unbind", "daily_brief", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("unbind core = %q, want %q", got, "date")
		}
		v.RunCLI("daily", "2026-02-04").MustSucceed(t)
		v.AssertFileNotContains("daily/2026-02-04.md", "## Notes")
	})
}

func TestIntegration_ReclassifyRejectsMalformedFieldFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
	}{
		{name: "missing equals", field: "author"},
		{name: "empty key", field: "=Tolkien"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(`version: 2
types:
  note:
    default_path: notes/
    fields:
      title:
        type: string
  book:
    default_path: books/
    fields:
      title:
        type: string
`).
				WithFile("notes/my-note.md", `---
type: note
title: My Note
---

Body
`).
				Build()

			result := v.RunCLI("reclassify", "notes/my-note", "book", "--field", tc.field, "--no-move", "--force")
			result.MustFail(t, "INVALID_INPUT")
			result.MustFailWithMessage(t, "expected key=value")

			v.AssertFileExists("notes/my-note.md")
			v.AssertFileNotExists("books/my-note.md")
			v.AssertFileContains("notes/my-note.md", "type: note")
			v.AssertFileNotContains("notes/my-note.md", "type: book")
		})
	}
}
