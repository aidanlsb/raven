package commandimpl

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestHandleCheckFix_WarnsWhenPlannedFixIsSkipped(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithFile("projects/roadmap.md", `---
type: project
title: Roadmap
owner: "[[people/freya]]"
---`).
		Build()

	result := handleCheckFix(vault.Path, nil, nil, &checksvc.RunResult{
		Issues: []check.Issue{
			{
				Type:     check.IssueShortRefCouldBeFullPath,
				FilePath: "projects/roadmap.md",
				Line:     4,
				Value:    "freya",
			},
		},
		ShortRefs: map[string]string{"freya": "people/freya"},
	}, true)

	if !result.OK {
		t.Fatalf("expected success envelope, got failure: %#v", result.Error)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want 1 warning", result.Warnings)
	}
	if result.Warnings[0].Code != checkApplyIncompleteWarningCode {
		t.Fatalf("warning code = %q, want %q", result.Warnings[0].Code, checkApplyIncompleteWarningCode)
	}

	data := result.Data.(map[string]interface{})
	if got, ok := data["ok"].(bool); !ok || got {
		t.Fatalf("data ok = %#v, want false", data["ok"])
	}
	if got, ok := data["fixed_issues"].(int); !ok || got != 0 {
		t.Fatalf("fixed_issues = %#v, want 0", data["fixed_issues"])
	}
	if got, ok := data["skipped_issues"].(int); !ok || got != 1 {
		t.Fatalf("skipped_issues = %#v, want 1", data["skipped_issues"])
	}

	skipped, ok := data["skipped_items"].([]checksvc.SkippedFix)
	if !ok || len(skipped) != 1 {
		t.Fatalf("skipped_items = %#v, want 1 skipped item", data["skipped_items"])
	}
}

func TestHandleCheckCreateMissing_WarnsWhenPageCreationFails(t *testing.T) {
	t.Parallel()

	result := handleCheckCreateMissing(t.TempDir(), &config.VaultConfig{
		ProtectedPrefixes: []string{"meeting/"},
	}, &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"meeting": {DefaultPath: "meeting/"},
		},
	}, &checksvc.RunResult{
		Scope: checksvc.Scope{Type: "full"},
		MissingRefs: []*check.MissingRef{
			{TargetPath: "all-hands", InferredType: "meeting"},
		},
	}, true)

	if !result.OK {
		t.Fatalf("expected success envelope, got failure: %#v", result.Error)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want 1 warning", result.Warnings)
	}
	if result.Warnings[0].Code != checkApplyIncompleteWarningCode {
		t.Fatalf("warning code = %q, want %q", result.Warnings[0].Code, checkApplyIncompleteWarningCode)
	}

	data := result.Data.(map[string]interface{})
	if got, ok := data["ok"].(bool); !ok || got {
		t.Fatalf("data ok = %#v, want false", data["ok"])
	}
	if got, ok := data["created_pages"].(int); !ok || got != 0 {
		t.Fatalf("created_pages = %#v, want 0", data["created_pages"])
	}
	if got, ok := data["failed_pages"].(int); !ok || got != 1 {
		t.Fatalf("failed_pages = %#v, want 1", data["failed_pages"])
	}

	failed, ok := data["failed_page_items"].([]checksvc.CreateMissingFailure)
	if !ok || len(failed) != 1 {
		t.Fatalf("failed_page_items = %#v, want 1 failure", data["failed_page_items"])
	}
}
