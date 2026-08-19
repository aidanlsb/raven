//go:build integration

package mcp_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParityTemplateTests(t *testing.T, binary string) {
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
			"default":     true,
		})
		cliResult := vCLI.RunCLI("schema", "template", "bind", "person_profile", "--type", "person", "--default")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"type", "template_id", "default_template"})
	})

	t.Run("schema_template_unbind_clear_default_type", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/person.md", "# Person Template\n")
		vCLI.WriteFile("templates/person.md", "# Person Template\n")
		vMCP.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md").MustSucceed(t)
		vMCP.RunCLI("schema", "template", "bind", "person_profile", "--type", "person", "--default").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "bind", "person_profile", "--type", "person", "--default").MustSucceed(t)

		mcpResult := server.callTool("schema_template_unbind", map[string]interface{}{
			"type":          "person",
			"template_id":   "person_profile",
			"clear-default": true,
		})
		cliResult := vCLI.RunCLI("schema", "template", "unbind", "person_profile", "--type", "person", "--clear-default")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"type", "template_id", "removed", "default_cleared"})
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
			"default":     true,
		})
		cliResult := vCLI.RunCLI("schema", "template", "bind", "daily_default", "--core", "date", "--default")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"core", "template_id", "default_template"})
	})

	t.Run("schema_template_unbind_clear_default_core", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.WriteFile("templates/daily.md", "# Daily Template\n")
		vCLI.WriteFile("templates/daily.md", "# Daily Template\n")
		vMCP.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md").MustSucceed(t)
		vMCP.RunCLI("schema", "template", "bind", "daily_default", "--core", "date", "--default").MustSucceed(t)
		vCLI.RunCLI("schema", "template", "bind", "daily_default", "--core", "date", "--default").MustSucceed(t)

		mcpResult := server.callTool("schema_template_unbind", map[string]interface{}{
			"core":          "date",
			"template_id":   "daily_default",
			"clear-default": true,
		})
		cliResult := vCLI.RunCLI("schema", "template", "unbind", "daily_default", "--core", "date", "--clear-default")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"core", "template_id", "removed", "default_cleared"})
	})
}
