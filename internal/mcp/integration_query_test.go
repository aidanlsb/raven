//go:build integration

package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_QueryObjects(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create some objects first
	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project A",
		"field": map[string]interface{}{"status": "active"},
	})
	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project B",
		"field": map[string]interface{}{"status": "done"},
	})

	// Query for active projects - uses == for equality
	result := server.callTool("query", map[string]interface{}{
		"query_string": "type:project .status==active",
	})

	if result.IsError {
		t.Fatalf("query failed: %s", result.Text)
	}

	// Parse the response to verify we got results - results are in "items"
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
		t.Errorf("expected 1 result, got %d", len(resp.Data.Items))
	}
}

// TestMCPIntegration_QuerySavedQueryInlineArgs tests MCP query_string containing
// "<saved-query-name> <inputs...>" in a single string argument.
func TestMCPIntegration_QuerySavedQueryInlineArgs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-by-status:
    query: "type:project .status=={{args.status}}"
    args: [status]
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create some objects first
	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project A",
		"field": map[string]interface{}{"status": "active"},
	})
	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project B",
		"field": map[string]interface{}{"status": "done"},
	})

	// MCP passes query_string as one arg; ensure saved query + inline input works.
	result := server.callTool("query", map[string]interface{}{
		"query_string": "project-by-status active",
	})

	if result.IsError {
		t.Fatalf("query failed: %s", result.Text)
	}

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
		t.Errorf("expected 1 result, got %d", len(resp.Data.Items))
	}
}

func TestMCPIntegration_QuerySavedQueryInlineQuotedArgs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-by-name:
    query: 'type:project .title=="{{args.name}}"'
    args: [name]
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "raven app",
	})
	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "other app",
	})

	tests := []string{
		`project-by-name "raven app"`,
		`project-by-name name="raven app"`,
	}

	for _, queryString := range tests {
		result := server.callTool("query", map[string]interface{}{
			"query_string": queryString,
		})
		if result.IsError {
			t.Fatalf("query %q failed: %s", queryString, result.Text)
		}

		var resp struct {
			OK   bool `json:"ok"`
			Data struct {
				Items []map[string]interface{} `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
			t.Fatalf("failed to parse response for %q: %v", queryString, err)
		}
		if len(resp.Data.Items) != 1 {
			t.Fatalf("query %q expected 1 result, got %d", queryString, len(resp.Data.Items))
		}
		if resp.Data.Items[0]["id"] != "projects/raven-app" {
			t.Fatalf("query %q returned unexpected item: %#v", queryString, resp.Data.Items[0])
		}
	}
}

func TestMCPIntegration_QuerySavedQueryTypedInputs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-by-status:
    query: "type:project .status=={{args.status}}"
    args: [status]
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project A",
		"field": map[string]interface{}{"status": "active"},
	})
	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project B",
		"field": map[string]interface{}{"status": "done"},
	})

	result := server.callTool("query", map[string]interface{}{
		"query_string": "project-by-status",
		"inputs": map[string]interface{}{
			"status": "active",
		},
	})

	if result.IsError {
		t.Fatalf("query failed: %s", result.Text)
	}

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
		t.Errorf("expected 1 result, got %d", len(resp.Data.Items))
	}
}

func TestMCPIntegration_QuerySavedQueryAllowsUnusedDeclaredArgs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-by-status:
    query: "type:project .status=={{args.status}}"
    args: [status, project]
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Project A",
		"field": map[string]interface{}{"status": "active"},
	})

	result := server.callTool("query", map[string]interface{}{
		"query_string": "project-by-status",
		"inputs": map[string]interface{}{
			"status": "active",
		},
	})

	if result.IsError {
		t.Fatalf("query failed: %s", result.Text)
	}

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
		t.Errorf("expected 1 result, got %d", len(resp.Data.Items))
	}
}

// TestMCPIntegration_ReadObject tests reading an object via MCP tool call.
func TestMCPIntegration_QueryRefreshRemovesDeletedFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	server.callTool("reindex", nil)

	if err := os.Remove(filepath.Join(v.Path, "people/alice.md")); err != nil {
		t.Fatalf("failed to remove person file: %v", err)
	}

	result := server.callTool("query", map[string]interface{}{
		"query_string": "type:person",
		"refresh":      true,
	})
	if result.IsError {
		t.Fatalf("expected query refresh to succeed, got error: %s", result.Text)
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []interface{} `json:"items"`
			Total int           `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse query refresh response: %v", err)
	}
	if resp.Data.Total != 0 || len(resp.Data.Items) != 0 {
		t.Fatalf("expected deleted file to be removed from refreshed query, got total=%d items=%d", resp.Data.Total, len(resp.Data.Items))
	}
}

// TestMCPIntegration_ErrorHandling tests that MCP errors are properly surfaced.
