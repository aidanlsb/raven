//go:build integration

package mcp_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParityRetrievalTests(t *testing.T, binary string) {
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
			"reference":  "people/read-target",
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

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query", "items"})
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
			"reference": "people/eve",
		})
		cliResult := vCLI.RunCLI("backlinks", "people/eve")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"target", "items"})

		grouped := server.callTool("backlinks", map[string]interface{}{
			"references": []interface{}{"people/eve"},
		})
		groupedEnv := parseMCPEnvelope(t, grouped.Text)
		if !groupedEnv.OK {
			t.Fatalf("grouped backlinks failed: %s", grouped.Text)
		}
		if groups, ok := groupedEnv.Data["items_by_target"].([]interface{}); !ok || len(groups) != 1 {
			t.Fatalf("grouped backlinks items_by_target = %#v, want 1 group", groupedEnv.Data["items_by_target"])
		}
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
			"reference": "projects/security",
		})
		cliResult := vCLI.RunCLI("outlinks", "projects/security")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"source", "items"})

		grouped := server.callTool("outlinks", map[string]interface{}{
			"references": []interface{}{"projects/security"},
		})
		groupedEnv := parseMCPEnvelope(t, grouped.Text)
		if !groupedEnv.OK {
			t.Fatalf("grouped outlinks failed: %s", grouped.Text)
		}
		if groups, ok := groupedEnv.Data["items_by_source"].([]interface{}); !ok || len(groups) != 1 {
			t.Fatalf("grouped outlinks items_by_source = %#v, want 1 group", groupedEnv.Data["items_by_source"])
		}
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
}
