//go:build integration

package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_InitFirstVaultAutoRegisters(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "New Notes")

	postInit := runInitPostInit(t, binary, configFile, stateFile, vaultPath)

	if got := postInit["suggested_name"]; got != "new-notes" {
		t.Fatalf("suggested_name = %#v, want %q", got, "new-notes")
	}
	if got := postInit["registered_name"]; got != "new-notes" {
		t.Fatalf("registered_name = %#v, want %q", got, "new-notes")
	}
	mustPostInitBool(t, postInit, "already_registered", true)
	mustPostInitBool(t, postInit, "registered", true)
	mustPostInitBool(t, postInit, "is_first_vault", true)
	mustPostInitBool(t, postInit, "has_existing_default", false)
	mustPostInitBool(t, postInit, "is_default", true)
	mustPostInitBool(t, postInit, "is_active", true)
	mustPostInitBool(t, postInit, "needs_user_choice_for_activate", false)
	mustPostInitBool(t, postInit, "needs_user_choice_for_default", false)

	// A first vault is fully configured: no pending actions and no next steps,
	// so an agent can proceed without guessing.
	if actions, ok := postInit["actions"].(map[string]interface{}); !ok {
		t.Fatalf("actions = %#v, want map", postInit["actions"])
	} else if len(actions) != 0 {
		t.Fatalf("actions = %#v, want empty for first vault", actions)
	}
	if steps, ok := postInit["next_steps"].([]interface{}); ok && len(steps) != 0 {
		t.Fatalf("next_steps = %#v, want empty for first vault", steps)
	}

	// The new vault now resolves via active_vault without any explicit flag.
	current := runVaultCurrent(t, binary, configFile, stateFile)
	if got := current["name"]; got != "new-notes" {
		t.Fatalf("vault current name = %#v, want %q", got, "new-notes")
	}
	if got := current["source"]; got != "active_vault" {
		t.Fatalf("vault current source = %#v, want %q", got, "active_vault")
	}
}

func TestIntegration_InitSecondVaultRegistersWithoutActivating(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")

	firstPath := filepath.Join(root, "first")
	secondPath := filepath.Join(root, "second")

	// The first vault becomes the machine default + active vault.
	first := runInitPostInit(t, binary, configFile, stateFile, firstPath)
	mustPostInitBool(t, first, "is_first_vault", true)

	// The second vault must be registered but must not change routing.
	second := runInitPostInit(t, binary, configFile, stateFile, secondPath)
	if got := second["registered_name"]; got != "second" {
		t.Fatalf("registered_name = %#v, want %q", got, "second")
	}
	mustPostInitBool(t, second, "already_registered", true)
	mustPostInitBool(t, second, "registered", true)
	mustPostInitBool(t, second, "is_first_vault", false)
	mustPostInitBool(t, second, "has_existing_default", true)
	mustPostInitBool(t, second, "is_default", false)
	mustPostInitBool(t, second, "is_active", false)
	mustPostInitBool(t, second, "needs_user_choice_for_activate", true)
	mustPostInitBool(t, second, "needs_user_choice_for_default", true)

	// Invocable actions to activate/pin the new vault must be present.
	actions, ok := second["actions"].(map[string]interface{})
	if !ok {
		t.Fatalf("actions = %#v, want map", second["actions"])
	}
	activate, ok := actions["activate"].(map[string]interface{})
	if !ok || activate["command"] != "vault use" {
		t.Fatalf("actions.activate = %#v, want command=vault use", actions["activate"])
	}

	// Routing must still point at the first vault.
	current := runVaultCurrent(t, binary, configFile, stateFile)
	if got := current["name"]; got != "first" {
		t.Fatalf("vault current name = %#v, want %q (routing must be unchanged)", got, "first")
	}
}

func runInitPostInit(t *testing.T, binary, configFile, stateFile, vaultPath string) map[string]interface{} {
	t.Helper()
	cmd := exec.Command(binary, "--config", configFile, "--state", stateFile, "--json", "init", vaultPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, output)
	}

	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal init response: %v\n%s", err, output)
	}
	if !resp.OK {
		t.Fatalf("expected init success, got: %s", output)
	}
	postInit, ok := resp.Data["post_init"].(map[string]interface{})
	if !ok {
		t.Fatalf("post_init = %#v, want map", resp.Data["post_init"])
	}
	return postInit
}

func runVaultCurrent(t *testing.T, binary, configFile, stateFile string) map[string]interface{} {
	t.Helper()
	cmd := exec.Command(binary, "--config", configFile, "--state", stateFile, "--json", "vault", "current")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vault current failed: %v\n%s", err, output)
	}
	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal vault current response: %v\n%s", err, output)
	}
	if !resp.OK {
		t.Fatalf("expected vault current success, got: %s", output)
	}
	return resp.Data
}

func mustPostInitBool(t *testing.T, data map[string]interface{}, key string, want bool) {
	t.Helper()
	got, ok := data[key].(bool)
	if !ok {
		t.Fatalf("%s = %#v, want bool", key, data[key])
	}
	if got != want {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
}

func TestIntegration_JSONPreRunMissingVaultReturnsEnvelope(t *testing.T) {
	t.Parallel()

	binary := testutil.BuildCLI(t)
	missingVault := filepath.Join(t.TempDir(), "missing-vault")

	cmd := exec.Command(binary, "--vault-path", missingVault, "--json", "query", "type:project")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected JSON envelope without process failure: %v\n%s", err, output)
	}

	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal prerun response: %v\n%s", err, output)
	}
	if resp.OK {
		t.Fatalf("expected failure envelope, got success: %s", output)
	}
	if resp.Error.Code != "VAULT_NOT_FOUND" {
		t.Fatalf("error.code=%q, want VAULT_NOT_FOUND\n%s", resp.Error.Code, output)
	}
	if !strings.Contains(resp.Error.Message, missingVault) {
		t.Fatalf("message=%q, want vault path", resp.Error.Message)
	}
	if !strings.Contains(resp.Error.Suggestion, "rvn init") {
		t.Fatalf("suggestion=%q, want init hint", resp.Error.Suggestion)
	}
}

func TestIntegration_JSONPreRunConfigFailureReturnsEnvelope(t *testing.T) {
	t.Parallel()

	binary := testutil.BuildCLI(t)
	dir := t.TempDir()
	configFile := filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(configFile, []byte("not = [valid"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(binary, "--config", configFile, "--json", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected JSON envelope without process failure: %v\n%s", err, output)
	}

	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal config response: %v\n%s", err, output)
	}
	if resp.OK {
		t.Fatalf("expected failure envelope, got success: %s", output)
	}
	if resp.Error.Code != "CONFIG_INVALID" {
		t.Fatalf("error.code=%q, want CONFIG_INVALID\n%s", resp.Error.Code, output)
	}
	if !strings.Contains(resp.Error.Message, "failed to load config") {
		t.Fatalf("message=%q, want config failure", resp.Error.Message)
	}
}
