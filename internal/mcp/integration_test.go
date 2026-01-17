//go:build integration

package mcp_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/mcp"
	"github.com/aidanlsb/raven/internal/testutil"
)

// TestMCPIntegration_ToolsList tests that the MCP server returns tool schemas.
func TestMCPIntegration_ToolsList(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	// Build CLI to ensure we have a binary
	_ = testutil.BuildCLI(t)

	// Create a server and get its tools
	tools := mcp.GenerateToolSchemas()

	if len(tools) == 0 {
		t.Fatal("expected at least one tool, got none")
	}

	// Verify some expected tools exist
	expectedTools := []string{"raven_new", "raven_query", "raven_search", "raven_read", "raven_set", "raven_delete"}
	foundTools := make(map[string]bool)
	for _, tool := range tools {
		foundTools[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !foundTools[expected] {
			t.Errorf("expected tool %s not found in tool list", expected)
		}
	}

	// Reference vault path to avoid unused variable warning
	_ = v.Path
}

// TestMCPIntegration_CreateObject tests creating an object via MCP tool call.
func TestMCPIntegration_CreateObject(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)

	// Create a test server that uses our built binary
	server := newTestServer(t, v.Path, binary)

	// Call the raven_new tool
	result := server.callTool("raven_new", map[string]interface{}{
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

// TestMCPIntegration_QueryObjects tests querying objects via MCP tool call.
func TestMCPIntegration_QueryObjects(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create some objects first
	server.callTool("raven_new", map[string]interface{}{
		"type":  "project",
		"title": "Project A",
		"field": map[string]interface{}{"status": "active"},
	})
	server.callTool("raven_new", map[string]interface{}{
		"type":  "project",
		"title": "Project B",
		"field": map[string]interface{}{"status": "done"},
	})

	// Query for active projects - uses == for equality
	result := server.callTool("raven_query", map[string]interface{}{
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

// TestMCPIntegration_ReadObject tests reading an object via MCP tool call.
func TestMCPIntegration_ReadObject(t *testing.T) {
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
	server.callTool("raven_reindex", nil)

	// Read the object
	result := server.callTool("raven_read", map[string]interface{}{
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
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create a person
	server.callTool("raven_new", map[string]interface{}{
		"type":  "person",
		"title": "Carol",
	})

	// Update the email field
	result := server.callTool("raven_set", map[string]interface{}{
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

// TestMCPIntegration_DeleteObject tests deleting an object via MCP tool call.
func TestMCPIntegration_DeleteObject(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create an object
	server.callTool("raven_new", map[string]interface{}{
		"type":  "person",
		"title": "Dave",
	})
	v.AssertFileExists("people/dave.md")

	// Delete it
	result := server.callTool("raven_delete", map[string]interface{}{
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
	server.callTool("raven_reindex", nil)

	// Search for roadmap
	result := server.callTool("raven_search", map[string]interface{}{
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
	server.callTool("raven_reindex", nil)

	// Get backlinks for Eve
	result := server.callTool("raven_backlinks", map[string]interface{}{
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
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Get schema types
	result := server.callTool("raven_schema", map[string]interface{}{
		"subcommand": "types",
	})

	if result.IsError {
		t.Fatalf("schema introspection failed: %s", result.Text)
	}

	// Verify person and project types are in the output
	if !strings.Contains(result.Text, "person") || !strings.Contains(result.Text, "project") {
		t.Errorf("expected schema to include person and project types, got: %s", result.Text)
	}
}

// TestMCPIntegration_Check tests vault check via MCP tool call.
func TestMCPIntegration_Check(t *testing.T) {
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
	server.callTool("raven_reindex", nil)

	// Run check - note: check command may return IsError=true when issues are found
	result := server.callTool("raven_check", nil)

	// Check returns a different format (not the standard ok/data envelope)
	// The response should contain "issues" and "missing_reference"
	if !strings.Contains(result.Text, "issues") {
		t.Errorf("expected check output to contain 'issues'\nText: %s", result.Text)
	}

	if !strings.Contains(result.Text, "missing_reference") {
		t.Errorf("expected check output to include 'missing_reference' issue\nText: %s", result.Text)
	}
}

// TestMCPIntegration_ErrorHandling tests that MCP errors are properly surfaced.
func TestMCPIntegration_ErrorHandling(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create a file first
	server.callTool("raven_new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
	})

	// Try to create a duplicate (should fail)
	result := server.callTool("raven_new", map[string]interface{}{
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

	// Build the CLI args using the public function
	cmdArgs := mcp.BuildCLIArgs(name, args)
	if len(cmdArgs) == 0 {
		return toolResult{Text: `{"ok":false,"error":{"code":"UNKNOWN_TOOL","message":"Unknown tool"}}`, IsError: true}
	}

	// Create a real MCP server but with custom executable
	server := mcp.NewServerWithExecutable(s.vaultPath, s.executable)

	// Create a simulated JSON-RPC request
	paramsBytes, _ := json.Marshal(map[string]interface{}{
		"name":      name,
		"arguments": args,
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
var _ = mcp.BuildCLIArgs

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
