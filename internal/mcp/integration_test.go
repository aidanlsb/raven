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
	if len(tools) == 0 {
		t.Fatal("expected at least one tool, got none")
	}

	// Verify some expected tools exist
	expectedTools := []string{"raven_new", "raven_query", "raven_search", "raven_read", "raven_set", "raven_delete"}
	foundTools := make(map[string]bool)
	toolByName := make(map[string]mcp.Tool)
	for _, tool := range tools {
		foundTools[tool.Name] = true
		toolByName[tool.Name] = tool
	}

	for _, expected := range expectedTools {
		if !foundTools[expected] {
			t.Errorf("expected tool %s not found in tool list", expected)
		}
	}

	// Verify schema field tools expose description in JSON-RPC tools/list output.
	for _, toolName := range []string{"raven_schema_add_field", "raven_schema_update_field"} {
		tool, ok := toolByName[toolName]
		if !ok {
			t.Fatalf("expected tool %s in tools/list response", toolName)
		}
		prop, ok := tool.InputSchema.Properties["description"]
		if !ok {
			t.Fatalf("expected %s to include description property", toolName)
		}
		propMap, ok := prop.(map[string]interface{})
		if !ok {
			t.Fatalf("expected %s description property to be an object, got %T", toolName, prop)
		}
		if got := propMap["type"]; got != "string" {
			t.Fatalf("expected %s description property type=string, got %#v", toolName, got)
		}
	}
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

