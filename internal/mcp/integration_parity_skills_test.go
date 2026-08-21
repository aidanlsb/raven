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

	t.Run("skill_install_named_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_install", map[string]interface{}{
			"names": []interface{}{"raven-core"},
			"scope": "user",
			"dest":  dest,
		})
		cliResult := vCLI.RunCLI("skill", "install", "raven-core", "--scope", "user", "--dest", dest)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"mode", "needs_confirm", "requested", "skills", "installed"})
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

	t.Run("skill_install_json_without_confirm_previews_only", func(t *testing.T) {
		v := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		dest := filepath.Join(t.TempDir(), "skills")

		result := v.RunCLI("skill", "install", "--scope", "user", "--dest", dest).MustSucceed(t)

		if mode, _ := result.Data["mode"].(string); mode != "preview" {
			t.Fatalf("skill install without --confirm mode = %v, want preview", result.Data["mode"])
		}
		if needsConfirm, _ := result.Data["needs_confirm"].(bool); !needsConfirm {
			t.Fatalf("skill install without --confirm needs_confirm = %v, want true", result.Data["needs_confirm"])
		}
		if entries, err := os.ReadDir(dest); err == nil && len(entries) != 0 {
			t.Fatalf("skill install without --confirm wrote %d entries, want 0", len(entries))
		}
	})

	t.Run("skill_install_confirm_reconciles_full_catalog", func(t *testing.T) {
		v := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		dest := filepath.Join(t.TempDir(), "skills")

		v.RunCLI("skill", "install", "raven-core", "--scope", "user", "--dest", dest, "--confirm").MustSucceed(t)
		corePath := filepath.Join(dest, "raven-core", "SKILL.md")
		if err := os.WriteFile(corePath, []byte("outdated managed content"), 0o644); err != nil {
			t.Fatalf("write outdated raven-core: %v", err)
		}
		retiredPath := filepath.Join(dest, "raven-retired")
		if err := os.MkdirAll(retiredPath, 0o755); err != nil {
			t.Fatalf("create retired skill path: %v", err)
		}
		if err := os.WriteFile(filepath.Join(retiredPath, "SKILL.md"), []byte("retired managed content"), 0o644); err != nil {
			t.Fatalf("write retired managed file: %v", err)
		}
		userFile := filepath.Join(retiredPath, "notes.md")
		if err := os.WriteFile(userFile, []byte("keep me"), 0o644); err != nil {
			t.Fatalf("write retired user file: %v", err)
		}
		receipt := `{"skill":"raven-retired","version":1,"scope":"user","checksum":"","files":["SKILL.md"],"installed_at":""}`
		if err := os.WriteFile(filepath.Join(retiredPath, ".rvn-skill-receipt.json"), []byte(receipt), 0o644); err != nil {
			t.Fatalf("write retired receipt: %v", err)
		}

		v.RunCLI("skill", "install", "--scope", "user", "--dest", dest, "--confirm").MustSucceed(t)
		for _, name := range []string{"raven-onboarding", "raven-core", "raven-query", "raven-schema", "raven-maintenance", "raven-templates", "raven-vault-admin"} {
			if _, err := os.Stat(filepath.Join(dest, name, "SKILL.md")); err != nil {
				t.Fatalf("expected %s installed after confirmed full skill install: %v", name, err)
			}
		}
		gotCore, err := os.ReadFile(corePath)
		if err != nil {
			t.Fatalf("read reconciled raven-core: %v", err)
		}
		if string(gotCore) == "outdated managed content" {
			t.Fatal("confirmed full skill install did not replace existing managed skill")
		}
		if _, err := os.Stat(filepath.Join(retiredPath, "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("retired managed skill file still exists, err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(retiredPath, ".rvn-skill-receipt.json")); !os.IsNotExist(err) {
			t.Fatalf("retired skill receipt still exists, err = %v", err)
		}
		if _, err := os.Stat(userFile); err != nil {
			t.Fatalf("non-Raven file at retired skill path was removed: %v", err)
		}
	})

	t.Run("skill_install_mcp_confirm_applies", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)
		dest := filepath.Join(t.TempDir(), "skills")

		mcpResult := server.callTool("skill_install", map[string]interface{}{
			"names":   []interface{}{"raven-core"},
			"scope":   "user",
			"dest":    dest,
			"confirm": true,
		})
		env := parseMCPEnvelope(t, mcpResult.Text)
		if !env.OK {
			t.Fatalf("skill_install via MCP failed: %s", mcpResult.Text)
		}
		if mode, _ := env.Data["mode"].(string); mode != "applied" {
			t.Fatalf("MCP skill_install with confirm mode = %v, want applied", env.Data["mode"])
		}
		if _, err := os.Stat(filepath.Join(dest, "raven-core", "SKILL.md")); err != nil {
			t.Fatalf("expected raven-core installed after MCP skill_install confirm: %v", err)
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
