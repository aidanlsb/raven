package commandimpl

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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

	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{})
	result := handleCheckFix(rt, nil, nil, &checksvc.RunResult{
		Issues: []check.Issue{
			{
				Type:     check.IssueShortRefCouldBeFullPath,
				FilePath: "projects/roadmap.md",
				Line:     4,
				Value:    "freya",
			},
		},
		ShortRefs: map[string]string{"freya": "people/freya"},
	}, true, "")

	if !result.OK {
		t.Fatalf("expected success envelope, got failure: %#v", result.Error)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want 1 warning", result.Warnings)
	}
	if result.Warnings[0].Code != checkApplyIncompleteWarningCode {
		t.Fatalf("warning code = %q, want %q", result.Warnings[0].Code, checkApplyIncompleteWarningCode)
	}

	data := result.Data.(commandpayload.CheckFixResult)
	if data.OK {
		t.Fatalf("data ok = true, want false")
	}
	if data.FixedIssues != 0 {
		t.Fatalf("fixed_issues = %d, want 0", data.FixedIssues)
	}
	if data.SkippedIssues != 1 {
		t.Fatalf("skipped_issues = %d, want 1", data.SkippedIssues)
	}

	if len(data.SkippedItems) != 1 {
		t.Fatalf("skipped_items = %#v, want 1 skipped item", data.SkippedItems)
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

	data := result.Data.(commandpayload.CheckCreateMissingResult)
	if data.OK == nil || *data.OK {
		t.Fatalf("data ok = %#v, want false", data.OK)
	}
	if data.CreatedPages == nil || *data.CreatedPages != 0 {
		t.Fatalf("created_pages = %#v, want 0", data.CreatedPages)
	}
	if data.FailedPages == nil || *data.FailedPages != 1 {
		t.Fatalf("failed_pages = %#v, want 1", data.FailedPages)
	}

	if data.FailedPageItems == nil || len(*data.FailedPageItems) != 1 {
		t.Fatalf("failed_page_items = %#v, want 1 failure", data.FailedPageItems)
	}
}
