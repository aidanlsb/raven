//go:build integration

package mcp_test

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_DirectDispatchReferenceErrorsParity(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)

	t.Run("read_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("read", map[string]interface{}{
			"reference": "people/missing",
		})
		cliResult := vCLI.RunCLI("read", "people/missing")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("set_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("set", map[string]interface{}{
			"reference": "people/missing",
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
			"reference": "alice",
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
		if env.Error.Message != "no references provided for bulk set" {
			t.Fatalf("unexpected MCP error message: %q", env.Error.Message)
		}
		if env.Error.Suggestion != "Provide references for the bulk update and retry" {
			t.Fatalf("unexpected MCP suggestion: %q", env.Error.Suggestion)
		}
	})

	t.Run("update_bulk_missing_ids", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("update", map[string]interface{}{
			"stdin": true,
			"value": "done",
		})

		env := parseMCPEnvelope(t, mcpResult.Text)
		if !mcpResult.IsError || env.OK {
			t.Fatalf("expected update bulk missing ids to fail: %s", mcpResult.Text)
		}
		if env.Error == nil || env.Error.Code != "MISSING_ARGUMENT" {
			t.Fatalf("expected MISSING_ARGUMENT, got: %s", mcpResult.Text)
		}
		if strings.Contains(env.Error.Message, "stdin") {
			t.Fatalf("expected MCP error message to avoid stdin wording, got: %q", env.Error.Message)
		}
		if env.Error.Message != "no trait_ids provided for bulk update" {
			t.Fatalf("unexpected MCP error message: %q", env.Error.Message)
		}
		if env.Error.Suggestion != "Provide trait_ids for the bulk update and retry" {
			t.Fatalf("unexpected MCP suggestion: %q", env.Error.Suggestion)
		}
	})

	t.Run("set_bulk_missing_fields_mcp_uses_arg_language", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Bulk Missing Fields"})

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"references": []interface{}{"people/set-bulk-missing-fields"},
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
			"references": []interface{}{"people/set-json-one", "people/set-json-two"},
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
			"reference": "people/missing",
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
			"reference": "robin",
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
			"query_string": "type:project .status===",
		})
		cliResult := vCLI.RunCLI("query", "type:project .status===")

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("backlinks_missing_reference", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("backlinks", map[string]interface{}{
			"reference": "people/missing",
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
			"reference": "sam",
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
