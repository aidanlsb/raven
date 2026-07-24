package commandimpl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
)

func TestHandleReindexPropagatesCallerCancellation(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("write markdown fixture: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := HandleReindex(ctx, commandexec.Request{
		VaultPath: vaultPath,
		Args: map[string]any{
			"dry-run": true,
		},
	})
	if result.OK {
		t.Fatalf("expected failure for canceled context, got success: %#v", result)
	}
	if result.Error == nil {
		t.Fatalf("expected error payload, got %#v", result)
	}
	if result.Error.Code != "FILE_READ_ERROR" {
		t.Fatalf("error code = %q, want %q", result.Error.Code, "FILE_READ_ERROR")
	}
	if !strings.Contains(result.Error.Message, "context canceled") {
		t.Fatalf("error message = %q, want substring %q", result.Error.Message, "context canceled")
	}
}

func TestSetupInitVaultFirstVaultRegistersPinsAndActivates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "My Notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	data, warnings, setupErr := setupInitVault(vaultPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	if got := data["suggested_name"]; got != "my-notes" {
		t.Fatalf("suggested_name = %#v, want %q", got, "my-notes")
	}
	if got := data["registered_name"]; got != "my-notes" {
		t.Fatalf("registered_name = %#v, want %q", got, "my-notes")
	}
	assertPostInitBool(t, data, "already_registered", true)
	assertPostInitBool(t, data, "registered", true)
	assertPostInitBool(t, data, "is_first_vault", true)
	assertPostInitBool(t, data, "has_existing_default", false)
	assertPostInitBool(t, data, "is_default", true)
	assertPostInitBool(t, data, "is_active", true)
	assertPostInitBool(t, data, "activated", true)
	assertPostInitBool(t, data, "needs_user_choice_for_activate", false)
	assertPostInitBool(t, data, "needs_user_choice_for_default", false)

	steps, ok := data["next_steps"].([]string)
	if !ok {
		t.Fatalf("next_steps = %#v, want []string", data["next_steps"])
	}
	if len(steps) != 0 {
		t.Fatalf("next_steps len = %d, want 0 for first vault (%#v)", len(steps), steps)
	}

	// A first vault is fully configured: no pending actions for the agent.
	actions, ok := data["actions"].(map[string]interface{})
	if !ok {
		t.Fatalf("actions = %#v, want map", data["actions"])
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %#v, want empty for first vault", actions)
	}

	ctx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath})
	if err != nil {
		t.Fatalf("load vault context: %v", err)
	}
	if got := ctx.Cfg.Vaults["my-notes"]; got != filepath.Clean(vaultPath) {
		t.Fatalf("vault path = %q, want %q", got, filepath.Clean(vaultPath))
	}
	if ctx.Cfg.DefaultVault != "my-notes" {
		t.Fatalf("default_vault = %q, want %q", ctx.Cfg.DefaultVault, "my-notes")
	}
	if ctx.State.ActiveVault != "my-notes" {
		t.Fatalf("active_vault = %q, want %q", ctx.State.ActiveVault, "my-notes")
	}
	assertPostInitVaultInfo(t, data, "active_vault", "my-notes", filepath.Clean(vaultPath))
	if data["previous_active_vault"] != nil {
		t.Fatalf("previous_active_vault = %#v, want nil", data["previous_active_vault"])
	}
}

func TestSetupInitVaultWithExistingDefaultRegistersAndActivates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")

	existingPath := filepath.Join(root, "existing")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("mkdir existing: %v", err)
	}
	cfg := &config.Config{
		DefaultVault: "existing",
		Vaults:       map[string]string{"existing": existingPath},
	}
	if err := config.SaveTo(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{ActiveVault: "existing"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	vaultPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	data, warnings, setupErr := setupInitVault(vaultPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	if got := data["registered_name"]; got != "notes" {
		t.Fatalf("registered_name = %#v, want %q", got, "notes")
	}
	assertPostInitBool(t, data, "already_registered", true)
	assertPostInitBool(t, data, "registered", true)
	assertPostInitBool(t, data, "is_first_vault", false)
	assertPostInitBool(t, data, "has_existing_default", true)
	assertPostInitBool(t, data, "is_default", false)
	assertPostInitBool(t, data, "is_active", true)
	assertPostInitBool(t, data, "activated", true)
	assertPostInitBool(t, data, "needs_user_choice_for_activate", false)
	assertPostInitBool(t, data, "needs_user_choice_for_default", true)

	// The default stays unchanged, while ambient routing switches to the new vault.
	ctx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath})
	if err != nil {
		t.Fatalf("load vault context: %v", err)
	}
	if ctx.Cfg.DefaultVault != "existing" {
		t.Fatalf("default_vault = %q, want %q", ctx.Cfg.DefaultVault, "existing")
	}
	if ctx.State.ActiveVault != "notes" {
		t.Fatalf("active_vault = %q, want %q", ctx.State.ActiveVault, "notes")
	}
	if got := ctx.Cfg.Vaults["notes"]; got != filepath.Clean(vaultPath) {
		t.Fatalf("notes vault path = %q, want %q", got, filepath.Clean(vaultPath))
	}

	assertPostInitVaultInfo(t, data, "active_vault", "notes", filepath.Clean(vaultPath))
	assertPostInitVaultInfo(t, data, "previous_active_vault", "existing", filepath.Clean(existingPath))
	if got := data["switch_back"]; got != `rvn --json vault use -- 'existing'` {
		t.Fatalf("switch_back = %#v, want restore command", got)
	}
	steps, ok := data["next_steps"].([]string)
	if !ok || len(steps) == 0 || !strings.Contains(steps[0], `rvn --json vault use -- 'existing'`) {
		t.Fatalf("next_steps = %#v, want switch-back command", data["next_steps"])
	}

	// Changing the default remains the only pending action.
	actions, ok := data["actions"].(map[string]interface{})
	if !ok {
		t.Fatalf("actions = %#v, want map", data["actions"])
	}
	if _, ok := actions["activate"]; ok {
		t.Fatalf("actions.activate = %#v, want automatic activation", actions["activate"])
	}
	setDefault, ok := actions["set_default"].(map[string]interface{})
	if !ok || setDefault["command"] != "vault pin" {
		t.Fatalf("actions.set_default = %#v, want command=vault pin", actions["set_default"])
	}
}

