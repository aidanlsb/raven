package schemamigrate

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestConvertTraitEnumToBoolMigratesDefaultAndAnnotations(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, medium, low]
    default: medium
`).
		WithFile("notes/work.md", "- Urgent @priority(high)\n- Later @priority(low)\n").
		Build()

	mapping := map[string]interface{}{"high": true, "medium": true, "low": false}
	beforeSchema := v.ReadFile("schema.yaml")
	beforeNote := v.ReadFile("notes/work.md")
	preview, err := ConvertTrait(ConvertTraitRequest{
		VaultPath: v.Path, TraitName: "priority", TargetType: "bool", Mapping: mapping,
	})
	if err != nil {
		t.Fatalf("preview ConvertTrait: %v", err)
	}
	if !preview.Preview || preview.TotalChanges == 0 {
		t.Fatalf("expected non-empty preview, got %#v", preview)
	}
	if got := v.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatal("preview changed schema.yaml")
	}
	if got := v.ReadFile("notes/work.md"); got != beforeNote {
		t.Fatal("preview changed Markdown")
	}

	applied, err := ConvertTrait(ConvertTraitRequest{
		VaultPath: v.Path, TraitName: "priority", TargetType: "bool", Mapping: mapping, Confirm: true,
	})
	if err != nil {
		t.Fatalf("apply ConvertTrait: %v", err)
	}
	if applied.ChangesApplied != preview.TotalChanges {
		t.Fatalf("preview total=%d, applied=%d", preview.TotalChanges, applied.ChangesApplied)
	}
	v.AssertFileContains("schema.yaml", "type: bool")
	v.AssertFileContains("schema.yaml", "default: true")
	v.AssertFileNotContains("schema.yaml", "values:")
	if got, want := v.ReadFile("notes/work.md"), "- Urgent @priority(true)\n- Later @priority(false)\n"; got != want {
		t.Fatalf("converted annotations corrupted Markdown:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestConvertFieldBoolToEnumMigratesFrontmatter(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      status:
        type: bool
traits: {}
`).
		WithFile("projects/a.md", "---\ntype: project\nstatus: true\n---\n# A\n").
		WithFile("projects/b.md", "---\ntype: project\nstatus: false\n---\n# B\n").
		Build()

	result, err := ConvertField(ConvertFieldRequest{
		VaultPath:  v.Path,
		TypeName:   "project",
		FieldName:  "status",
		TargetType: "enum",
		Mapping:    map[string]interface{}{"true": "done", "false": "todo"},
		Confirm:    true,
	})
	if err != nil {
		t.Fatalf("ConvertField: %v", err)
	}
	if result.ChangesApplied == 0 {
		t.Fatal("expected applied changes")
	}
	v.AssertFileContains("schema.yaml", "type: enum")
	v.AssertFileContains("schema.yaml", "- done")
	v.AssertFileContains("schema.yaml", "- todo")
	v.AssertFileContains("projects/a.md", "status: done")
	v.AssertFileContains("projects/b.md", "status: todo")
}

func TestConvertSameTypeEnumRemap(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [urgent, high, medium, low]
`).
		WithFile("notes/work.md", "- Fix now @priority(urgent)\n- Keep @priority(high)\n").
		Build()

	_, err := ConvertTrait(ConvertTraitRequest{
		VaultPath: v.Path,
		TraitName: "priority",
		Mapping: map[string]interface{}{
			"urgent": "critical",
			"high":   "high",
			"medium": "medium",
			"low":    "low",
		},
		Confirm: true,
	})
	if err != nil {
		t.Fatalf("ConvertTrait: %v", err)
	}
	v.AssertFileContains("schema.yaml", "- critical")
	v.AssertFileNotContains("schema.yaml", "- urgent")
	v.AssertFileContains("notes/work.md", "@priority(critical)")
	v.AssertFileContains("notes/work.md", "@priority(high)")
}

func TestConvertRequiresEverySchemaAllowedValue(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, medium, low]
`).
		Build()

	_, err := ConvertTrait(ConvertTraitRequest{
		VaultPath:  v.Path,
		TraitName:  "priority",
		TargetType: "bool",
		Mapping:    map[string]interface{}{"high": true, "medium": true},
	})
	assertMissingMappingValue(t, err, "low")
}

