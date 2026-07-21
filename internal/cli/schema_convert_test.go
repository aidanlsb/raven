package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestSchemaConvertTraitPreviewAndConfirm(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, medium, low]
    default: medium
`).
		WithFile("notes/work.md", "- Now @priority(high)\n- Later @priority(low)\n").
		Build()

	beforeSchema := v.ReadFile("schema.yaml")
	beforeNote := v.ReadFile("notes/work.md")
	args := []string{
		"schema", "convert", "trait", "priority",
		"--type", "bool",
		"--map-json", `{"high":true,"medium":true,"low":false}`,
	}

	preview := v.RunCLI(args...)
	preview.MustSucceed(t)
	if preview.Data["preview"] != true {
		t.Fatalf("expected preview=true, got %#v", preview.Data)
	}
	if got := v.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatal("preview changed schema.yaml")
	}
	if got := v.ReadFile("notes/work.md"); got != beforeNote {
		t.Fatal("preview changed Markdown")
	}

	applied := v.RunCLI(append(args, "--confirm")...)
	applied.MustSucceed(t)
	if applied.Data["converted"] != true {
		t.Fatalf("expected converted=true, got %#v", applied.Data)
	}
	v.AssertFileContains("schema.yaml", "type: bool")
	v.AssertFileContains("schema.yaml", "default: true")
	if got, want := v.ReadFile("notes/work.md"), "- Now @priority(true)\n- Later @priority(false)\n"; got != want {
		t.Fatalf("converted annotations corrupted Markdown:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestSchemaConvertFieldRefusesMissingLiveValueWithNonzeroExit(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      status:
        type: enum
        values: [todo, done]
traits: {}
`).
		WithFile("projects/a.md", "---\ntype: project\nstatus: legacy\n---\n").
		Build()

	result := v.RunCLI(
		"schema", "convert", "field", "project", "status",
		"--map-json", `{"todo":"todo","done":"done"}`,
		"--confirm",
	)
	result.MustFailWithMessage(t, "legacy")
	if result.ExitCode == 0 {
		t.Fatal("expected JSON CLI failure to exit nonzero")
	}
	v.AssertFileContains("schema.yaml", "values: [todo, done]")
	v.AssertFileContains("projects/a.md", "status: legacy")
}

func TestSchemaConvertFieldBoolToEnum(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      status:
        type: bool
traits: {}
`).
		WithFile("projects/a.md", "---\ntype: project\nstatus: true\n---\n").
		Build()

	result := v.RunCLI(
		"schema", "convert", "field", "project", "status",
		"--type", "enum",
		"--map-json", `{"true":"done","false":"todo"}`,
		"--confirm",
	)
	result.MustSucceed(t)
	v.AssertFileContains("schema.yaml", "type: enum")
	v.AssertFileContains("schema.yaml", "- done")
	v.AssertFileContains("schema.yaml", "- todo")
	v.AssertFileContains("projects/a.md", "status: done")
}
