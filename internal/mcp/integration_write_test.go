//go:build integration

package mcp_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

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

// TestMCPIntegration_CreatePageWithObjectRootFallback verifies that when
// directories.page is omitted, it defaults to directories.type for creation.
func TestMCPIntegration_CreatePageWithObjectRootFallback(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`directories:
  type: objects/
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
		"type":        "project",
		"title":       "MCP Compat Project",
		"fields-json": `{"status":"active"}`,
	})
	upsertInvalidEnv := parseMCPEnvelope(t, upsertInvalid.Text)
	if !upsertInvalid.IsError || upsertInvalidEnv.OK {
		t.Fatalf("expected upsert fields-json string to fail, got: %s", upsertInvalid.Text)
	}
	if upsertInvalidEnv.Error == nil || upsertInvalidEnv.Error.Code != "INVALID_ARGS" {
		t.Fatalf("expected INVALID_ARGS for upsert fields-json string, got: %s", upsertInvalid.Text)
	}

	// The removed singular flag name is not accepted as an MCP argument alias.
	upsertRemoved := server.callTool("upsert", map[string]interface{}{
		"type":       "project",
		"title":      "MCP Compat Project",
		"field-json": map[string]interface{}{"status": "active"},
	})
	upsertRemovedEnv := parseMCPEnvelope(t, upsertRemoved.Text)
	if !upsertRemoved.IsError || upsertRemovedEnv.OK || upsertRemovedEnv.Error == nil || upsertRemovedEnv.Error.Code != "INVALID_ARGS" {
		t.Fatalf("expected removed upsert field-json argument to fail with INVALID_ARGS, got: %s", upsertRemoved.Text)
	}

	// JSON-typed flag provided as an object succeeds.
	upsertValid := server.callTool("upsert", map[string]interface{}{
		"type":        "project",
		"title":       "MCP Compat Project",
		"fields-json": map[string]interface{}{"status": "active"},
	})
	if upsertValid.IsError {
		t.Fatalf("upsert with fields-json object failed: %s", upsertValid.Text)
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

	// Single-object MCP delete applies immediately when invoked.
	result := server.callTool("delete", map[string]interface{}{
		"object_id": "people/dave",
	})
	if result.IsError {
		t.Fatalf("delete failed: %s", result.Text)
	}

	// Verify it was deleted (moved to trash)
	v.AssertFileNotExists("people/dave.md")
}

// TestMCPIntegration_Search tests full-text search via MCP tool call.
