//go:build integration

package mcp_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/mcp"
	"github.com/aidanlsb/raven/internal/testutil"
)

// TestMCPIntegration_ToolsList tests that the MCP server returns tool schemas.
func TestMCPIntegration_ToolsList(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := mcp.NewServerWithExecutable(v.Path, binary)

	request := mcp.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	var output bytes.Buffer
	server.SetIO(strings.NewReader(""), &output)
	server.HandleRequest(&request)

	var response struct {
		Result struct {
			Tools []mcp.Tool `json:"tools"`
		} `json:"result"`
		Error *mcp.RPCError `json:"error,omitempty"`
	}
	if err := json.NewDecoder(&output).Decode(&response); err != nil {
		t.Fatalf("failed to parse tools/list response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tools/list returned error: %s", response.Error.Message)
	}

	tools := response.Result.Tools
	if len(tools) != 3 {
		t.Fatalf("expected 3 compact tools, got %d", len(tools))
	}

	// Verify compact tools exist
	expectedTools := []string{"raven_discover", "raven_describe", "raven_invoke"}
	foundTools := make(map[string]bool)
	for _, tool := range tools {
		foundTools[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !foundTools[expected] {
			t.Errorf("expected tool %s not found in tool list", expected)
		}
	}
}

func TestMCPIntegration_InvokeVaultPathOverride(t *testing.T) {
	t.Parallel()
	primary := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	override := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, primary.Path, binary)

	result := server.callTool("raven_invoke", map[string]interface{}{
		"command":    "new",
		"vault_path": override.Path,
		"args": map[string]interface{}{
			"type":  "person",
			"title": "Vault Override",
		},
	})
	if result.IsError {
		t.Fatalf("expected invoke success, got error: %s", result.Text)
	}
	if primary.FileExists("people/vault-override.md") {
		t.Fatal("expected pinned vault to remain unchanged")
	}
	if !override.FileExists("people/vault-override.md") {
		t.Fatal("expected object to be created in override vault")
	}
}

// TestMCPIntegration_ServeRejectsLegacyToolNames ensures the live `rvn serve`
// JSON-RPC path only accepts compact tools and rejects legacy names.
func TestMCPIntegration_ServeRejectsLegacyToolNames(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	binary := testutil.BuildCLI(t)

	requests := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"raven_new","arguments":{"type":"page","title":"Legacy Path"}}}`,
	}, "\n") + "\n"

	cmd := exec.Command(binary, "--vault-path", v.Path, "serve")
	cmd.Stdin = strings.NewReader(requests)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("serve command failed: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 JSON-RPC responses, got %d\nstdout: %s", len(lines), stdout.String())
	}

	var initResp struct {
		Result map[string]interface{} `json:"result"`
		Error  *mcp.RPCError          `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed to parse initialize response: %v\nraw: %s", err, lines[0])
	}
	if initResp.Error != nil {
		t.Fatalf("initialize returned rpc error: %+v", initResp.Error)
	}
	serverInfo, ok := initResp.Result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("initialize missing serverInfo: %#v", initResp.Result)
	}
	version, _ := serverInfo["version"].(string)
	wantVersion := maintsvc.CurrentVersionInfoFromExecutable(binary).Version
	if version != wantVersion {
		t.Fatalf("initialize serverInfo.version=%q, want %q", version, wantVersion)
	}

	var toolCallResp struct {
		Result mcp.ToolResult `json:"result"`
		Error  *mcp.RPCError  `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &toolCallResp); err != nil {
		t.Fatalf("failed to parse tools/call response: %v\nraw: %s", err, lines[1])
	}
	if toolCallResp.Error != nil {
		t.Fatalf("tools/call returned rpc error: %+v", toolCallResp.Error)
	}
	if !toolCallResp.Result.IsError {
		t.Fatalf("expected tools/call isError=true for legacy tool name\nresponse: %s", lines[1])
	}
	if len(toolCallResp.Result.Content) == 0 {
		t.Fatalf("expected tool response content, got none: %s", lines[1])
	}

	env := parseMCPEnvelope(t, toolCallResp.Result.Content[0].Text)
	if env.Error == nil || env.Error.Code != "UNKNOWN_TOOL" {
		t.Fatalf("expected UNKNOWN_TOOL envelope error, got: %s", toolCallResp.Result.Content[0].Text)
	}
	if env.Error.Suggestion != "Call raven_discover to list available tools" {
		t.Fatalf("unexpected UNKNOWN_TOOL suggestion: %q", env.Error.Suggestion)
	}
}

// TestMCPIntegration_CreateObject tests creating an object via MCP tool call.
func TestMCPIntegration_CreateObject(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)

	// Create a test server that uses our built binary
	server := newTestServer(t, v.Path, binary)

	// Call the raven_new tool
	result := server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
		"field": map[string]interface{}{
			"email": "alice@example.com",
		},
	})

	if result.IsError {
		t.Fatalf("tool call failed: %s", result.Text)
	}

	// Verify the file was created
	v.AssertFileExists("people/alice.md")
	v.AssertFileContains("people/alice.md", "name: Alice")
}

func TestMCPIntegration_SchemaAddTypeDefaultsPathToTypeName(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("schema_add_type", map[string]interface{}{
		"name": "meeting",
	})

	if result.IsError {
		t.Fatalf("tool call failed: %s", result.Text)
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			DefaultPath string `json:"default_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Data.DefaultPath != "meeting/" {
		t.Fatalf("expected default_path %q, got %q", "meeting/", resp.Data.DefaultPath)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meeting/")
}

// TestMCPIntegration_CreatePageWithObjectRootFallback verifies that when
// directories.page is omitted, it defaults to directories.object for creation.
func TestMCPIntegration_CreatePageWithObjectRootFallback(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`directories:
  object: objects/
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("new", map[string]interface{}{
		"type":  "page",
		"title": "Scratch Note",
	})

	if result.IsError {
		t.Fatalf("tool call failed: %s", result.Text)
	}

	v.AssertFileExists("objects/scratch-note.md")
	v.AssertFileNotExists("scratch-note.md")
}

// TestMCPIntegration_QueryObjects tests querying objects via MCP tool call.
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
		"query_string": "object:project .status==active",
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
    query: "object:project .status=={{args.status}}"
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
    query: 'object:project .title=="{{args.name}}"'
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
    query: "object:project .status=={{args.status}}"
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
    query: "object:project .status=={{args.status}}"
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
		"path": "people/bob.md",
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
func TestMCPIntegration_SetFields(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create a person
	server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Carol",
	})

	// Update the email field
	result := server.callTool("set", map[string]interface{}{
		"object_id": "people/carol",
		"fields": map[string]interface{}{
			"email": "carol@example.com",
		},
	})

	if result.IsError {
		t.Fatalf("set failed: %s", result.Text)
	}

	// Verify the file was updated
	v.AssertFileContains("people/carol.md", "email: carol@example.com")
}

// TestMCPIntegration_StringEncodedStructuredInputs verifies strict invoke typing:
// string-encoded structured payloads are rejected, while typed objects succeed.
func TestMCPIntegration_StringEncodedStructuredInputs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: notes/
    name_field: title
    fields:
      title:
        type: string
        required: true
  project:
    default_path: projects/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: enum
        values: [active, done]
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// JSON-typed flag provided as JSON string is rejected.
	upsertInvalid := server.callTool("upsert", map[string]interface{}{
		"type":       "project",
		"title":      "MCP Compat Project",
		"field-json": `{"status":"active"}`,
	})
	upsertInvalidEnv := parseMCPEnvelope(t, upsertInvalid.Text)
	if !upsertInvalid.IsError || upsertInvalidEnv.OK {
		t.Fatalf("expected upsert field-json string to fail, got: %s", upsertInvalid.Text)
	}
	if upsertInvalidEnv.Error == nil || upsertInvalidEnv.Error.Code != "INVALID_ARGS" {
		t.Fatalf("expected INVALID_ARGS for upsert field-json string, got: %s", upsertInvalid.Text)
	}

	// JSON-typed flag provided as an object succeeds.
	upsertValid := server.callTool("upsert", map[string]interface{}{
		"type":       "project",
		"title":      "MCP Compat Project",
		"field-json": map[string]interface{}{"status": "active"},
	})
	if upsertValid.IsError {
		t.Fatalf("upsert with field-json object failed: %s", upsertValid.Text)
	}
	v.AssertFileContains("projects/mcp-compat-project.md", "status: active")

	// Key-value map provided as a single "k=v" string is rejected.
	setInvalid := server.callTool("set", map[string]interface{}{
		"object_id": "projects/mcp-compat-project",
		"fields":    "status=done",
	})
	setInvalidEnv := parseMCPEnvelope(t, setInvalid.Text)
	if !setInvalid.IsError || setInvalidEnv.OK {
		t.Fatalf("expected set fields string to fail, got: %s", setInvalid.Text)
	}
	if setInvalidEnv.Error == nil || setInvalidEnv.Error.Code != "INVALID_ARGS" {
		t.Fatalf("expected INVALID_ARGS for set fields string, got: %s", setInvalid.Text)
	}

	// Typed object for fields succeeds.
	setValid := server.callTool("set", map[string]interface{}{
		"object_id": "projects/mcp-compat-project",
		"fields":    map[string]interface{}{"status": "done"},
	})
	if setValid.IsError {
		t.Fatalf("set with fields object failed: %s", setValid.Text)
	}
	v.AssertFileContains("projects/mcp-compat-project.md", "status: done")
}

// TestMCPIntegration_EditDeleteWithEmptyString tests deleting text via raven_edit with empty new_str.
func TestMCPIntegration_EditDeleteWithEmptyString(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-01-02.md", `---
type: page
---
# Daily

- old task
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("edit", map[string]interface{}{
		"path":    "daily/2026-01-02.md",
		"old_str": "- old task",
		"new_str": "",
		"confirm": true,
	})

	if result.IsError {
		t.Fatalf("edit delete failed: %s", result.Text)
	}

	v.AssertFileNotContains("daily/2026-01-02.md", "- old task")
}