func TestConvertRequiresObservedOutlierValue(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, low]
`).
		WithFile("notes/work.md", "- Existing invalid value @priority(urgent)\n").
		Build()

	_, err := ConvertTrait(ConvertTraitRequest{
		VaultPath:  v.Path,
		TraitName:  "priority",
		TargetType: "bool",
		Mapping:    map[string]interface{}{"high": true, "low": false},
		Confirm:    true,
	})
	assertMissingMappingValue(t, err, "urgent")
	v.AssertFileContains("schema.yaml", "type: enum")
	v.AssertFileContains("notes/work.md", "@priority(urgent)")
}

func TestConvertEnumArrayMapsEachMember(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      states:
        type: enum[]
        values: [todo, blocked, done]
traits: {}
`).
		WithFile("projects/a.md", "---\ntype: project\nstates: [todo, blocked]\n---\n").
		Build()

	_, err := ConvertField(ConvertFieldRequest{
		VaultPath: v.Path,
		TypeName:  "project",
		FieldName: "states",
		Mapping: map[string]interface{}{
			"todo":    "open",
			"blocked": "blocked",
			"done":    "closed",
		},
		Confirm: true,
	})
	if err != nil {
		t.Fatalf("ConvertField: %v", err)
	}
	v.AssertFileContains("schema.yaml", "- open")
	v.AssertFileContains("schema.yaml", "- closed")
	content := v.ReadFile("projects/a.md")
	if !strings.Contains(content, "- open") || !strings.Contains(content, "- blocked") {
		t.Fatalf("expected member-wise array conversion, got:\n%s", content)
	}
}

func TestConvertRejectsWrongJSONRepresentationAndNull(t *testing.T) {
	t.Parallel()

	t.Run("trait bool requires JSON boolean", func(t *testing.T) {
		t.Parallel()
		v := testutil.NewTestVault(t).
			WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, low]
`).
			Build()
		_, err := ConvertTrait(ConvertTraitRequest{
			VaultPath:  v.Path,
			TraitName:  "priority",
			TargetType: "bool",
			Mapping:    map[string]interface{}{"high": "true", "low": "false"},
		})
		if err == nil || !strings.Contains(err.Error(), "JSON boolean") {
			t.Fatalf("expected strict JSON boolean error, got %v", err)
		}
	})

	t.Run("array members cannot map to null", func(t *testing.T) {
		t.Parallel()
		v := testutil.NewTestVault(t).
			WithSchema(`version: 1
types:
  project:
    fields:
      labels:
        type: string[]
traits: {}
`).
			WithFile("projects/a.md", "---\ntype: project\nlabels: [old]\n---\n").
			Build()
		_, err := ConvertField(ConvertFieldRequest{
			VaultPath: v.Path,
			TypeName:  "project",
			FieldName: "labels",
			Mapping:   map[string]interface{}{"old": nil},
		})
		if err == nil || !strings.Contains(err.Error(), "null is not a schema value type") {
			t.Fatalf("expected null rejection, got %v", err)
		}
	})

	t.Run("trait values cannot contain backticks", func(t *testing.T) {
		t.Parallel()
		v := testutil.NewTestVault(t).
			WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, low]
`).
			Build()
		_, err := ConvertTrait(ConvertTraitRequest{
			VaultPath: v.Path,
			TraitName: "priority",
			Mapping:   map[string]interface{}{"high": "`code`", "low": "low"},
		})
		if err == nil || !strings.Contains(err.Error(), "backticks") {
			t.Fatalf("expected backtick rejection, got %v", err)
		}
	})
}

func TestConvertSkipsExcludedMarkdown(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  priority:
    type: enum
    values: [high, low]
`).
		WithRavenYAML("exclude:\n  - ignored/\n").
		WithFile("notes/work.md", "- Included @priority(high)\n").
		WithFile("ignored/work.md", "- Excluded outlier @priority(urgent)\n").
		Build()

	_, err := ConvertTrait(ConvertTraitRequest{
		VaultPath: v.Path,
		TraitName: "priority",
		Mapping:   map[string]interface{}{"high": "critical", "low": "low"},
		Confirm:   true,
	})
	if err != nil {
		t.Fatalf("ConvertTrait: %v", err)
	}
	v.AssertFileContains("notes/work.md", "@priority(critical)")
	v.AssertFileContains("ignored/work.md", "@priority(urgent)")
	v.AssertFileNotContains("ignored/work.md", "@priority(critical)")
}

func TestConvertFieldRejectsNonReferenceToReferenceWithoutTarget(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      owner:
        type: string
traits: {}
`).
		Build()

	_, err := ConvertField(ConvertFieldRequest{
		VaultPath:  v.Path,
		TypeName:   "project",
		FieldName:  "owner",
		TargetType: "ref",
		Mapping:    map[string]interface{}{"alice": "[[people/alice]]"},
	})
	if err == nil || !strings.Contains(err.Error(), "without a reference target") {
		t.Fatalf("expected reference-target error, got %v", err)
	}
}

func assertMissingMappingValue(t *testing.T, err error, value string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected missing mapping error for %q", value)
	}
	var svcErr *schemasvc.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected schemasvc.Error, got %T: %v", err, err)
	}
	if svcErr.Code != schemasvc.ErrorInvalidInput {
		t.Fatalf("error code=%s, want %s", svcErr.Code, schemasvc.ErrorInvalidInput)
	}
	if !strings.Contains(svcErr.Message, value) {
		t.Fatalf("error %q does not mention missing value %q", svcErr.Message, value)
	}
}
