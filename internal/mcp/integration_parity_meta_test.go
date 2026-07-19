//go:build integration

package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParityMetaTests(t *testing.T, binary string) {
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
		configPath := seedGlobalDocsConfig(t, map[string]string{
			"index.yaml":                      docsIndex,
			"getting-started/installation.md": "# Installation\n\nWelcome.\n",
			"querying/query-language.md":      "# Query Language\n\nquery predicate examples.\n",
		})
		server := newTestServerWithBaseArgs(t, baseArgsForConfig(configPath), binary)

		mcpResult := server.callTool("docs", map[string]interface{}{})
		cliResult := runCLIWithConfig(t, binary, configPath, "docs")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"sections", "command_docs", "navigation_tip"})
	})

	t.Run("docs_list", func(t *testing.T) {
		docsIndex := `sections:
  getting-started:
    topics:
      installation:
        path: installation.md
`
		configPath := seedGlobalDocsConfig(t, map[string]string{
			"index.yaml":                      docsIndex,
			"getting-started/installation.md": "# Installation\n\nWelcome.\n",
		})
		server := newTestServerWithBaseArgs(t, baseArgsForConfig(configPath), binary)

		mcpResult := server.callTool("docs_list", map[string]interface{}{})
		cliResult := runCLIWithConfig(t, binary, configPath, "docs", "list")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"sections", "command_docs", "navigation_tip"})
	})

	t.Run("docs_search", func(t *testing.T) {
		docsIndex := `sections:
  querying:
    topics:
      query-language:
        path: query-language.md
`
		configPath := seedGlobalDocsConfig(t, map[string]string{
			"index.yaml":                 docsIndex,
			"querying/query-language.md": "# Query Language\n\nquery predicate examples.\nquery trait examples.\nquery refs examples.\n",
		})
		server := newTestServerWithBaseArgs(t, baseArgsForConfig(configPath), binary)

		mcpResult := server.callTool("docs_search", map[string]interface{}{
			"query":   "query",
			"section": "querying",
		})
		cliResult := runCLIWithConfig(t, binary, configPath, "docs", "search", "query", "--section", "querying")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"query", "count", "returned", "limit", "offset", "has_more", "items"})

		mcpPage := server.callTool("docs_search", map[string]interface{}{
			"query":   "query",
			"section": "querying",
			"limit":   2,
			"offset":  2,
		})
		cliPage := runCLIWithConfig(t, binary, configPath, "docs", "search", "query", "--section", "querying", "--limit", "2", "--offset", "2")

		assertEnvelopeParity(t, mcpPage, cliPage, []string{"query", "count", "returned", "limit", "offset", "has_more", "items"})
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

		configPath := seedGlobalDocsConfig(t, nil)
		server := newTestServerWithBaseArgs(t, baseArgsForConfig(configPath), binary)

		source := httpServer.URL + "/archive"
		mcpResult := server.callTool("docs_fetch", map[string]interface{}{
			"source": source,
			"ref":    "main",
		})
		cliResult := runCLIWithConfig(t, binary, configPath, "docs", "fetch", "--source", source, "--ref", "main")

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

		// Init now auto-registers the new vault, so both sides must write to a
		// throwaway config instead of the host's. Share one config dir so the
		// derived docs path (and thus the docs envelope) stays identical.
		sharedConfig := filepath.Join(t.TempDir(), "config.toml")
		server := newTestServerWithBaseArgs(t, baseArgsForConfig(sharedConfig), binary)

		mcpInitPath := filepath.Join(vMCP.Path, "new-vault")
		cliInitPath := filepath.Join(vCLI.Path, "new-vault")

		mcpResult := server.callTool("init", map[string]interface{}{
			"path": mcpInitPath,
		})
		cliResult := runCLIWithConfig(t, binary, sharedConfig, "init", cliInitPath)

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
}
