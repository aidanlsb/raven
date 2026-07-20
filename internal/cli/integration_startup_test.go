//go:build integration

package cli_test

import (
	"encoding/json"
	"errors"
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
	mustPostInitBool(t, postInit, "activated", true)
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

func TestIntegration_InitSecondVaultRegistersAndActivates(t *testing.T) {
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

	// The second vault is registered and becomes active; the default stays first.
	second := runInitPostInit(t, binary, configFile, stateFile, secondPath)
	if got := second["registered_name"]; got != "second" {
		t.Fatalf("registered_name = %#v, want %q", got, "second")
	}
	mustPostInitBool(t, second, "already_registered", true)
	mustPostInitBool(t, second, "registered", true)
	mustPostInitBool(t, second, "is_first_vault", false)
	mustPostInitBool(t, second, "has_existing_default", true)
	mustPostInitBool(t, second, "is_default", false)
	mustPostInitBool(t, second, "is_active", true)
	mustPostInitBool(t, second, "activated", true)
	mustPostInitBool(t, second, "needs_user_choice_for_activate", false)
	mustPostInitBool(t, second, "needs_user_choice_for_default", true)

	// Default changes remain explicit; activation is already complete.
	actions, ok := second["actions"].(map[string]interface{})
	if !ok {
		t.Fatalf("actions = %#v, want map", second["actions"])
	}
	if _, ok := actions["activate"]; ok {
		t.Fatalf("actions.activate = %#v, want automatic activation", actions["activate"])
	}

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

	// Ambient routing now points at the newly initialized vault.
	current := runVaultCurrent(t, binary, configFile, stateFile)
	if got := current["name"]; got != "second" {
		t.Fatalf("vault current name = %#v, want %q", got, "second")
	}
}

func TestIntegration_InitSecondVaultHumanOutputDisclosesSwitch(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	firstPath := filepath.Join(root, "first")
	secondPath := filepath.Join(root, "second")

	runInitPostInit(t, binary, configFile, stateFile, firstPath)

	cmd := exec.Command(binary, "--config", configFile, "--state", stateFile, "init", secondPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("human init failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Active vault switched to 'second'",
		secondPath,
		"Previously active: 'first'",
		firstPath,
		`Switch back: rvn --json vault use -- 'first'`,
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("human output missing %q:\n%s", want, output)
		}
	}
}

func TestIntegration_InitSecondVaultRoutesAmbientWritesToNewActiveVault(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	firstPath := filepath.Join(root, "first")
	secondPath := filepath.Join(root, "second")

	runInitPostInit(t, binary, configFile, stateFile, firstPath)
	second := runInitPostInit(t, binary, configFile, stateFile, secondPath)
	if got := second["switch_back"]; got != `rvn --json vault use -- 'first'` {
		t.Fatalf("switch_back = %#v, want exact restore command", got)
	}

	ambient := exec.Command(
		binary,
		"--config", configFile,
		"--state", stateFile,
		"--json",
		"schema", "add", "type", "ambient-second", "--name-field", "title",
	)
	ambientOutput, err := ambient.CombinedOutput()
	if err != nil {
		t.Fatalf("ambient write to auto-activated vault failed: %v\n%s", err, ambientOutput)
	}
	firstSchema, err := os.ReadFile(filepath.Join(firstPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read first schema: %v", err)
	}
	if strings.Contains(string(firstSchema), "ambient-second") {
		t.Fatalf("ambient write incorrectly reached previous vault:\n%s", firstSchema)
	}
	secondSchema, err := os.ReadFile(filepath.Join(secondPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read second schema: %v", err)
	}
	if !strings.Contains(string(secondSchema), "ambient-second") {
		t.Fatalf("ambient write did not reach auto-activated vault:\n%s", secondSchema)
	}

	explicit := exec.Command(
		binary,
		"--config", configFile,
		"--state", stateFile,
		"--vault", "second",
		"--json",
		"schema", "add", "type", "explicit-second", "--name-field", "title",
	)
	explicitOutput, err := explicit.CombinedOutput()
	if err != nil {
		t.Fatalf("explicit second-vault write failed: %v\n%s", err, explicitOutput)
	}
	secondSchema, err = os.ReadFile(filepath.Join(secondPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read second schema: %v", err)
	}
	if !strings.Contains(string(secondSchema), "explicit-second") {
		t.Fatalf("explicit write did not reach second vault:\n%s", secondSchema)
	}

	use := exec.Command(binary, "--config", configFile, "--state", stateFile, "--json", "vault", "use", "--", "first")
	if useOutput, useErr := use.CombinedOutput(); useErr != nil {
		t.Fatalf("switch back to first vault: %v\n%s", useErr, useOutput)
	}
	afterUse := exec.Command(
		binary,
		"--config", configFile,
		"--state", stateFile,
		"--json",
		"schema", "add", "type", "restored-first", "--name-field", "title",
	)
	if afterUseOutput, afterUseErr := afterUse.CombinedOutput(); afterUseErr != nil {
		t.Fatalf("ambient write after switch-back failed: %v\n%s", afterUseErr, afterUseOutput)
	}
	firstSchema, err = os.ReadFile(filepath.Join(firstPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read first schema after switch-back: %v", err)
	}
	if !strings.Contains(string(firstSchema), "restored-first") {
		t.Fatalf("ambient write after switch-back did not reach first vault:\n%s", firstSchema)
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
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected JSON failure to exit nonzero, got %v\n%s", err, output)
	}
	if got := exitErr.ExitCode(); got != 1 {
		t.Fatalf("exit code=%d, want 1\n%s", got, output)
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
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected JSON failure to exit nonzero, got %v\n%s", err, output)
	}
	if got := exitErr.ExitCode(); got != 1 {
		t.Fatalf("exit code=%d, want 1\n%s", got, output)
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

func TestIntegration_JSONSuccessReturnsZeroExit(t *testing.T) {
	t.Parallel()

	binary := testutil.BuildCLI(t)
	configFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configFile, nil, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(binary, "--config", configFile, "--json", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected JSON success to exit zero: %v\n%s", err, output)
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal version response: %v\n%s", err, output)
	}
	if !resp.OK {
		t.Fatalf("expected success envelope: %s", output)
	}
}