// TestMCPIntegration_DeleteObject tests deleting an object via MCP tool call.
func TestMCPIntegration_DeleteObject(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create an object
	server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Dave",
	})
	v.AssertFileExists("people/dave.md")

	// Delete it
	result := server.callTool("delete", map[string]interface{}{
		"object_id": "people/dave",
		"force":     true,
	})

	if result.IsError {
		t.Fatalf("delete failed: %s", result.Text)
	}

	// Verify it was deleted (moved to trash)
	v.AssertFileNotExists("people/dave.md")
}

// TestMCPIntegration_Search tests full-text search via MCP tool call.
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
			Results []interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data.Results) < 1 {
		t.Errorf("expected at least 1 search result, got %d", len(resp.Data.Results))
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
		"target": "people/eve",
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
func TestMCPIntegration_SchemaIntrospection(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Get schema types
	result := server.callTool("schema", map[string]interface{}{
		"subcommand": "types",
	})

	if result.IsError {
		t.Fatalf("schema introspection failed: %s", result.Text)
	}

	// Verify person and project types are in the output
	if !strings.Contains(result.Text, "person") || !strings.Contains(result.Text, "project") {
		t.Errorf("expected schema to include person and project types, got: %s", result.Text)
	}

	// Get details for one type using explicit positional args.
	typeResult := server.callTool("schema", map[string]interface{}{
		"subcommand": "type",
		"name":       "person",
	})

	if typeResult.IsError {
		t.Fatalf("schema type introspection failed: %s", typeResult.Text)
	}

	var typeResp struct {
		Data struct {
			Type struct {
				Name string `json:"name"`
			} `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(typeResult.Text), &typeResp); err != nil {
		t.Fatalf("failed to parse schema type response: %v\n%s", err, typeResult.Text)
	}
	if typeResp.Data.Type.Name != "person" {
		t.Errorf("expected type details for person, got: %s", typeResult.Text)
	}
}

// TestMCPIntegration_SchemaFieldDescriptionsViaToolCall verifies schema field
// descriptions can be added/updated/removed through MCP tools/call.
func TestMCPIntegration_SchemaFieldDescriptionsViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Add a new field with a description.
	addFieldResult := server.callTool("schema_add_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "website",
		"type":        "string",
		"description": "Primary website URL",
	})
	if addFieldResult.IsError {
		t.Fatalf("schema add field failed: %s", addFieldResult.Text)
	}
	v.AssertFileContains("schema.yaml", "website:")
	v.AssertFileContains("schema.yaml", "description: Primary website URL")

	// Update existing field description.
	updateFieldResult := server.callTool("schema_update_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "email",
		"description": "Primary contact email",
	})
	if updateFieldResult.IsError {
		t.Fatalf("schema update field failed: %s", updateFieldResult.Text)
	}
	v.AssertFileContains("schema.yaml", "description: Primary contact email")

	// Remove the description with "-" sentinel.
	removeDescriptionResult := server.callTool("schema_update_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "email",
		"description": "-",
	})
	if removeDescriptionResult.IsError {
		t.Fatalf("schema update field remove description failed: %s", removeDescriptionResult.Text)
	}
	v.AssertFileNotContains("schema.yaml", "description: Primary contact email")
}

// TestMCPIntegration_SchemaFieldEnumValuesViaToolCall verifies enum field values
// can be updated via MCP without removing/recreating the field.
func TestMCPIntegration_SchemaFieldEnumValuesViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	updateFieldResult := server.callTool("schema_update_field", map[string]interface{}{
		"type_name":  "project",
		"field_name": "status",
		"values":     "active,paused,done,archived",
	})
	if updateFieldResult.IsError {
		t.Fatalf("schema update field values failed: %s", updateFieldResult.Text)
	}

	v.AssertFileContains("schema.yaml", "status:")
	v.AssertFileContains("schema.yaml", "- archived")
}

func TestMCPIntegration_SchemaUpdateTypeAndTraitViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	updateTypeResult := server.callTool("schema_update_type", map[string]interface{}{
		"name":        "project",
		"description": "Tracked work items",
		"add-trait":   "priority",
	})
	if updateTypeResult.IsError {
		t.Fatalf("schema update type failed: %s", updateTypeResult.Text)
	}
	v.AssertFileContains("schema.yaml", "description: Tracked work items")
	v.AssertFileContains("schema.yaml", "- priority")

	updateTraitResult := server.callTool("schema_update_trait", map[string]interface{}{
		"name":   "priority",
		"values": "low,medium,high,critical",
	})
	if updateTraitResult.IsError {
		t.Fatalf("schema update trait failed: %s", updateTraitResult.Text)
	}
	v.AssertFileContains("schema.yaml", "- critical")
}

func TestMCPIntegration_SchemaRemoveTypeAndTraitWarningsViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	createProject := server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Apollo",
	})
	if createProject.IsError {
		t.Fatalf("schema remove setup (project create) failed: %s", createProject.Text)
	}

	addTraitUsage := server.callTool("add", map[string]interface{}{
		"text": "@priority(high)",
		"to":   "projects/apollo.md",
	})
	if addTraitUsage.IsError {
		t.Fatalf("schema remove setup (trait usage) failed: %s", addTraitUsage.Text)
	}

	removeType := server.callTool("schema_remove_type", map[string]interface{}{
		"name": "project",
	})
	if removeType.IsError {
		t.Fatalf("schema remove type failed: %s", removeType.Text)
	}

	var removeTypeResp struct {
		OK       bool `json:"ok"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(removeType.Text), &removeTypeResp); err != nil {
		t.Fatalf("failed to parse schema remove type response: %v", err)
	}
	if !removeTypeResp.OK {
		t.Fatalf("expected ok=true in schema remove type response: %s", removeType.Text)
	}
	if len(removeTypeResp.Warnings) == 0 || removeTypeResp.Warnings[0].Code != "ORPHANED_FILES" {
		t.Fatalf("expected ORPHANED_FILES warning, got: %s", removeType.Text)
	}
	v.AssertFileNotContains("schema.yaml", "project:")

	removeTrait := server.callTool("schema_remove_trait", map[string]interface{}{
		"name": "priority",
	})
	if removeTrait.IsError {
		t.Fatalf("schema remove trait failed: %s", removeTrait.Text)
	}

	var removeTraitResp struct {
		OK       bool `json:"ok"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(removeTrait.Text), &removeTraitResp); err != nil {
		t.Fatalf("failed to parse schema remove trait response: %v", err)
	}
	if !removeTraitResp.OK {
		t.Fatalf("expected ok=true in schema remove trait response: %s", removeTrait.Text)
	}
	if len(removeTraitResp.Warnings) == 0 || removeTraitResp.Warnings[0].Code != "ORPHANED_TRAITS" {
		t.Fatalf("expected ORPHANED_TRAITS warning, got: %s", removeTrait.Text)
	}
	v.AssertFileNotContains("schema.yaml", "priority:")
}

// TestMCPIntegration_SchemaRenameTypeWithDefaultPathRename verifies MCP JSON
// preview/apply behavior for type rename with optional default_path directory
// migration.
func TestMCPIntegration_SchemaRenameTypeWithDefaultPathRename(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  event:
    default_path: events/
    fields:
      title: { type: string }
  project:
    default_path: projects/
    fields:
      kickoff:
        type: ref
        target: event
traits: {}
`).
		WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
		WithFile("events/planning.md", `---
type: event
title: Planning
---
# Planning
`).
		WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
Planning: [[events/planning|Planning]]
::project(kickoff=events/kickoff)
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	preview := server.callTool("schema_rename_type", map[string]interface{}{
		"old_name": "event",
		"new_name": "meeting",
	})
	if preview.IsError {
		t.Fatalf("schema rename type preview failed: %s", preview.Text)
	}

	var previewResp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(preview.Text), &previewResp); err != nil {
		t.Fatalf("failed to parse preview response: %v\nraw: %s", err, preview.Text)
	}
	if !previewResp.OK {
		t.Fatalf("expected preview ok=true, got: %s", preview.Text)
	}
	if got, _ := previewResp.Data["preview"].(bool); !got {
		t.Fatalf("expected preview=true, got: %#v", previewResp.Data["preview"])
	}
	if got, _ := previewResp.Data["default_path_rename_available"].(bool); !got {
		t.Fatalf("expected default_path_rename_available=true, got: %#v", previewResp.Data["default_path_rename_available"])
	}
	if got, _ := previewResp.Data["default_path_old"].(string); got != "events/" {
		t.Fatalf("expected default_path_old=events/, got: %#v", previewResp.Data["default_path_old"])
	}
	if got, _ := previewResp.Data["default_path_new"].(string); got != "meetings/" {
		t.Fatalf("expected default_path_new=meetings/, got: %#v", previewResp.Data["default_path_new"])
	}

	v.AssertFileExists("events/kickoff.md")
	v.AssertFileExists("events/planning.md")

	apply := server.callTool("schema_rename_type", map[string]interface{}{
		"old_name":            "event",
		"new_name":            "meeting",
		"confirm":             true,
		"rename_default_path": true, // underscore variant should normalize
	})
	if apply.IsError {
		t.Fatalf("schema rename type apply failed: %s", apply.Text)
	}

	var applyResp struct {
		OK   bool `json:"ok"`
		Data struct {
			DefaultPathRenamed    bool   `json:"default_path_renamed"`
			DefaultPathOld        string `json:"default_path_old"`
			DefaultPathNew        string `json:"default_path_new"`
			FilesMoved            int    `json:"files_moved"`
			ReferenceFilesUpdated int    `json:"reference_files_updated"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(apply.Text), &applyResp); err != nil {
		t.Fatalf("failed to parse apply response: %v\nraw: %s", err, apply.Text)
	}
	if !applyResp.OK {
		t.Fatalf("expected apply ok=true, got: %s", apply.Text)
	}
	if !applyResp.Data.DefaultPathRenamed {
		t.Fatalf("expected default_path_renamed=true, got false")
	}
	if applyResp.Data.DefaultPathOld != "events/" {
		t.Fatalf("expected default_path_old=events/, got %q", applyResp.Data.DefaultPathOld)
	}
	if applyResp.Data.DefaultPathNew != "meetings/" {
		t.Fatalf("expected default_path_new=meetings/, got %q", applyResp.Data.DefaultPathNew)
	}
	if applyResp.Data.FilesMoved != 2 {
		t.Fatalf("expected files_moved=2, got %d", applyResp.Data.FilesMoved)
	}
	if applyResp.Data.ReferenceFilesUpdated < 1 {
		t.Fatalf("expected reference_files_updated>=1, got %d", applyResp.Data.ReferenceFilesUpdated)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meetings/")
	v.AssertFileContains("schema.yaml", "target: meeting")
	v.AssertFileNotContains("schema.yaml", "\n  event:\n")

	v.AssertFileExists("meetings/kickoff.md")
	v.AssertFileExists("meetings/planning.md")
	v.AssertFileNotExists("events/kickoff.md")
	v.AssertFileNotExists("events/planning.md")
	v.AssertFileContains("meetings/kickoff.md", "type: meeting")
	v.AssertFileContains("meetings/planning.md", "type: meeting")

	v.AssertFileContains("projects/roadmap.md", "kickoff: meetings/kickoff")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/kickoff]]")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/planning|Planning]]")
	v.AssertFileContains("projects/roadmap.md", "::project(kickoff=meetings/kickoff)")
	v.AssertFileNotContains("projects/roadmap.md", "events/kickoff")
	v.AssertFileNotContains("projects/roadmap.md", "events/planning")
}

// TestMCPIntegration_Check tests vault check via MCP tool call.
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

	preview := server.callTool("check", map[string]interface{}{
		"fix": true,
	})
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

	apply := server.callTool("check", map[string]interface{}{
		"fix":     true,
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
  object: objects/
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

	result := server.callTool("check", map[string]interface{}{
		"create-missing": true,
		"confirm":        true,
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
		"query_string": "object:person",
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
func TestMCPIntegration_ErrorHandling(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create a file first
	server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
	})

	// Try to create a duplicate (should fail)
	result := server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
	})

	// Should be an error
	if !result.IsError {
		t.Fatalf("expected error for duplicate object, got success: %s", result.Text)
	}

	// Parse the error response
	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if resp.Error.Code != "FILE_EXISTS" {
		t.Errorf("expected error code FILE_EXISTS, got %s", resp.Error.Code)
	}
}

func TestMCPIntegration_QueryParseErrorsIncludeSuggestion(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("query", map[string]interface{}{
		"query_string": "from issue where",
	})
	if !result.IsError {
		t.Fatalf("expected query parse failure, got success: %s", result.Text)
	}

	env := parseMCPEnvelope(t, result.Text)
	if env.Error == nil || env.Error.Code != "QUERY_INVALID" {
		t.Fatalf("expected QUERY_INVALID, got: %s", result.Text)
	}
	if env.Error.Suggestion != "Check the query syntax, quote string literals, and retry." {
		t.Fatalf("unexpected query suggestion: %q", env.Error.Suggestion)
	}
}

func TestMCPIntegration_SearchSyntaxErrorsReturnInvalidInput(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("search", map[string]interface{}{
		"query": "@broken",
	})
	if !result.IsError {
		t.Fatalf("expected search syntax failure, got success: %s", result.Text)
	}

	env := parseMCPEnvelope(t, result.Text)
	if env.Error == nil || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT, got: %s", result.Text)
	}
	if env.Error.Message != "invalid search query" {
		t.Fatalf("unexpected search error message: %q", env.Error.Message)
	}
	if env.Error.Suggestion != "Quote special characters or use a simpler full-text query and retry." {
		t.Fatalf("unexpected search suggestion: %q", env.Error.Suggestion)
	}
}

func TestMCPIntegration_DirectDispatchParityWithCLI(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)

	t.Run("new", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Person",
			"field": map[string]interface{}{
				"email": "parity@example.com",
			},
		})
		cliResult := vCLI.RunCLI("new", "person", "Parity Person", "--field", "email=parity@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "id", "title", "type"})
	})

	t.Run("upsert", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("upsert", map[string]interface{}{
			"type":    "project",
			"title":   "Parity Project",
			"field":   map[string]interface{}{"status": "active"},
			"content": "# Parity Body",
		})
		cliResult := vCLI.RunCLI("upsert", "project", "Parity Project", "--field", "status=active", "--content", "# Parity Body")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"status", "id", "file", "type", "title"})
	})

	t.Run("add", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Add",
		})
		vCLI.RunCLI("new", "person", "Parity Add").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"text": "Parity add content",
			"to":   "people/parity-add",
		})
		cliResult := vCLI.RunCLI("add", "Parity add content", "--to", "people/parity-add")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "line", "content"})
	})

	t.Run("add_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Bulk Two"})
		vCLI.RunCLI("new", "person", "Add Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Add Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"stdin":      true,
			"object_ids": []interface{}{"people/add-bulk-one", "people/add-bulk-two"},
			"text":       "bulk add preview",
		})
		cliResult := vCLI.RunCLIWithStdin("people/add-bulk-one\npeople/add-bulk-two\n", "add", "--stdin", "bulk add preview")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "content"})
	})

	t.Run("add_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Apply Two"})
		vCLI.RunCLI("new", "person", "Add Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Add Apply Two").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"object_ids": []interface{}{"people/add-apply-one", "people/add-apply-two"},
			"text":       "bulk add apply",
		})
		cliResult := vCLI.RunCLIWithStdin("people/add-apply-one\npeople/add-apply-two\n", "add", "--stdin", "--confirm", "bulk add apply")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "results", "total", "skipped", "errors", "added", "content"})
	})

	t.Run("set", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Set",
		})
		vCLI.RunCLI("new", "person", "Parity Set").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"object_id": "people/parity-set",
			"fields": map[string]interface{}{
				"email": "set@example.com",
			},
		})
		cliResult := vCLI.RunCLI("set", "people/parity-set", "email=set@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "object_id", "type", "updated_fields"})
	})

	t.Run("set_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Bulk Two"})
		vCLI.RunCLI("new", "person", "Set Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Set Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"object_ids": []interface{}{"people/set-bulk-one", "people/set-bulk-two"},
			"fields": map[string]interface{}{
				"email": "bulk@example.com",
			},
		})
		cliResult := vCLI.RunCLIWithStdin("people/set-bulk-one\npeople/set-bulk-two\n", "set", "--stdin", "email=bulk@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "fields"})
	})

	t.Run("set_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Apply Two"})
		vCLI.RunCLI("new", "person", "Set Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Set Apply Two").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"object_ids": []interface{}{"people/set-apply-one", "people/set-apply-two"},
			"fields": map[string]interface{}{
				"email": "apply@example.com",
			},
		})
		cliResult := vCLI.RunCLIWithStdin("people/set-apply-one\npeople/set-apply-two\n", "set", "--stdin", "--confirm", "email=apply@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "results", "total", "skipped", "errors", "modified", "fields"})
	})

	t.Run("delete", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Delete",
		})
		vCLI.RunCLI("new", "person", "Parity Delete").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"object_id": "people/parity-delete",
			"confirm":   true,
		})
		cliResult := vCLI.RunCLI("delete", "people/parity-delete", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"deleted", "behavior", "trash_path"})
	})

	t.Run("delete_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Bulk Two"})
		vCLI.RunCLI("new", "person", "Delete Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Delete Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"stdin":      true,
			"object_ids": []interface{}{"people/delete-bulk-one", "people/delete-bulk-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/delete-bulk-one\npeople/delete-bulk-two\n", "delete", "--stdin")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "behavior"})
	})

	t.Run("delete_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Apply Two"})
		vCLI.RunCLI("new", "person", "Delete Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Delete Apply Two").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"object_ids": []interface{}{"people/delete-apply-one", "people/delete-apply-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/delete-apply-one\npeople/delete-apply-two\n", "delete", "--stdin", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "results", "total", "skipped", "errors", "deleted", "behavior"})
	})

	t.Run("move", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Move Me"})
		server.callTool("new", map[string]interface{}{
			"type":  "project",
			"title": "Move Ref",
			"field": map[string]interface{}{
				"status": "active",
				"owner":  "people/move-me",
			},
		})
		vCLI.RunCLI("new", "person", "Move Me").MustSucceed(t)
		vCLI.RunCLI("new", "project", "Move Ref", "--field", "status=active", "--field", "owner=people/move-me").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"source":      "people/move-me",
			"destination": "archive/move-me-archived",
		})
		cliResult := vCLI.RunCLI("move", "people/move-me", "archive/move-me-archived")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"source", "destination", "updated_refs"})
	})

	t.Run("move_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk Two"})
		vCLI.RunCLI("new", "person", "Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"stdin":       true,
			"destination": "archive/",
			"object_ids":  []interface{}{"people/bulk-one", "people/bulk-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/bulk-one\npeople/bulk-two\n", "move", "--stdin", "archive/")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "destination"})
	})

	t.Run("move_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk Apply Two"})
		vCLI.RunCLI("new", "person", "Bulk Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Bulk Apply Two").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"stdin":       true,
			"confirm":     true,
			"destination": "archive/",
			"object_ids":  []interface{}{"people/bulk-apply-one", "people/bulk-apply-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/bulk-apply-one\npeople/bulk-apply-two\n", "move", "--stdin", "--confirm", "archive/")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "results", "total", "skipped", "errors", "moved", "destination"})
	})

	t.Run("reindex", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/roadmap.md", `---
type: project
status: active
---
# Roadmap
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/roadmap.md", `---
type: project
status: active
---
# Roadmap
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("reindex", map[string]interface{}{
			"full": true,
		})
		cliResult := vCLI.RunCLI("reindex", "--full")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{
			"files_indexed",
			"files_skipped",
			"files_deleted",
			"objects",
			"traits",
			"references",
			"schema_rebuilt",
			"incremental",
			"dry_run",
			"errors",
			"refs_resolved",
			"refs_unresolved",
		})
	})

	t.Run("check", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/security.md", `---
type: project
status: active
owner: people/missing-owner
---
# Security
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/security.md", `---
type: project
status: active
owner: people/missing-owner
---
# Security
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("check", map[string]interface{}{})
		cliResult := vCLI.RunCLI("check")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{
			"file_count",
			"error_count",
			"warning_count",
			"issues",
			"summary",
		})
	})

	t.Run("read_raw", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/read-target.md", `---
type: person
name: Read Target
---
# Read Target

Line one
Line two
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/read-target.md", `---
type: person
name: Read Target
---
# Read Target

Line one
Line two
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("read", map[string]interface{}{
			"path":       "people/read-target",
			"raw":        true,
			"lines":      true,
			"start_line": 1,
			"end_line":   5,
		})
		cliResult := vCLI.RunCLI("read", "people/read-target", "--raw", "--lines", "--start-line", "1", "--end-line", "5")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"path", "content", "line_count", "start_line", "end_line", "lines"})
	})

	t.Run("search", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/roadmap.md", `---
type: project
status: active
---
# Roadmap

Contains roadmap milestones.
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/roadmap.md", `---
type: project
status: active
---
# Roadmap

Contains roadmap milestones.
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("search", map[string]interface{}{
			"query": "roadmap milestones",
			"limit": 5,
		})
		cliResult := vCLI.RunCLI("search", "roadmap milestones", "--limit", "5")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query", "results"})
	})

	t.Run("backlinks", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/eve.md", `---
type: person
name: Eve
---
# Eve
`).
			WithFile("projects/security.md", `---
type: project
status: active
owner: people/eve
---
# Security

Owner [[people/eve]]
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/eve.md", `---
type: person
name: Eve
---
# Eve
`).
			WithFile("projects/security.md", `---
type: project
status: active
owner: people/eve
---
# Security

Owner [[people/eve]]
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("backlinks", map[string]interface{}{
			"target": "people/eve",
		})
		cliResult := vCLI.RunCLI("backlinks", "people/eve")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"target", "items"})
	})

	t.Run("outlinks", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/eve.md", `---
type: person
name: Eve
---
# Eve
`).
			WithFile("projects/security.md", `---
type: project
status: active
owner: people/eve
---
# Security

Owner [[people/eve]]
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/eve.md", `---
type: person
name: Eve
---
# Eve
`).
			WithFile("projects/security.md", `---
type: project
status: active
owner: people/eve
---
# Security

Owner [[people/eve]]
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("outlinks", map[string]interface{}{
			"source": "projects/security",
		})
		cliResult := vCLI.RunCLI("outlinks", "projects/security")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"source", "items"})
	})

	t.Run("resolve", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/alex.md", `---
type: person
name: Alex
---
# Alex
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/alex.md", `---
type: person
name: Alex
---
# Alex
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("resolve", map[string]interface{}{
			"reference": "alex",
		})
		cliResult := vCLI.RunCLI("resolve", "alex")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"resolved", "object_id", "file_path", "is_section", "type", "match_source"})
	})

	t.Run("query_full", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/alpha.md", `---
type: project
status: active
---
# Alpha
`).
			WithFile("projects/beta.md", `---
type: project
status: paused
---
# Beta
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/alpha.md", `---
type: project
status: active
---
# Alpha
`).
			WithFile("projects/beta.md", `---
type: project
status: paused
---
# Beta
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("query", map[string]interface{}{
			"query_string": "object:project .status==active",
			"limit":        10,
			"offset":       0,
		})
		cliResult := vCLI.RunCLI("query", "object:project .status==active", "--limit", "10", "--offset", "0")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query_type", "type", "items", "total", "returned", "offset", "limit"})
	})

	t.Run("query_saved_list", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today", "--description", "Overdue tasks").MustSucceed(t)
		vCLI.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today", "--description", "Overdue tasks").MustSucceed(t)

		mcpResult := server.callTool("query_saved_list", map[string]interface{}{})
		cliResult := vCLI.RunCLI("query", "saved", "list")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"queries"})
	})

	t.Run("query_saved_get", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today", "--description", "Overdue tasks").MustSucceed(t)
		vCLI.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today", "--description", "Overdue tasks").MustSucceed(t)

		mcpResult := server.callTool("query_saved_get", map[string]interface{}{
			"name": "overdue",
		})
		cliResult := vCLI.RunCLI("query", "saved", "get", "overdue")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"name", "query", "args", "description"})
	})

	t.Run("query_saved_set", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("query_saved_set", map[string]interface{}{
			"name":         "overdue",
			"query_string": "trait:due .value<today",
			"description":  "Overdue tasks",
		})
		cliResult := vCLI.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today", "--description", "Overdue tasks")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"name", "query", "args", "description", "status"})
	})

	t.Run("query_saved_remove", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today").MustSucceed(t)
		vCLI.RunCLI("query", "saved", "set", "overdue", "trait:due .value<today").MustSucceed(t)

		mcpResult := server.callTool("query_saved_remove", map[string]interface{}{
			"name": "overdue",
		})
		cliResult := vCLI.RunCLI("query", "saved", "remove", "overdue")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"name", "removed"})
	})

	t.Run("docs", func(t *testing.T) {
		docsIndex := `sections:
  getting-started:
    topics:
      installation:
        path: installation.md
  querying:
    topics:
      query-language:
        path: query-language.md
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile(".raven/docs/index.yaml", docsIndex).
			WithFile(".raven/docs/getting-started/installation.md", "# Installation\n\nWelcome.\n").
			WithFile(".raven/docs/querying/query-language.md", "# Query Language\n\nquery predicate examples.\n").
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile(".raven/docs/index.yaml", docsIndex).
			WithFile(".raven/docs/getting-started/installation.md", "# Installation\n\nWelcome.\n").
			WithFile(".raven/docs/querying/query-language.md", "# Query Language\n\nquery predicate examples.\n").
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("docs", map[string]interface{}{})
		cliResult := vCLI.RunCLI("docs")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"sections", "command_docs", "navigation_tip"})
	})

	t.Run("docs_list", func(t *testing.T) {
		docsIndex := `sections:
  getting-started:
    topics:
      installation:
        path: installation.md
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile(".raven/docs/index.yaml", docsIndex).
			WithFile(".raven/docs/getting-started/installation.md", "# Installation\n\nWelcome.\n").
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile(".raven/docs/index.yaml", docsIndex).
			WithFile(".raven/docs/getting-started/installation.md", "# Installation\n\nWelcome.\n").
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("docs_list", map[string]interface{}{})
		cliResult := vCLI.RunCLI("docs", "list")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"sections", "command_docs", "navigation_tip"})
	})

	t.Run("docs_search", func(t *testing.T) {
		docsIndex := `sections:
  querying:
    topics:
      query-language:
        path: query-language.md
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile(".raven/docs/index.yaml", docsIndex).
			WithFile(".raven/docs/querying/query-language.md", "# Query Language\n\nquery predicate examples.\n").
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile(".raven/docs/index.yaml", docsIndex).
			WithFile(".raven/docs/querying/query-language.md", "# Query Language\n\nquery predicate examples.\n").
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("docs_search", map[string]interface{}{
			"query":   "query",
			"section": "querying",
			"limit":   5,
		})
		cliResult := vCLI.RunCLI("docs", "search", "query", "--section", "querying", "--limit", "5")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query", "count", "matches"})
	})

	t.Run("docs_fetch", func(t *testing.T) {
		archive := buildDocsArchiveBytes(t, map[string]string{
			"raven-main/docs/index.yaml":                 "sections:\n  guide:\n    topics:\n      start:\n        path: start.md\n",
			"raven-main/docs/guide/start.md":             "# Start\n",
			"raven-main/internal/mcp/agent-guide/foo.md": "ignored\n",
		})

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/archive/main" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(archive)
		}))
		defer httpServer.Close()

		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		source := httpServer.URL + "/archive"
		mcpResult := server.callTool("docs_fetch", map[string]interface{}{
			"source": source,
			"ref":    "main",
		})
		cliResult := vCLI.RunCLI("docs", "fetch", "--source", source, "--ref", "main")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"path", "file_count", "byte_count", "source", "ref", "archive_url", "fetched_at", "cli_version", "manifest_ver"})
	})

	t.Run("stats", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("vault_stats", map[string]interface{}{})
		cliResult := vCLI.RunCLI("vault", "stats")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file_count", "object_count", "trait_count", "ref_count"})
	})

	t.Run("version", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("version", map[string]interface{}{})
		cliResult := vCLI.RunCLI("version")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"version", "module_path", "commit", "commit_time", "modified", "go_version", "goos", "goarch"})
	})

	t.Run("init", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpInitPath := filepath.Join(vMCP.Path, "new-vault")
		cliInitPath := filepath.Join(vCLI.Path, "new-vault")

		mcpResult := server.callTool("init", map[string]interface{}{
			"path": mcpInitPath,
		})
		cliResult := vCLI.RunCLI("init", cliInitPath)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"status", "created_config", "created_schema", "gitignore_state", "docs"})
	})

	t.Run("daily", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("daily", map[string]interface{}{
			"date": "2026-02-18",
		})
		cliResult := vCLI.RunCLI("daily", "2026-02-18")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "date", "created", "opened"})
	})

	t.Run("date", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("daily", "2026-02-18").MustSucceed(t)
		vCLI.RunCLI("daily", "2026-02-18").MustSucceed(t)
		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("date", map[string]interface{}{
			"date": "2026-02-18",
		})
		cliResult := vCLI.RunCLI("date", "2026-02-18")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"date", "day_of_week", "daily_note_id", "daily_path", "daily_exists", "items", "backlinks"})
	})

	t.Run("skill_list", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("skill_list", map[string]interface{}{})
		cliResult := vCLI.RunCLI("skill", "list")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"skills"})
	})

	t.Run("skill_install_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_install", map[string]interface{}{
			"name":   "raven-core",
			"target": "codex",
			"scope":  "user",
			"dest":   dest,
		})
		cliResult := vCLI.RunCLI("skill", "install", "raven-core", "--target", "codex", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"mode", "plan"})
	})

	t.Run("skill_remove_not_installed", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_remove", map[string]interface{}{
			"name":   "raven-core",
			"target": "codex",
			"scope":  "user",
			"dest":   dest,
		})
		cliResult := vCLI.RunCLI("skill", "remove", "raven-core", "--target", "codex", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("skill_doctor", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_doctor", map[string]interface{}{
			"target": "codex",
			"scope":  "user",
			"dest":   dest,
		})
		cliResult := vCLI.RunCLI("skill", "doctor", "--target", "codex", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"reports"})
	})

	t.Run("schema", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema", map[string]interface{}{
			"subcommand": "types",
		})
		cliResult := vCLI.RunCLI("schema", "types")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"types", "hint"})
	})

	t.Run("schema_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema", map[string]interface{}{
			"subcommand": "type",
			"name":       "person",
		})
		cliResult := vCLI.RunCLI("schema", "type", "person")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"type"})
	})

	t.Run("schema_add_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_add_type", map[string]interface{}{
			"name": "meeting",
		})
		cliResult := vCLI.RunCLI("schema", "add", "type", "meeting")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"added", "name", "default_path"})
	})

	t.Run("schema_validate", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_validate", map[string]interface{}{})
		cliResult := vCLI.RunCLI("schema", "validate")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"valid", "issues", "types", "traits"})
	})

	t.Run("schema_add_trait", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_add_trait", map[string]interface{}{
			"name":   "priority",
			"type":   "enum",
			"values": "high,medium,low",
		})
		cliResult := vCLI.RunCLI("schema", "add", "trait", "priority", "--type", "enum", "--values", "high,medium,low")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"added", "name", "type", "values"})
	})

	t.Run("schema_add_field", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_add_field", map[string]interface{}{
			"type_name":   "person",
			"field_name":  "website",
			"type":        "string",
			"description": "Primary website URL",
		})
		cliResult := vCLI.RunCLI("schema", "add", "field", "person", "website", "--type", "string", "--description", "Primary website URL")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"added", "type", "field", "field_type", "required", "description"})
	})

	t.Run("schema_update_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_update_type", map[string]interface{}{
			"name":        "project",
			"description": "Tracked work items",
			"add-trait":   "priority",
		})
		cliResult := vCLI.RunCLI("schema", "update", "type", "project", "--description", "Tracked work items", "--add-trait", "priority")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"updated", "name", "changes"})
	})

	t.Run("schema_update_trait", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_update_trait", map[string]interface{}{
			"name":   "priority",
			"values": "low,medium,high,critical",
		})
		cliResult := vCLI.RunCLI("schema", "update", "trait", "priority", "--values", "low,medium,high,critical")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"updated", "name", "changes"})
	})

	t.Run("schema_update_field", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_update_field", map[string]interface{}{
			"type_name":  "project",
			"field_name": "status",
			"values":     "active,paused,done,archived",
		})
		cliResult := vCLI.RunCLI("schema", "update", "field", "project", "status", "--values", "active,paused,done,archived")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"updated", "type", "field", "changes"})
	})

	t.Run("schema_remove_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_remove_type", map[string]interface{}{
			"name": "project",
		})
		cliResult := vCLI.RunCLI("schema", "remove", "type", "project")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"removed", "name"})
	})

	t.Run("schema_remove_trait", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_remove_trait", map[string]interface{}{
			"name": "priority",
		})
		cliResult := vCLI.RunCLI("schema", "remove", "trait", "priority")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"removed", "name"})
	})

	t.Run("schema_remove_field", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_remove_field", map[string]interface{}{
			"type_name":  "project",
			"field_name": "owner",
		})
		cliResult := vCLI.RunCLI("schema", "remove", "field", "project", "owner")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"removed", "type", "field"})
	})

	t.Run("schema_rename_field_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/alex.md", `---
type: person
name: Alex
email: alex@example.com
---
# Alex
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/alex.md", `---
type: person
name: Alex
email: alex@example.com
---
# Alex
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_rename_field", map[string]interface{}{
			"type_name": "person",
			"old_field": "email",
			"new_field": "primary_email",
		})
		cliResult := vCLI.RunCLI("schema", "rename", "field", "person", "email", "primary_email")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "type", "old_field", "new_field", "total_changes", "hint"})
	})

	t.Run("schema_rename_field_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/alex.md", `---
type: person
name: Alex
email: alex@example.com
---
# Alex
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/alex.md", `---
type: person
name: Alex
email: alex@example.com
---
# Alex
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_rename_field", map[string]interface{}{
			"type_name": "person",
			"old_field": "email",
			"new_field": "primary_email",
			"confirm":   true,
		})
		cliResult := vCLI.RunCLI("schema", "rename", "field", "person", "email", "primary_email", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"renamed", "type", "old_field", "new_field", "changes_applied", "hint"})
	})

	t.Run("schema_rename_type_preview", func(t *testing.T) {
		schemaYAML := `version: 2
types:
  event:
    default_path: events/
    fields:
      title: { type: string }
  project:
    default_path: projects/
    fields:
      kickoff:
        type: ref
        target: event
traits: {}
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(schemaYAML).
			WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
			WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(schemaYAML).
			WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
			WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_rename_type", map[string]interface{}{
			"old_name": "event",
			"new_name": "meeting",
		})
		cliResult := vCLI.RunCLI("schema", "rename", "type", "event", "meeting")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{
			"preview", "old_name", "new_name", "total_changes", "hint",
			"default_path_rename_available", "default_path_old", "default_path_new",
			"optional_total_changes", "files_to_move",
		})
	})

	t.Run("schema_rename_type_apply", func(t *testing.T) {
		schemaYAML := `version: 2
types:
  event:
    default_path: events/
    fields:
      title: { type: string }
  project:
    default_path: projects/
    fields:
      kickoff:
        type: ref
        target: event
traits: {}
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(schemaYAML).
			WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
			WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(schemaYAML).
			WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
			WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_rename_type", map[string]interface{}{
			"old_name":            "event",
			"new_name":            "meeting",
			"confirm":             true,
			"rename_default_path": true,
		})
		cliResult := vCLI.RunCLI("schema", "rename", "type", "event", "meeting", "--confirm", "--rename-default-path")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{
			"renamed", "old_name", "new_name", "changes_applied", "hint",
			"default_path_rename_available", "default_path_renamed", "default_path_old", "default_path_new",
			"files_moved", "reference_files_updated",
		})
	})

	t.Run("template_list", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/meeting.md", "# Meeting Template\n")
		vCLI.WriteFile("templates/meeting.md", "# Meeting Template\n")

		mcpResult := server.callTool("template_list", map[string]interface{}{})
		cliResult := vCLI.RunCLI("template", "list")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"template_dir", "templates"})
	})

	t.Run("template_write", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("template_write", map[string]interface{}{
			"path":    "meeting.md",
			"content": "# Meeting Template\n",
		})
		cliResult := vCLI.RunCLI("template", "write", "meeting.md", "--content", "# Meeting Template\n")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"path", "status", "template_dir"})
	})

	t.Run("template_delete", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/meeting.md", "# Meeting Template\n")
		vCLI.WriteFile("templates/meeting.md", "# Meeting Template\n")

		mcpResult := server.callTool("template_delete", map[string]interface{}{
			"path": "meeting.md",
		})
		cliResult := vCLI.RunCLI("template", "delete", "meeting.md")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"deleted", "trash_path", "forced", "template_ids"})
	})

	t.Run("schema_template_set", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/person.md", "# Person Template\n")
		vCLI.WriteFile("templates/person.md", "# Person Template\n")

		mcpResult := server.callTool("schema_template_set", map[string]interface{}{
			"template_id": "person_profile",
			"file":        "templates/person.md",
			"description": "Person profile template",
		})
		cliResult := vCLI.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md", "--description", "Person profile template")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"id", "file", "description"})
	})

	t.Run("schema_template_get", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/person.md", "# Person Template\n")
		vCLI.WriteFile("templates/person.md", "# Person Template\n")
		vMCP.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md", "--description", "Person profile template").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md", "--description", "Person profile template").MustSucceed(t)

		mcpResult := server.callTool("schema_template_get", map[string]interface{}{
			"template_id": "person_profile",
		})
		cliResult := vCLI.RunCLI("schema", "template", "get", "person_profile")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"id", "file", "description"})
	})

	t.Run("schema_template_remove", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/person.md", "# Person Template\n")
		vCLI.WriteFile("templates/person.md", "# Person Template\n")
		vMCP.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)

		mcpResult := server.callTool("schema_template_remove", map[string]interface{}{
			"template_id": "person_profile",
		})
		cliResult := vCLI.RunCLI("schema", "template", "remove", "person_profile")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"removed", "id"})
	})

	t.Run("schema_template_bind_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/person.md", "# Person Template\n")
		vCLI.WriteFile("templates/person.md", "# Person Template\n")
		vMCP.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)

		mcpResult := server.callTool("schema_template_bind", map[string]interface{}{
			"type":        "person",
			"template_id": "person_profile",
		})
		cliResult := vCLI.RunCLI("schema", "template", "bind", "person_profile", "--type", "person")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"type", "template_id"})
	})

	t.Run("schema_template_default_clear_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/person.md", "# Person Template\n")
		vCLI.WriteFile("templates/person.md", "# Person Template\n")
		vMCP.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)
		vMCP.RunCLI("schema", "template", "bind", "person_profile", "--type", "person").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "bind", "person_profile", "--type", "person").MustSucceed(t)
		vMCP.RunCLI("schema", "template", "default", "person_profile", "--type", "person").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "default", "person_profile", "--type", "person").MustSucceed(t)

		mcpResult := server.callTool("schema_template_default", map[string]interface{}{
			"type":  "person",
			"clear": true,
		})
		cliResult := vCLI.RunCLI("schema", "template", "default", "--type", "person", "--clear")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"type", "default_template"})
	})

	t.Run("schema_template_bind_core", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/daily.md", "# Daily Template\n")
		vCLI.WriteFile("templates/daily.md", "# Daily Template\n")
		vMCP.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md").MustSucceed(t)

		mcpResult := server.callTool("schema_template_bind", map[string]interface{}{
			"core":        "date",
			"template_id": "daily_default",
		})
		cliResult := vCLI.RunCLI("schema", "template", "bind", "daily_default", "--core", "date")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"core_type", "template_id"})
	})

	t.Run("schema_template_default_clear_core", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/daily.md", "# Daily Template\n")
		vCLI.WriteFile("templates/daily.md", "# Daily Template\n")
		vMCP.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md").MustSucceed(t)
		vMCP.RunCLI("schema", "template", "bind", "daily_default", "--core", "date").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "bind", "daily_default", "--core", "date").MustSucceed(t)
		vMCP.RunCLI("schema", "template", "default", "daily_default", "--core", "date").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "default", "daily_default", "--core", "date").MustSucceed(t)

		mcpResult := server.callTool("schema_template_default", map[string]interface{}{
			"core":  "date",
			"clear": true,
		})
		cliResult := vCLI.RunCLI("schema", "template", "default", "--core", "date", "--clear")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"core_type", "default_template"})
	})
}