func TestSetupInitVaultResolvesNameCollision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")

	otherPath := filepath.Join(root, "other")
	if err := os.MkdirAll(otherPath, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	cfg := &config.Config{
		DefaultVault: "notes",
		Vaults:       map[string]string{"notes": otherPath},
	}
	if err := config.SaveTo(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	vaultPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	data, warnings, setupErr := setupInitVault(vaultPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if got := data["registered_name"]; got != "notes-2" {
		t.Fatalf("registered_name = %#v, want %q", got, "notes-2")
	}
	assertPostInitBool(t, data, "registered", true)
	assertPostInitBool(t, data, "is_first_vault", false)
	assertPostInitBool(t, data, "has_existing_default", true)
	assertPostInitBool(t, data, "is_active", true)
	assertPostInitBool(t, data, "activated", true)
	assertPostInitVaultInfo(t, data, "active_vault", "notes-2", filepath.Clean(vaultPath))
	assertPostInitVaultInfo(t, data, "previous_vault", "notes", filepath.Clean(otherPath))
	if got := data["switch_back"]; got != "rvn --json vault clear" {
		t.Fatalf("switch_back = %#v, want vault clear", got)
	}
}

func TestSetupInitVaultDisclosesRoutingChangeFromMissingActive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	firstPath := filepath.Join(root, "first")
	secondPath := filepath.Join(root, "second")
	for _, path := range []string{firstPath, secondPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir vault: %v", err)
		}
	}
	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "first",
		Vaults:       map[string]string{"first": firstPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{ActiveVault: "second"}); err != nil {
		t.Fatalf("save stale active state: %v", err)
	}

	data, warnings, setupErr := setupInitVault(secondPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	assertPostInitBool(t, data, "activated", true)
	assertPostInitVaultInfo(t, data, "active_vault", "second", secondPath)
	if data["previous_active_vault"] != nil {
		t.Fatalf("previous_active_vault = %#v, want nil for missing active name", data["previous_active_vault"])
	}
	if data["previous_vault"] != nil {
		t.Fatalf("previous_vault = %#v, want nil when stale active state prevents ambient resolution", data["previous_vault"])
	}
	if got := data["switch_back"]; got != "rvn --json vault clear" {
		t.Fatalf("switch_back = %#v, want vault clear", got)
	}
}

func TestSetupInitVaultProvidesSwitchBackWithoutPriorSelection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	oldPath := filepath.Join(root, "old")
	newPath := filepath.Join(root, "new")
	for _, path := range []string{oldPath, newPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir vault: %v", err)
		}
	}
	if err := config.SaveTo(configPath, &config.Config{
		Vaults: map[string]string{"old": oldPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	data, warnings, setupErr := setupInitVault(newPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	assertPostInitBool(t, data, "activated", true)
	if data["previous_active_vault"] != nil || data["previous_vault"] != nil {
		t.Fatalf("previous vaults = active:%#v resolved:%#v, want nil", data["previous_active_vault"], data["previous_vault"])
	}
	if got := data["switch_back"]; got != "rvn --json vault clear" {
		t.Fatalf("switch_back = %#v, want vault clear", got)
	}
}

func TestSetupInitVaultRepairsStaleDefaultWithEffectiveSwitchBack(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	oldPath := filepath.Join(root, "old")
	newPath := filepath.Join(root, "new")
	for _, path := range []string{oldPath, newPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir vault: %v", err)
		}
	}
	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "new",
		Vaults:       map[string]string{"old": oldPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	data, warnings, setupErr := setupInitVault(newPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	assertPostInitBool(t, data, "activated", true)
	if got := data["switch_back"]; got != "rvn --json config unset default_vault && rvn --json vault clear" {
		t.Fatalf("switch_back = %#v, want command that restores no-selection behavior", got)
	}
}

func TestSetupInitVaultAlreadyRegisteredSoleVaultPinsAndActivates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")

	vaultPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	// Registered by path, but no default and no active vault yet.
	cfg := &config.Config{Vaults: map[string]string{"notes": vaultPath}}
	if err := config.SaveTo(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	data, warnings, setupErr := setupInitVault(vaultPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	assertPostInitBool(t, data, "already_registered", true)
	assertPostInitBool(t, data, "registered", false)
	assertPostInitBool(t, data, "is_first_vault", true)
	assertPostInitBool(t, data, "is_default", true)
	assertPostInitBool(t, data, "is_active", true)
	assertPostInitBool(t, data, "activated", true)

	ctx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath})
	if err != nil {
		t.Fatalf("load vault context: %v", err)
	}
	if ctx.Cfg.DefaultVault != "notes" {
		t.Fatalf("default_vault = %q, want %q", ctx.Cfg.DefaultVault, "notes")
	}
	if ctx.State.ActiveVault != "notes" {
		t.Fatalf("active_vault = %q, want %q", ctx.State.ActiveVault, "notes")
	}
}

func TestSetupInitVaultReportsConfiguredPathForSymlinkAlias(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "notes")
	aliasPath := filepath.Join(root, "notes-alias")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.Symlink(vaultPath, aliasPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := config.SaveTo(configPath, &config.Config{
		Vaults: map[string]string{"notes": vaultPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	data, warnings, setupErr := setupInitVault(aliasPath, configPath, statePath)
	if setupErr != nil {
		t.Fatalf("setup init vault: %v", setupErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	assertPostInitVaultInfo(t, data, "active_vault", "notes", vaultPath)
}

func TestSetupInitVaultFailsWhenGlobalStateCannotLoad(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := config.SaveTo(configPath, &config.Config{}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("active_vault = [\n"), 0o644); err != nil {
		t.Fatalf("write invalid state: %v", err)
	}

	data, warnings, setupErr := setupInitVault(vaultPath, configPath, "")
	if setupErr == nil {
		t.Fatal("expected setup failure for invalid state")
	}
	var typedErr *initVaultSetupError
	if !errors.As(setupErr, &typedErr) {
		t.Fatalf("setup error type = %T, want *initVaultSetupError", setupErr)
	}
	if typedErr.code != codes.ErrConfigInvalid {
		t.Fatalf("setup error code = %q, want %q", typedErr.code, codes.ErrConfigInvalid)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none on fatal setup failure", warnings)
	}
	if got := data["state_path"]; got != statePath {
		t.Fatalf("state_path = %#v, want %q", got, statePath)
	}
	if guidance, _ := data["guidance"].(string); !strings.Contains(guidance, "could not be registered and activated") {
		t.Fatalf("guidance = %q, want failed setup warning", guidance)
	}
}

func assertPostInitBool(t *testing.T, data map[string]interface{}, key string, want bool) {
	t.Helper()
	got, ok := data[key].(bool)
	if !ok {
		t.Fatalf("%s = %#v, want bool", key, data[key])
	}
	if got != want {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
}

func assertPostInitVaultInfo(t *testing.T, data map[string]interface{}, key, wantName, wantPath string) {
	t.Helper()
	got, ok := data[key].(map[string]interface{})
	if !ok {
		t.Fatalf("%s = %#v, want map", key, data[key])
	}
	if got["name"] != wantName || got["path"] != wantPath {
		t.Fatalf("%s = %#v, want name=%q path=%q", key, got, wantName, wantPath)
	}
}

func TestFormatSuggestedCommandPathNormalizesWindowsSeparators(t *testing.T) {
	t.Parallel()
	got := formatSuggestedCommandPath(`C:\Users\me\New Notes`)
	want := `'C:/Users/me/New Notes'`
	if got != want {
		t.Fatalf("formatSuggestedCommandPath() = %q, want %q", got, want)
	}
}

func TestSetInitSwitchBackShellQuotesVaultName(t *testing.T) {
	t.Parallel()
	state := initPostInitState{
		previousActiveName: "work$(touch /tmp/pwn)",
		previousActivePath: "/vault/work",
	}
	setInitSwitchBack(&state)
	want := "rvn --json vault use -- 'work$(touch /tmp/pwn)'"
	if state.switchBack != want {
		t.Fatalf("switch_back = %q, want %q", state.switchBack, want)
	}
}
