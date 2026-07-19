//go:build integration

package mcp_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParityQueryTests(t *testing.T, binary string) {
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
			"query_string": "type:project .status==active",
			"limit":        10,
			"offset":       0,
		})
		cliResult := vCLI.RunCLI("query", "type:project .status==active", "--limit", "10", "--offset", "0")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query_kind", "type", "items", "total", "returned", "offset", "limit", "has_more"})
	})

	t.Run("query_asset", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/raven.md", `---
type: project
status: active
---
# Raven

See [[assets/pdfs/paper.pdf]].
`).
			WithFile("assets/pdfs/paper.pdf", "%PDF-1.7\nhello").
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/raven.md", `---
type: project
status: active
---
# Raven

See [[assets/pdfs/paper.pdf]].
`).
			WithFile("assets/pdfs/paper.pdf", "%PDF-1.7\nhello").
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("query", map[string]interface{}{
			"query_string": "asset .extension==pdf",
			"limit":        10,
			"offset":       0,
		})
		cliResult := vCLI.RunCLI("query", "asset .extension==pdf", "--limit", "10", "--offset", "0")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query_kind", "items", "total", "returned", "offset", "limit", "has_more"})
	})

	t.Run("query_apply_object_preview", func(t *testing.T) {
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
			"query_string": "type:project .status==active",
			"apply":        []interface{}{"set status=done"},
		})
		cliResult := vCLI.RunCLI("query", "type:project .status==active", "--apply", "set status=done")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "fields"})
		vMCP.AssertFileContains("projects/alpha.md", "status: active")
		vMCP.AssertFileNotContains("projects/alpha.md", "status: done")
		vCLI.AssertFileContains("projects/alpha.md", "status: active")
		vCLI.AssertFileNotContains("projects/alpha.md", "status: done")
	})

	t.Run("query_apply_trait_confirm", func(t *testing.T) {
		taskFile := `---
type: page
---
# Task 1

- @priority(low) First task
- @priority(low) Second task
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		vMCP.RunCLI("reindex").MustSucceed(t)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("query", map[string]interface{}{
			"query_string": "trait:priority .value==low",
			"apply":        []interface{}{"update high"},
			"confirm":      true,
		})
		cliResult := vCLI.RunCLI("query", "trait:priority .value==low", "--apply", "update high", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"action", "items", "total", "modified", "skipped", "errors"})
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
}
