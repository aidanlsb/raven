package configsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestPendingInitVaultConflictFor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	firstPath := filepath.Join(root, "first")
	secondPath := filepath.Join(root, "second")
	cfg := &config.Config{Vaults: map[string]string{
		"first":  firstPath,
		"second": secondPath,
	}}
	state := &config.State{PendingInitVaultPath: secondPath}

	tests := []struct {
		name        string
		currentName string
		currentPath string
		pending     bool
		want        bool
	}{
		{name: "different ambient vault conflicts", currentName: "first", currentPath: firstPath, pending: true, want: true},
		{name: "initialized vault is safe", currentName: "second", currentPath: secondPath, pending: true, want: false},
		{name: "explicit decision no longer pending", currentName: "first", currentPath: firstPath, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testState := *state
			if !tt.pending {
				testState.PendingInitVaultPath = ""
			}
			got := PendingInitVaultConflictFor(cfg, &testState, tt.currentName, tt.currentPath)
			if (got != nil) != tt.want {
				t.Fatalf("conflict = %#v, want present=%v", got, tt.want)
			}
			if got != nil {
				if got.PendingName != "second" || got.CurrentName != "first" {
					t.Fatalf("conflict = %#v, want pending=second current=first", got)
				}
				if !strings.Contains(got.Message(), "unqualified command would target") {
					t.Fatalf("message = %q, want wrong-target explanation", got.Message())
				}
			}
		})
	}
}

func TestPendingInitVaultConflictTreatsSymlinkAsSameVault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	vaultPath := filepath.Join(root, "vault")
	aliasPath := filepath.Join(root, "vault-alias")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.Symlink(vaultPath, aliasPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	state := &config.State{PendingInitVaultPath: aliasPath}
	if conflict := PendingInitVaultConflictFor(&config.Config{}, state, "", vaultPath); conflict != nil {
		t.Fatalf("symlink alias produced conflict: %#v", conflict)
	}
}

func TestUseVaultClearsPendingInitVault(t *testing.T) {
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
	if err := config.SaveTo(configPath, &config.Config{Vaults: map[string]string{
		"first":  firstPath,
		"second": secondPath,
	}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		ActiveVault:          "first",
		PendingInitVaultPath: secondPath,
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if _, err := UseVault(ContextOptions{
		ConfigPathOverride: configPath,
		StatePathOverride:  statePath,
	}, "second"); err != nil {
		t.Fatalf("use vault: %v", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveVault != "second" {
		t.Fatalf("active_vault = %q, want second", state.ActiveVault)
	}
	if state.PendingInitVaultPath != "" {
		t.Fatalf("pending_init_vault_path = %q, want empty", state.PendingInitVaultPath)
	}
}

func TestStateFileChangesRequireResolvedInitSelection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	oldStatePath := filepath.Join(root, "old-state.toml")
	newStatePath := filepath.Join(root, "new-state.toml")
	pendingPath := filepath.Join(root, "new-vault")
	if err := config.SaveTo(configPath, &config.Config{StateFile: filepath.Base(oldStatePath)}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(oldStatePath, &config.State{PendingInitVaultPath: pendingPath}); err != nil {
		t.Fatalf("save old state: %v", err)
	}

	newStateFile := filepath.Base(newStatePath)
	_, err := Set(SetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath},
		StateFile:      &newStateFile,
	})
	if err == nil {
		t.Fatal("expected state_file change to fail while init selection is pending")
	}
	svcErr, ok := AsError(err)
	if !ok || svcErr.Code != CodeVaultAmbiguous {
		t.Fatalf("set state_file error = %#v, want VAULT_AMBIGUOUS", err)
	}
	loadedCfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if loadedCfg.StateFile != filepath.Base(oldStatePath) {
		t.Fatalf("state_file = %q, want unchanged", loadedCfg.StateFile)
	}
	if _, err := os.Stat(newStatePath); !os.IsNotExist(err) {
		t.Fatalf("destination state should not be created, stat error = %v", err)
	}

	if err := config.SaveState(oldStatePath, &config.State{}); err != nil {
		t.Fatalf("clear pending state: %v", err)
	}
	result, err := Set(SetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath},
		StateFile:      &newStateFile,
	})
	if err != nil {
		t.Fatalf("set state_file after resolving selection: %v", err)
	}
	if result.Context.StatePath != newStatePath {
		t.Fatalf("result state_path = %q, want %q", result.Context.StatePath, newStatePath)
	}
	if err := config.SaveState(newStatePath, &config.State{PendingInitVaultPath: pendingPath}); err != nil {
		t.Fatalf("save pending new state: %v", err)
	}
	if _, err := Unset(UnsetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath},
		StateFile:      true,
	}); err == nil {
		t.Fatal("expected state_file unset to fail while init selection is pending")
	}
	loadedCfg, err = config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("reload config after unset rejection: %v", err)
	}
	if loadedCfg.StateFile != filepath.Base(newStatePath) {
		t.Fatalf("state_file = %q, want unchanged after unset rejection", loadedCfg.StateFile)
	}
}
