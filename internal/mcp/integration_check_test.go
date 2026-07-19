//go:build integration

package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_Check(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("notes/broken.md", `---
type: page
---
# Broken Note

References [[nonexistent/page]] which doesn't exist.
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Reindex
	server.callTool("reindex", nil)

	// Run check - note: check command may return IsError=true when issues are found
	result := server.callTool("check", nil)

	// Check returns a different format (not the standard ok/data envelope)
	// The response should contain "issues" and "missing_reference"
	if !strings.Contains(result.Text, "issues") {
		t.Errorf("expected check output to contain 'issues'\nText: %s", result.Text)
	}

	if !strings.Contains(result.Text, "missing_reference") {
		t.Errorf("expected check output to include 'missing_reference' issue\nText: %s", result.Text)
	}
}

func TestMCPIntegration_CheckFixPreviewAndConfirm(t *testing.T) {
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

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	server.callTool("reindex", nil)

	preview := server.callTool("check_fix", map[string]interface{}{})
	if preview.IsError {
		t.Fatalf("expected check fix preview to succeed, got error: %s", preview.Text)
	}

	var previewResp struct {
		OK   bool `json:"ok"`
		Data struct {
			Preview       bool `json:"preview"`
			FixableIssues int  `json:"fixable_issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(preview.Text), &previewResp); err != nil {
		t.Fatalf("failed to parse check fix preview response: %v", err)
	}
	if !previewResp.Data.Preview {
		t.Fatalf("expected preview=true, got %#v", previewResp.Data.Preview)
	}
	if previewResp.Data.FixableIssues < 1 {
		t.Fatalf("expected at least 1 fixable issue, got %d", previewResp.Data.FixableIssues)
	}

	apply := server.callTool("check_fix", map[string]interface{}{
		"confirm": true,
	})
	if apply.IsError {
		t.Fatalf("expected check fix apply to succeed, got error: %s", apply.Text)
	}

	var applyResp struct {
		OK   bool `json:"ok"`
		Data struct {
			Preview     bool `json:"preview"`
			FixedIssues int  `json:"fixed_issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(apply.Text), &applyResp); err != nil {
		t.Fatalf("failed to parse check fix apply response: %v", err)
	}
	if applyResp.Data.Preview {
		t.Fatalf("expected preview=false after apply, got %#v", applyResp.Data.Preview)
	}
	if applyResp.Data.FixedIssues < 1 {
		t.Fatalf("expected at least 1 fixed issue, got %d", applyResp.Data.FixedIssues)
	}

	v.AssertFileContains("projects/roadmap.md", "owner: \"[[people/freya]]\"")
}

// TestMCPIntegration_CheckCreateMissingWithConfirm verifies non-interactive
// create-missing behavior via MCP (JSON mode + confirm=true).
func TestMCPIntegration_CheckCreateMissingWithConfirm(t *testing.T) {
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

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("check create-missing", map[string]interface{}{
		"confirm": true,
	})
	if result.IsError {
		t.Fatalf("expected create-missing to succeed, got error: %s", result.Text)
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Preview      bool `json:"preview"`
			CreatedPages int  `json:"created_pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse create-missing response: %v", err)
	}
	if resp.Data.Preview {
		t.Fatalf("expected preview=false after confirm, got %#v", resp.Data.Preview)
	}
	if resp.Data.CreatedPages != 1 {
		t.Fatalf("expected created_pages=1, got %#v", resp.Data.CreatedPages)
	}

	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
}
