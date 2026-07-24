//go:build integration

package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_ReadObject(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/bob.md", `---
type: person
name: Bob
---
# Bob

Bob is a developer.
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Reindex first to pick up the file
	server.callTool("reindex", nil)

	// Read the object
	result := server.callTool("read", map[string]interface{}{
		"reference": "people/bob.md",
	})

	if result.IsError {
		t.Fatalf("read failed: %s", result.Text)
	}

	// Verify we got the content
	if !strings.Contains(result.Text, "Bob is a developer") {
		t.Errorf("expected content to include 'Bob is a developer', got: %s", result.Text)
	}
}

// TestMCPIntegration_SetFields tests updating object fields via MCP tool call.
func TestMCPIntegration_Search(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/meeting.md", `---
type: page
---
# Weekly Meeting

Discussed the product roadmap and timeline.
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Reindex
	server.callTool("reindex", nil)

	// Search for roadmap
	result := server.callTool("search", map[string]interface{}{
		"query": "roadmap",
	})

	if result.IsError {
		t.Fatalf("search failed: %s", result.Text)
	}

	// Parse response
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data.Items) < 1 {
		t.Errorf("expected at least 1 search result, got %d", len(resp.Data.Items))
	}
}

// TestMCPIntegration_Backlinks tests backlinks retrieval via MCP tool call.
func TestMCPIntegration_Backlinks(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/eve.md", `---
type: person
name: Eve
---
# Eve
`).
		WithFile("projects/secret.md", `---
type: project
title: Secret Project
owner: "[[people/eve]]"
---
# Secret Project

Eve's secret project.
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Reindex
	server.callTool("reindex", nil)

	// Get backlinks for Eve
	result := server.callTool("backlinks", map[string]interface{}{
		"reference": "people/eve",
	})

	if result.IsError {
		t.Fatalf("backlinks failed: %s", result.Text)
	}

	// Parse response - backlinks are in "items" field
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data.Items) != 1 {
		t.Errorf("expected 1 backlink, got %d", len(resp.Data.Items))
	}
}

// TestMCPIntegration_SchemaIntrospection tests schema introspection via MCP.