func TestMCPIntegration_DirectDispatchReferenceErrorsParity(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)

	t.Run("read_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("read", map[string]interface{}{
			"path": "people/missing",
		})
		cliResult := vCLI.RunCLI("read", "people/missing")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("set_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("set", map[string]interface{}{
			"object_id": "people/missing",
			"fields": map[string]interface{}{
				"alias": "ghost",
			},
		})
		cliResult := vCLI.RunCLI("set", "people/missing", "alias=ghost")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("set_ambiguous_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Alice"})
		server.callTool("new", map[string]interface{}{"type": "project", "title": "Alice", "field": map[string]interface{}{"status": "active"}})
		vCLI.RunCLI("new", "person", "Alice").MustSucceed(t)
		vCLI.RunCLI("new", "project", "Alice", "--field", "status=active").MustSucceed(t)

		// Ensure resolver-backed reference lookup sees both objects.
		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"object_id": "alice",
			"fields": map[string]interface{}{
				"alias": "ambiguous",
			},
		})
		cliResult := vCLI.RunCLI("set", "alice", "alias=ambiguous")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("set_bulk_missing_ids", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin": true,
			"fields": map[string]interface{}{
				"email": "bulk@example.com",
			},
		})

		env := parseMCPEnvelope(t, mcpResult.Text)
		if !mcpResult.IsError || env.OK {
			t.Fatalf("expected set bulk missing ids to fail: %s", mcpResult.Text)
		}
		if env.Error == nil || env.Error.Code != "MISSING_ARGUMENT" {
			t.Fatalf("expected MISSING_ARGUMENT, got: %s", mcpResult.Text)
		}
		if strings.Contains(env.Error.Message, "stdin") {
			t.Fatalf("expected MCP error message to avoid stdin wording, got: %q", env.Error.Message)
		}
		if env.Error.Message != "no object_ids provided for bulk set" {
			t.Fatalf("unexpected MCP error message: %q", env.Error.Message)
		}
		if env.Error.Suggestion != "Provide object_ids for the bulk update and retry" {
			t.Fatalf("unexpected MCP suggestion: %q", env.Error.Suggestion)
		}
	})

	t.Run("set_bulk_missing_fields_mcp_uses_arg_language", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Bulk Missing Fields"})

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"object_ids": []interface{}{"people/set-bulk-missing-fields"},
		})

		env := parseMCPEnvelope(t, mcpResult.Text)
		if !mcpResult.IsError || env.OK {
			t.Fatalf("expected set bulk missing fields to fail: %s", mcpResult.Text)
		}
		if env.Error == nil || env.Error.Code != "MISSING_ARGUMENT" {
			t.Fatalf("expected MISSING_ARGUMENT, got: %s", mcpResult.Text)
		}
		if strings.Contains(env.Error.Suggestion, "--stdin") || strings.Contains(env.Error.Suggestion, "--fields-json") {
			t.Fatalf("expected MCP suggestion to avoid CLI flags, got: %q", env.Error.Suggestion)
		}
		if env.Error.Suggestion != "Provide fields or fields-json in args" {
			t.Fatalf("unexpected MCP suggestion: %q", env.Error.Suggestion)
		}
	})

	t.Run("set_bulk_fields_json_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Json One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Json Two"})
		vCLI.RunCLI("new", "person", "Set Json One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Set Json Two").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"object_ids": []interface{}{"people/set-json-one", "people/set-json-two"},
			"fields_json": map[string]interface{}{
				"email": "true",
			},
		})
		cliResult := vCLI.RunCLIWithStdin(
			"people/set-json-one\npeople/set-json-two\n",
			"set",
			"--stdin",
			"--confirm",
			"--fields-json",
			"{\"email\":\"true\"}",
		)

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
		vMCP.AssertFileContains("people/set-json-one.md", `email: "true"`)
		vMCP.AssertFileContains("people/set-json-two.md", `email: "true"`)
		vCLI.AssertFileContains("people/set-json-one.md", `email: "true"`)
		vCLI.AssertFileContains("people/set-json-two.md", `email: "true"`)
	})

	t.Run("add_missing_text", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("add", map[string]interface{}{
			"to": "people/missing",
		})
		env := parseMCPEnvelope(t, mcpResult.Text)
		if !mcpResult.IsError || env.OK {
			t.Fatalf("expected invoke validation error, got: %s", mcpResult.Text)
		}
		if env.Error == nil || env.Error.Code != "INVALID_ARGS" {
			t.Fatalf("expected INVALID_ARGS, got: %s", mcpResult.Text)
		}
	})

	t.Run("add_bulk_missing_ids", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("add", map[string]interface{}{
			"stdin": true,
			"text":  "bulk add",
		})
		cliResult := vCLI.RunCLIWithStdin("", "add", "--stdin", "bulk add")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("add_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("add", map[string]interface{}{
			"text": "missing ref add",
			"to":   "people/missing",
		})
		cliResult := vCLI.RunCLI("add", "missing ref add", "--to", "people/missing")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("add_ambiguous_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Jordan"})
		server.callTool("new", map[string]interface{}{"type": "project", "title": "Jordan", "field": map[string]interface{}{"status": "active"}})
		vCLI.RunCLI("new", "person", "Jordan").MustSucceed(t)
		vCLI.RunCLI("new", "project", "Jordan", "--field", "status=active").MustSucceed(t)

		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"text": "ambiguous ref add",
			"to":   "jordan",
		})
		cliResult := vCLI.RunCLI("add", "ambiguous ref add", "--to", "jordan")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("delete_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"object_id": "people/missing",
			"force":     true,
		})
		cliResult := vCLI.RunCLI("delete", "people/missing", "--force")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("delete_ambiguous_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Robin"})
		server.callTool("new", map[string]interface{}{"type": "project", "title": "Robin", "field": map[string]interface{}{"status": "active"}})
		vCLI.RunCLI("new", "person", "Robin").MustSucceed(t)
		vCLI.RunCLI("new", "project", "Robin", "--field", "status=active").MustSucceed(t)

		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"object_id": "robin",
			"force":     true,
		})
		cliResult := vCLI.RunCLI("delete", "robin", "--force")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("delete_bulk_missing_ids", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"stdin": true,
		})
		cliResult := vCLI.RunCLIWithStdin("", "delete", "--stdin")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("move_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("move", map[string]interface{}{
			"source":      "people/missing",
			"destination": "archive/missing",
		})
		cliResult := vCLI.RunCLI("move", "people/missing", "archive/missing")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("move_ambiguous_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Sam"})
		server.callTool("new", map[string]interface{}{"type": "project", "title": "Sam", "field": map[string]interface{}{"status": "active"}})
		vCLI.RunCLI("new", "person", "Sam").MustSucceed(t)
		vCLI.RunCLI("new", "project", "Sam", "--field", "status=active").MustSucceed(t)

		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"source":      "sam",
			"destination": "archive/sam",
		})
		cliResult := vCLI.RunCLI("move", "sam", "archive/sam")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("move_bulk_missing_ids", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("move", map[string]interface{}{
			"stdin":       true,
			"destination": "archive/",
		})
		cliResult := vCLI.RunCLIWithStdin("", "move", "--stdin", "archive/")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("schema_add_field_invalid_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_add_field", map[string]interface{}{
			"type_name":  "person",
			"field_name": "manager",
			"type":       "person",
		})
		cliResult := vCLI.RunCLI("schema", "add", "field", "person", "manager", "--type", "person")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("schema_update_field_missing_field", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_update_field", map[string]interface{}{
			"type_name":  "person",
			"field_name": "missing",
			"type":       "string",
		})
		cliResult := vCLI.RunCLI("schema", "update", "field", "person", "missing", "--type", "string")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("schema_remove_field_missing_field", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_remove_field", map[string]interface{}{
			"type_name":  "person",
			"field_name": "missing",
		})
		cliResult := vCLI.RunCLI("schema", "remove", "field", "person", "missing")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("schema_remove_type_missing_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_remove_type", map[string]interface{}{
			"name": "missing_type",
		})
		cliResult := vCLI.RunCLI("schema", "remove", "type", "missing_type")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("schema_validate_invalid_schema", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("schema.yaml", "version: [")
		vCLI.WriteFile("schema.yaml", "version: [")

		mcpResult := server.callTool("schema_validate", map[string]interface{}{})
		cliResult := vCLI.RunCLI("schema", "validate")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("query_parse_error", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("query", map[string]interface{}{
			"query_string": "object:project .status===",
		})
		cliResult := vCLI.RunCLI("query", "object:project .status===")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("backlinks_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("backlinks", map[string]interface{}{
			"target": "people/missing",
		})
		cliResult := vCLI.RunCLI("backlinks", "people/missing")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("outlinks_ambiguous_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/sam.md", `---
type: person
name: Sam
---
# Sam
`).
			WithFile("projects/sam.md", `---
type: project
status: active
---
# Sam
`).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/sam.md", `---
type: person
name: Sam
---
# Sam
`).
			WithFile("projects/sam.md", `---
type: project
status: active
---
# Sam
`).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("outlinks", map[string]interface{}{
			"source": "sam",
		})
		cliResult := vCLI.RunCLI("outlinks", "sam")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})
}

func TestMCPIntegration_DirectDispatchSchemaRenameErrorsParity(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)

	t.Run("schema_rename_field_missing_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_rename_field", map[string]interface{}{
			"type_name": "ghost",
			"old_field": "email",
			"new_field": "primary_email",
		})
		cliResult := vCLI.RunCLI("schema", "rename", "field", "ghost", "email", "primary_email")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("schema_rename_type_target_exists", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("schema_rename_type", map[string]interface{}{
			"old_name": "person",
			"new_name": "project",
		})
		cliResult := vCLI.RunCLI("schema", "rename", "type", "person", "project")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})
}

type mcpEnvelope struct {
	OK       bool                   `json:"ok"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    *mcpErrorEnvelope      `json:"error,omitempty"`
	Warnings []mcpWarningEnvelope   `json:"warnings,omitempty"`
}

