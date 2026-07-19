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

func TestStateFileChangesPreservePendingInitDecision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	defaultStatePath := filepath.Join(root, "state.toml")
	oldStatePath := filepath.Join(root, "old-state.toml")
	newStatePath := filepath.Join(root, "new-state.toml")
	pendingPath := filepath.Join(root, "new-vault")

	if err := config.SaveTo(configPath, &config.Config{StateFile: filepath.Base(oldStatePath)}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(oldStatePath, &config.State{
		ActiveVault:          "first",
		PendingInitVaultPath: pendingPath,
	}); err != nil {
		t.Fatalf("save old state: %v", err)
	}
	if err := config.SaveState(newStatePath, &config.State{ActiveVault: "second"}); err != nil {
		t.Fatalf("save new state: %v", err)
	}

	newStateFile := filepath.Base(newStatePath)
	if _, err := Set(SetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath},
		StateFile:      &newStateFile,
	}); err != nil {
		t.Fatalf("set state file: %v", err)
	}
	newState, err := config.LoadState(newStatePath)
	if err != nil {
		t.Fatalf("load new state: %v", err)
	}
	if newState.ActiveVault != "second" {
		t.Fatalf("new active_vault = %q, want preserved destination value second", newState.ActiveVault)
	}
	if newState.PendingInitVaultPath != pendingPath {
		t.Fatalf("new pending_init_vault_path = %q, want %q", newState.PendingInitVaultPath, pendingPath)
	}

	// Clearing the decision in the configured state and then returning to the
	// default state path must also clear any stale marker there.
	newState.PendingInitVaultPath = ""
	if err := config.SaveState(newStatePath, newState); err != nil {
		t.Fatalf("clear new pending state: %v", err)
	}
	if err := config.SaveState(defaultStatePath, &config.State{PendingInitVaultPath: pendingPath}); err != nil {
		t.Fatalf("save stale default state: %v", err)
	}
	if _, err := Unset(UnsetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath},
		StateFile:      true,
	}); err != nil {
		t.Fatalf("unset state file: %v", err)
	}
	defaultState, err := config.LoadState(defaultStatePath)
	if err != nil {
		t.Fatalf("load default state: %v", err)
	}
	if defaultState.PendingInitVaultPath != "" {
		t.Fatalf("default pending_init_vault_path = %q, want cleared", defaultState.PendingInitVaultPath)
	}
}
