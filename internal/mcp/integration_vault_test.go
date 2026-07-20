//go:build integration

package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_InvokeVaultPathOverride(t *testing.T) {
	t.Parallel()
	primary := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	override := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, primary.Path, binary)

	result := server.callTool("raven_invoke", map[string]interface{}{
		"command":    "new",
		"vault_path": override.Path,
		"args": map[string]interface{}{
			"type":  "person",
			"title": "Vault Override",
		},
	})
	if result.IsError {
		t.Fatalf("expected invoke success, got error: %s", result.Text)
	}
	if primary.FileExists("people/vault-override.md") {
		t.Fatal("expected pinned vault to remain unchanged")
	}
	if !override.FileExists("people/vault-override.md") {
		t.Fatal("expected object to be created in override vault")
	}
}

func TestMCPIntegration_StrictVaultRejectsAmbientFallback(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	globalDir := t.TempDir()
	configPath := filepath.Join(globalDir, "config.toml")
	cfg := "default_vault = \"primary\"\n[vaults]\nprimary = \"" + v.Path + "\"\n"
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	binary := testutil.BuildCLI(t)
	server := newTestServerWithBaseArgs(t, baseArgsForConfig(configPath), binary)
	server.strictVault = true

	// Without an explicit vault, strict mode must refuse to fall back.
	result := server.callTool("raven_invoke", map[string]interface{}{
		"command": "new",
		"args": map[string]interface{}{
			"type":  "person",
			"title": "Strict No Vault",
		},
	})
	if !result.IsError {
		t.Fatalf("expected VAULT_AMBIGUOUS error, got success: %s", result.Text)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.Text), &envelope); err != nil {
		t.Fatalf("parse envelope: %v; text=%s", err, result.Text)
	}
	if envelope.Error.Code != "VAULT_AMBIGUOUS" {
		t.Fatalf("error code = %q, want VAULT_AMBIGUOUS; text=%s", envelope.Error.Code, result.Text)
	}
	if v.FileExists("people/strict-no-vault.md") {
		t.Fatal("expected no object to be created when strict resolution fails")
	}

	// With an explicit vault_path, strict mode allows the write.
	ok := server.callTool("raven_invoke", map[string]interface{}{
		"command":    "new",
		"vault_path": v.Path,
		"args": map[string]interface{}{
			"type":  "person",
			"title": "Strict With Vault",
		},
	})
	if ok.IsError {
		t.Fatalf("expected strict invoke with explicit vault to succeed, got error: %s", ok.Text)
	}
	if !v.FileExists("people/strict-with-vault.md") {
		t.Fatal("expected object to be created with explicit vault_path")
	}
}