type mcpErrorEnvelope struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Suggestion string                 `json:"suggestion,omitempty"`
}

type mcpWarningEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

func parseMCPEnvelope(t *testing.T, raw string) *mcpEnvelope {
	t.Helper()
	var env mcpEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("failed to parse MCP envelope: %v\nraw: %s", err, raw)
	}
	return &env
}

func assertEnvelopeParity(t *testing.T, mcpResult toolResult, cliResult *testutil.CLIResult, dataKeys []string) {
	t.Helper()

	env := parseMCPEnvelope(t, mcpResult.Text)

	if env.OK != cliResult.OK {
		t.Fatalf("ok mismatch: mcp=%v cli=%v\nmcp: %s\ncli: %s", env.OK, cliResult.OK, mcpResult.Text, cliResult.RawJSON)
	}
	if mcpResult.IsError != !env.OK {
		t.Fatalf("isError mismatch: isError=%v ok=%v\nmcp: %s", mcpResult.IsError, env.OK, mcpResult.Text)
	}

	if cliResult.Error == nil {
		if env.Error != nil {
			t.Fatalf("expected no error, got mcp error %+v", env.Error)
		}
	} else {
		if env.Error == nil {
			t.Fatalf("expected mcp error code %q, got nil\nmcp: %s\ncli: %s", cliResult.Error.Code, mcpResult.Text, cliResult.RawJSON)
		}
		if env.Error.Code != cliResult.Error.Code {
			t.Fatalf("error code mismatch: mcp=%q cli=%q\nmcp: %s\ncli: %s", env.Error.Code, cliResult.Error.Code, mcpResult.Text, cliResult.RawJSON)
		}
	}

	for _, key := range dataKeys {
		var mcpVal interface{}
		if env.Data != nil {
			mcpVal = env.Data[key]
		}
		var cliVal interface{}
		if cliResult.Data != nil {
			cliVal = cliResult.Data[key]
		}
		if key == "fetched_at" {
			mcpTS, mcpOK := mcpVal.(string)
			cliTS, cliOK := cliVal.(string)
			if mcpOK && cliOK && mcpTS != "" && cliTS != "" {
				if mcpParsed, err := time.Parse(time.RFC3339, mcpTS); err == nil {
					if cliParsed, err := time.Parse(time.RFC3339, cliTS); err == nil {
						diff := mcpParsed.Sub(cliParsed)
						if diff < 0 {
							diff = -diff
						}
						if diff <= 2*time.Second {
							continue
						}
					}
				}
			}
		}
		mcpVal = normalizeParityValue(mcpVal)
		cliVal = normalizeParityValue(cliVal)
		if !reflect.DeepEqual(mcpVal, cliVal) {
			t.Fatalf("data mismatch for key %q: mcp=%#v cli=%#v\nmcp: %s\ncli: %s", key, mcpVal, cliVal, mcpResult.Text, cliResult.RawJSON)
		}
	}

	mcpWarningCodes := make([]string, 0, len(env.Warnings))
	for _, warning := range env.Warnings {
		mcpWarningCodes = append(mcpWarningCodes, warning.Code)
	}
	cliWarningCodes := make([]string, 0, len(cliResult.Warnings))
	for _, warning := range cliResult.Warnings {
		cliWarningCodes = append(cliWarningCodes, warning.Code)
	}
	sort.Strings(mcpWarningCodes)
	sort.Strings(cliWarningCodes)
	if !reflect.DeepEqual(mcpWarningCodes, cliWarningCodes) {
		t.Fatalf("warning code mismatch: mcp=%v cli=%v\nmcp: %s\ncli: %s", mcpWarningCodes, cliWarningCodes, mcpResult.Text, cliResult.RawJSON)
	}
}

func normalizeParityValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			if key == "query_time_ms" {
				continue
			}
			out[key] = normalizeParityValue(value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, value := range typed {
			out[i] = normalizeParityValue(value)
		}
		return out
	default:
		return v
	}
}

func buildDocsArchiveBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q): %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// testServer wraps the MCP Server for testing purposes.
type testServer struct {
	t          *testing.T
	vaultPath  string
	executable string
}

// toolResult represents the result of a tool call.
type toolResult struct {
	Text    string
	IsError bool
}

// newTestServer creates a test server with a custom executable path.
func newTestServer(t *testing.T, vaultPath, executable string) *testServer {
	return &testServer{
		t:          t,
		vaultPath:  vaultPath,
		executable: executable,
	}
}

// callTool invokes a tool by simulating the MCP JSON-RPC protocol.
func (s *testServer) callTool(name string, args map[string]interface{}) toolResult {
	s.t.Helper()

	requestName := name
	requestArgs := args
	if name != "raven_discover" && name != "raven_describe" && name != "raven_invoke" {
		requestName = "raven_invoke"
		requestArgs = map[string]interface{}{"command": name}
		if args != nil {
			requestArgs["args"] = args
		}
	}

	// Create a real MCP server but with custom executable
	server := mcp.NewServerWithExecutable(s.vaultPath, s.executable)

	// Create a simulated JSON-RPC request
	paramsBytes, _ := json.Marshal(map[string]interface{}{
		"name":      requestName,
		"arguments": requestArgs,
	})
	params := json.RawMessage(paramsBytes)

	request := mcp.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  &params,
	}

	// Capture output
	var output bytes.Buffer
	server.SetIO(strings.NewReader(""), &output)

	// Handle the request directly
	server.HandleRequest(&request)

	// Parse the response
	var response struct {
		Result mcp.ToolResult `json:"result"`
	}
	if err := json.NewDecoder(&output).Decode(&response); err != nil {
		return toolResult{Text: "Failed to parse MCP response: " + err.Error(), IsError: true}
	}

	text := ""
	if len(response.Result.Content) > 0 {
		text = response.Result.Content[0].Text
	}

	return toolResult{
		Text:    text,
		IsError: response.Result.IsError,
	}
}

// Verify the integration test helpers compile correctly by importing from mcp package
var _ = mcp.GenerateToolSchemas

// testServerInterface is used to verify we're implementing the expected pattern.
type testServerInterface interface {
	callTool(name string, args map[string]interface{}) toolResult
}

var _ testServerInterface = (*testServer)(nil)

// Create a stub io.Writer for testing
type discardWriter struct{}

func (discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

var _ io.Writer = discardWriter{}
