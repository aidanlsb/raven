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

func TestVaultAddAddsAndPinsVault(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	vaultPath := filepath.Join(tmp, "vault-personal")

	if err := os.WriteFile(cfgPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("create vault path: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevReplace := vaultAddReplace
	prevPin := vaultAddPin
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		vaultAddReplace = prevReplace
		vaultAddPin = prevPin
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true
	vaultAddReplace = false
	vaultAddPin = true

	if err := vaultAddCmd.RunE(vaultAddCmd, []string{"personal", vaultPath}); err != nil {
		t.Fatalf("vaultAddCmd.RunE: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := cfg.DefaultVault; got != "personal" {
		t.Fatalf("expected default_vault=personal, got %q", got)
	}
	if got := cfg.Vaults["personal"]; got != vaultPath {
		t.Fatalf("expected vault path %q, got %q", vaultPath, got)
	}
}

func TestVaultAddRejectsDuplicateWithoutReplace(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	oldPath := filepath.Join(tmp, "vault-work")
	newPath := filepath.Join(tmp, "vault-work-new")

	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("create old path: %v", err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("create new path: %v", err)
	}

	cfgContent := `default_vault = "work"

[vaults]
work = "` + oldPath + `"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevReplace := vaultAddReplace
	prevPin := vaultAddPin
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		vaultAddReplace = prevReplace
		vaultAddPin = prevPin
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = false
	vaultAddReplace = false
	vaultAddPin = false

	err := vaultAddCmd.RunE(vaultAddCmd, []string{"work", newPath})
	if err == nil {
		t.Fatalf("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate-name error message, got %v", err)
	}

	cfg, loadErr := config.LoadFrom(cfgPath)
	if loadErr != nil {
		t.Fatalf("reload config: %v", loadErr)
	}
	if got := cfg.Vaults["work"]; got != oldPath {
		t.Fatalf("expected existing path to remain %q, got %q", oldPath, got)
	}
}

func TestVaultRemoveRequiresClearFlagsForDefaultAndActive(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	workPath := filepath.Join(tmp, "vault-work")
	personalPath := filepath.Join(tmp, "vault-personal")

	if err := os.MkdirAll(workPath, 0o755); err != nil {
		t.Fatalf("create work path: %v", err)
	}
	if err := os.MkdirAll(personalPath, 0o755); err != nil {
		t.Fatalf("create personal path: %v", err)
	}

	cfgContent := `default_vault = "work"

[vaults]
work = "` + workPath + `"
personal = "` + personalPath + `"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	stateContent := `version = 1
active_vault = "work"
`
	if err := os.WriteFile(statePath, []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevClearDefault := vaultRemoveClearDefault
	prevClearActive := vaultRemoveClearActive
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		vaultRemoveClearDefault = prevClearDefault
		vaultRemoveClearActive = prevClearActive
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = false
	vaultRemoveClearDefault = false
	vaultRemoveClearActive = false

	err := vaultRemoveCmd.RunE(vaultRemoveCmd, []string{"work"})
	if err == nil {
		t.Fatalf("expected confirmation-required error")
	}
	if !strings.Contains(err.Error(), "default vault") {
		t.Fatalf("expected default vault error, got %v", err)
	}
}

func TestVaultRemoveClearsDefaultAndActive(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	workPath := filepath.Join(tmp, "vault-work")
	personalPath := filepath.Join(tmp, "vault-personal")

	if err := os.MkdirAll(workPath, 0o755); err != nil {
		t.Fatalf("create work path: %v", err)
	}
	if err := os.MkdirAll(personalPath, 0o755); err != nil {
		t.Fatalf("create personal path: %v", err)
	}

	cfgContent := `default_vault = "work"

[vaults]
work = "` + workPath + `"
personal = "` + personalPath + `"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	stateContent := `version = 1
active_vault = "work"
`
	if err := os.WriteFile(statePath, []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevClearDefault := vaultRemoveClearDefault
	prevClearActive := vaultRemoveClearActive
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		vaultRemoveClearDefault = prevClearDefault
		vaultRemoveClearActive = prevClearActive
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true
	vaultRemoveClearDefault = true
	vaultRemoveClearActive = true

	if err := vaultRemoveCmd.RunE(vaultRemoveCmd, []string{"work"}); err != nil {
		t.Fatalf("vaultRemoveCmd.RunE: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := cfg.DefaultVault; got != "" {
		t.Fatalf("expected default_vault to be cleared, got %q", got)
	}
	if _, ok := cfg.Vaults["work"]; ok {
		t.Fatalf("expected work vault to be removed")
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if strings.TrimSpace(state.ActiveVault) != "" {
		t.Fatalf("expected active_vault to be cleared, got %q", state.ActiveVault)
	}
}