func TestMCPIntegration_InitFirstRunVaultPolicy(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)

	configPath := filepath.Join(t.TempDir(), "config.toml")
	server := newTestServerWithBaseArgs(t, baseArgsForConfig(configPath), binary)

	vaultRoot := t.TempDir()
	firstPath := filepath.Join(vaultRoot, "first")
	secondPath := filepath.Join(vaultRoot, "second")

	// First vault on the machine: auto-registered, default, and active.
	first := mcpInitPostInit(t, server, firstPath)
	mustToolBool(t, first, "already_registered", true)
	mustToolBool(t, first, "is_first_vault", true)
	mustToolBool(t, first, "is_default", true)
	mustToolBool(t, first, "is_active", true)
	mustToolBool(t, first, "activated", true)
	mustToolBool(t, first, "needs_user_choice_for_activate", false)
	mustToolBool(t, first, "needs_user_choice_for_default", false)
	// Fully configured: no pending actions, so the agent can proceed immediately.
	if actions, ok := first["actions"].(map[string]interface{}); !ok {
		t.Fatalf("actions = %#v, want map", first["actions"])
	} else if len(actions) != 0 {
		t.Fatalf("actions = %#v, want empty for first vault", actions)
	}

	// Second vault: registered and made active, with restore guidance.
	second := mcpInitPostInit(t, server, secondPath)
	if got := second["registered_name"]; got != "second" {
		t.Fatalf("registered_name = %#v, want %q", got, "second")
	}
	mustToolBool(t, second, "already_registered", true)
	mustToolBool(t, second, "registered", true)
	mustToolBool(t, second, "is_first_vault", false)
	mustToolBool(t, second, "has_existing_default", true)
	mustToolBool(t, second, "is_default", false)
	mustToolBool(t, second, "is_active", true)
	mustToolBool(t, second, "activated", true)
	mustToolBool(t, second, "needs_user_choice_for_activate", false)
	mustToolBool(t, second, "needs_user_choice_for_default", true)
	active, ok := second["active_vault"].(map[string]interface{})
	if !ok || active["name"] != "second" || active["path"] != secondPath {
		t.Fatalf("active_vault = %#v, want second at %s", second["active_vault"], secondPath)
	}
	previous, ok := second["previous_active_vault"].(map[string]interface{})
	if !ok || previous["name"] != "first" || previous["path"] != firstPath {
		t.Fatalf("previous_active_vault = %#v, want first at %s", second["previous_active_vault"], firstPath)
	}
	if got := second["switch_back"]; got != `rvn --json vault use -- 'first'` {
		t.Fatalf("switch_back = %#v, want exact restore command", got)
	}

	// An unqualified call now targets the newly initialized active vault.
	ambient := server.callTool("raven_invoke", map[string]interface{}{
		"command": "schema_add_type",
		"args": map[string]interface{}{
			"name": "ambient-second",
		},
	})
	if ambient.IsError {
		t.Fatalf("ambient call to auto-activated vault failed: %s", ambient.Text)
	}
	firstSchema, err := os.ReadFile(filepath.Join(firstPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read first schema: %v", err)
	}
	if strings.Contains(string(firstSchema), "ambient-second") {
		t.Fatalf("ambient call incorrectly mutated previous vault:\n%s", firstSchema)
	}
	secondSchema, err := os.ReadFile(filepath.Join(secondPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read second schema: %v", err)
	}
	if !strings.Contains(string(secondSchema), "ambient-second") {
		t.Fatalf("ambient call did not mutate new active vault:\n%s", secondSchema)
	}

	// Explicit per-call targets remain available.
	explicit := server.callTool("raven_invoke", map[string]interface{}{
		"command": "schema_add_type",
		"vault":   "second",
		"args": map[string]interface{}{
			"name": "explicit-second",
		},
	})
	if explicit.IsError {
		t.Fatalf("explicit second-vault call failed: %s", explicit.Text)
	}
	secondSchema, err = os.ReadFile(filepath.Join(secondPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read second schema: %v", err)
	}
	if !strings.Contains(string(secondSchema), "explicit-second") {
		t.Fatalf("explicit call did not write to second vault:\n%s", secondSchema)
	}
}

func mcpInitPostInit(t *testing.T, server *testServer, path string) map[string]interface{} {
	t.Helper()
	res := server.callTool("init", map[string]interface{}{"path": path})
	if res.IsError {
		t.Fatalf("init returned error: %s", res.Text)
	}
	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Text), &resp); err != nil {
		t.Fatalf("unmarshal init response: %v\n%s", err, res.Text)
	}
	if !resp.OK {
		t.Fatalf("expected init success, got: %s", res.Text)
	}
	postInit, ok := resp.Data["post_init"].(map[string]interface{})
	if !ok {
		t.Fatalf("post_init = %#v, want map", resp.Data["post_init"])
	}
	return postInit
}

func mustToolBool(t *testing.T, data map[string]interface{}, key string, want bool) {
	t.Helper()
	got, ok := data[key].(bool)
	if !ok {
		t.Fatalf("%s = %#v, want bool", key, data[key])
	}
	if got != want {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
}
