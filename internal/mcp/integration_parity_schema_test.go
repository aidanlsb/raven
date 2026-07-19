//go:build integration

package mcp_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParitySchemaTests(t *testing.T, binary string) {
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
}
