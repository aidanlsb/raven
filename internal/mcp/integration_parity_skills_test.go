//go:build integration

package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParitySkillTests(t *testing.T, binary string) {
	t.Run("skill_list", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("skill_list", map[string]interface{}{})
		cliResult := vCLI.RunCLI("skill", "list")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"scope", "root", "skills"})
	})

	t.Run("skill_sync_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_sync", map[string]interface{}{
			"name":  "raven-core",
			"scope": "user",
			"dest":  dest,
		})
		cliResult := vCLI.RunCLI("skill", "sync", "raven-core", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"mode", "plan"})
	})

	t.Run("skill_install_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_install", map[string]interface{}{
			"scope": "user",
			"dest":  dest,
		})
		cliResult := vCLI.RunCLI("skill", "install", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"mode", "needs_confirm", "requested", "skills", "installed"})
	})

	t.Run("skill_install_json_without_yes_previews_only", func(t *testing.T) {
		v := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		dest := filepath.Join(t.TempDir(), "skills")

		result := v.RunCLI("skill", "install", "--scope", "user", "--dest", dest).MustSucceed(t)

		if mode, _ := result.Data["mode"].(string); mode != "preview" {
			t.Fatalf("skill install without --yes mode = %v, want preview", result.Data["mode"])
		}
		if needsConfirm, _ := result.Data["needs_confirm"].(bool); !needsConfirm {
			t.Fatalf("skill install without --yes needs_confirm = %v, want true", result.Data["needs_confirm"])
		}
		if entries, err := os.ReadDir(dest); err == nil && len(entries) != 0 {
			t.Fatalf("skill install without --yes wrote %d entries, want 0", len(entries))
		}
	})

	t.Run("skill_install_yes_installs_all_shipped_skills", func(t *testing.T) {
		v := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		dest := filepath.Join(t.TempDir(), "skills")

		v.RunCLI("skill", "install", "--scope", "user", "--dest", dest, "--yes").MustSucceed(t)

		for _, name := range []string{"raven-onboarding", "raven-core", "raven-query", "raven-schema", "raven-maintenance", "raven-templates", "raven-vault-admin"} {
			if _, err := os.Stat(filepath.Join(dest, name, "SKILL.md")); err != nil {
				t.Fatalf("expected %s installed after skill install --yes: %v", name, err)
			}
		}
	})

	t.Run("skill_install_mcp_yes_applies", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_install", map[string]interface{}{
			"names": []interface{}{"raven-core"},
			"scope": "user",
			"dest":  dest,
			"yes":   true,
		})
		env := parseMCPEnvelope(t, mcpResult.Text)
		if !env.OK {
			t.Fatalf("skill_install via MCP failed: %s", mcpResult.Text)
		}
		if mode, _ := env.Data["mode"].(string); mode != "applied" {
			t.Fatalf("MCP skill_install with yes mode = %v, want applied", env.Data["mode"])
		}
		if _, err := os.Stat(filepath.Join(dest, "raven-core", "SKILL.md")); err != nil {
			t.Fatalf("expected raven-core installed after MCP skill_install yes: %v", err)
		}
	})

	t.Run("skill_remove_not_installed", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_remove", map[string]interface{}{
			"name":  "raven-core",
			"scope": "user",
			"dest":  dest,
		})
		cliResult := vCLI.RunCLI("skill", "remove", "raven-core", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, nil)
	})

	t.Run("skill_doctor", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_doctor", map[string]interface{}{
			"scope": "user",
			"dest":  dest,
		})
		cliResult := vCLI.RunCLI("skill", "doctor", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"reports"})
	})
}
