package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestVaultRows(t *testing.T) {
	cfg := &config.Config{
		DefaultVault: "work",
		Vaults: map[string]string{
			"work":     "/vault/work",
			"personal": "/vault/personal",
		},
	}
	state := &config.State{ActiveVault: "personal"}

	rows, defaultName, activeName, activeMissing := vaultRows(cfg, state)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if defaultName != "work" {
		t.Fatalf("expected default_name=work, got %q", defaultName)
	}
	if activeName != "personal" {
		t.Fatalf("expected active_name=personal, got %q", activeName)
	}
	if activeMissing {
		t.Fatalf("expected active_missing=false")
	}
}

func TestResolveCurrentVault(t *testing.T) {
	t.Run("prefers active vault", func(t *testing.T) {
		cfg := &config.Config{
			DefaultVault: "work",
			Vaults: map[string]string{
				"work":     "/vault/work",
				"personal": "/vault/personal",
			},
		}
		state := &config.State{ActiveVault: "personal"}

		got, err := resolveCurrentVault(cfg, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "personal" {
			t.Fatalf("expected name=personal, got %q", got.Name)
		}
		if got.Path != "/vault/personal" {
			t.Fatalf("expected path=/vault/personal, got %q", got.Path)
		}
		if got.Source != "active_vault" {
			t.Fatalf("expected source=active_vault, got %q", got.Source)
		}
	})

	t.Run("falls back to default when active missing", func(t *testing.T) {
		cfg := &config.Config{
			DefaultVault: "work",
			Vaults: map[string]string{
				"work": "/vault/work",
			},
		}
		state := &config.State{ActiveVault: "personal"}

		got, err := resolveCurrentVault(cfg, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "work" {
			t.Fatalf("expected name=work, got %q", got.Name)
		}
		if got.Source != "default_vault_fallback" {
			t.Fatalf("expected source=default_vault_fallback, got %q", got.Source)
		}
		if !got.ActiveMissing {
			t.Fatalf("expected active_missing=true")
		}
	})

	t.Run("errors when neither active nor default is available", func(t *testing.T) {
		cfg := &config.Config{}
		state := &config.State{ActiveVault: "missing"}

		_, err := resolveCurrentVault(cfg, state)
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestVaultUseCreatesStateFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")

	content := `default_vault = "work"

[vaults]
work = "/vault/work"
personal = "/vault/personal"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("expected state file to not exist yet")
	}

	if err := vaultUseCmd.RunE(vaultUseCmd, []string{"personal"}); err != nil {
		t.Fatalf("vaultUseCmd.RunE: %v", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveVault != "personal" {
		t.Fatalf("expected active_vault=personal, got %q", state.ActiveVault)
	}
}

func TestVaultPinUpdatesDefaultVault(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")

	content := `default_vault = "work"

[vaults]
work = "/vault/work"
personal = "/vault/personal"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true

	if err := vaultPinCmd.RunE(vaultPinCmd, []string{"personal"}); err != nil {
		t.Fatalf("vaultPinCmd.RunE: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.DefaultVault != "personal" {
		t.Fatalf("expected default_vault=personal, got %q", cfg.DefaultVault)
	}
}

func TestVaultClearRemovesActiveVault(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")

	cfgContent := `default_vault = "work"

[vaults]
work = "/vault/work"
personal = "/vault/personal"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stateContent := `version = 1
active_vault = "personal"
`
	if err := os.WriteFile(statePath, []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true

	if err := vaultClearCmd.RunE(vaultClearCmd, []string{}); err != nil {
		t.Fatalf("vaultClearCmd.RunE: %v", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if strings.TrimSpace(state.ActiveVault) != "" {
		t.Fatalf("expected empty active_vault, got %q", state.ActiveVault)
	}
}