// TestMCPIntegration_CreatePageWithObjectRootFallback verifies that when
// directories.page is omitted, it defaults to directories.object for creation.
func TestMCPIntegration_CreatePageWithObjectRootFallback(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`directories:
  object: objects/
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("raven_new", map[string]interface{}{
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

// TestMCPIntegration_QuerySavedQueryInlineArgs tests MCP query_string containing
// "<saved-query-name> <inputs...>" in a single string argument.
func TestMCPIntegration_QuerySavedQueryInlineArgs(t *testing.T) {
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

	// MCP passes query_string as one arg; ensure saved query + inline input works.
	result := server.callTool("raven_query", map[string]interface{}{
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

// TestMCPIntegration_StringEncodedStructuredInputs verifies that MCP tool calls
// succeed when structured inputs are provided as strings (strict-client compatibility).
func TestMCPIntegration_StringEncodedStructuredInputs(t *testing.T) {
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
		WithRavenYAML(`workflows:
  string-compat:
    file: workflows/string-compat.yaml
`).
		WithFile("workflows/string-compat.yaml", `description: String payload compatibility workflow
inputs:
  date:
    type: string
    required: true
steps:
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Compose a short brief for {{inputs.date}}.
  - id: save
    type: tool
    tool: raven_upsert
    arguments:
      type: page
      title: "Brief {{inputs.date}}"
      content: "{{steps.compose.outputs.markdown}}"
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// JSON-typed flag provided as JSON string (not object).
	upsertResult := server.callTool("raven_upsert", map[string]interface{}{
		"type":       "project",
		"title":      "MCP Compat Project",
		"field-json": `{"status":"active"}`,
	})
	if upsertResult.IsError {
		t.Fatalf("upsert with field-json string failed: %s", upsertResult.Text)
	}
	v.AssertFileContains("projects/mcp-compat-project.md", "status: active")

	// Key-value structured input provided as a single "k=v" string.
	setResult := server.callTool("raven_set", map[string]interface{}{
		"object_id": "projects/mcp-compat-project",
		"fields":    "status=done",
	})
	if setResult.IsError {
		t.Fatalf("set with fields string failed: %s", setResult.Text)
	}
	v.AssertFileContains("projects/mcp-compat-project.md", "status: done")

	// Update trait value uses a plain string positional value.
	updateResult := server.callTool("raven_update", map[string]interface{}{
		"trait_id": "tasks/missing.md:trait:0",
		"value":    "done",
	})
	if updateResult.IsError {
		t.Fatalf("update with plain string value failed: %s", updateResult.Text)
	}

	// JSON-typed workflow input provided as string (underscore key variant).
	runResult := server.callTool("raven_workflow_run", map[string]interface{}{
		"name":       "string-compat",
		"input_json": `{"date":"2026-02-16"}`,
	})
	if runResult.IsError {
		t.Fatalf("workflow run with input_json string failed: %s", runResult.Text)
	}

	var runResp struct {
		OK   bool `json:"ok"`
		Data struct {
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(runResult.Text), &runResp); err != nil {
		t.Fatalf("failed to parse workflow run response: %v\nraw: %s", err, runResult.Text)
	}
	if runResp.Data.RunID == "" {
		t.Fatalf("expected workflow run_id, got empty response: %s", runResult.Text)
	}
	if runResp.Data.Status != "awaiting_agent" {
		t.Fatalf("expected awaiting_agent status, got %q", runResp.Data.Status)
	}

	// JSON-typed workflow continue payload provided as string (underscore keys).
	continueResult := server.callTool("raven_workflow_continue", map[string]interface{}{
		"run_id":            runResp.Data.RunID,
		"agent_output_json": `{"outputs":{"markdown":"# Brief\nGenerated from string payloads."}}`,
	})
	if continueResult.IsError {
		t.Fatalf("workflow continue with agent_output_json string failed: %s", continueResult.Text)
	}

	var continueResp struct {
		OK   bool `json:"ok"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(continueResult.Text), &continueResp); err != nil {
		t.Fatalf("failed to parse workflow continue response: %v\nraw: %s", err, continueResult.Text)
	}
	if continueResp.Data.Status != "completed" {
		t.Fatalf("expected completed status after continue, got %q", continueResp.Data.Status)
	}

	searchResult := server.callTool("raven_search", map[string]interface{}{
		"query": "Generated from string payloads",
		"limit": float64(5),
	})
	if searchResult.IsError {
		t.Fatalf("search for persisted workflow output failed: %s", searchResult.Text)
	}
	var searchResp struct {
		OK   bool `json:"ok"`
		Data struct {
			Results []interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(searchResult.Text), &searchResp); err != nil {
		t.Fatalf("failed to parse search response: %v\nraw: %s", err, searchResult.Text)
	}
	if len(searchResp.Data.Results) == 0 {
		t.Fatalf("expected persisted output to be searchable, got no results: %s", searchResult.Text)
	}
}

// TestMCPIntegration_EditDeleteWithEmptyString tests deleting text via raven_edit with empty new_str.
func TestMCPIntegration_EditDeleteWithEmptyString(t *testing.T) {
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

	result := server.callTool("raven_edit", map[string]interface{}{
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

	// Get details for one type using explicit positional args.
	typeResult := server.callTool("raven_schema", map[string]interface{}{
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
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Add a new field with a description.
	addFieldResult := server.callTool("raven_schema_add_field", map[string]interface{}{
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
	updateFieldResult := server.callTool("raven_schema_update_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "email",
		"description": "Primary contact email",
	})
	if updateFieldResult.IsError {
		t.Fatalf("schema update field failed: %s", updateFieldResult.Text)
	}
	v.AssertFileContains("schema.yaml", "description: Primary contact email")

	// Remove the description with "-" sentinel.
	removeDescriptionResult := server.callTool("raven_schema_update_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "email",
		"description": "-",
	})
	if removeDescriptionResult.IsError {
		t.Fatalf("schema update field remove description failed: %s", removeDescriptionResult.Text)
	}
	v.AssertFileNotContains("schema.yaml", "description: Primary contact email")
}

// TestMCPIntegration_SchemaRenameTypeWithDefaultPathRename verifies MCP JSON
// preview/apply behavior for type rename with optional default_path directory
// migration.
func TestMCPIntegration_SchemaRenameTypeWithDefaultPathRename(t *testing.T) {
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

	preview := server.callTool("raven_schema_rename_type", map[string]interface{}{
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

	apply := server.callTool("raven_schema_rename_type", map[string]interface{}{
		"old_name":            "event",
		"new_name":            "meeting",
		"confirm":             true,
		"rename-default-path": true,
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

// TestMCPIntegration_CheckCreateMissingWithConfirm verifies non-interactive
// create-missing behavior via MCP (JSON mode + confirm=true).
func TestMCPIntegration_CheckCreateMissingWithConfirm(t *testing.T) {
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

	_ = server.callTool("raven_check", map[string]interface{}{
		"create-missing": true,
		"confirm":        true,
	})

	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
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
