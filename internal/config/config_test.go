package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigGetVaultPath(t *testing.T) {
	t.Run("named vault", func(t *testing.T) {
		cfg := &Config{
			Vaults: map[string]string{
				"work":     "/path/to/work",
				"personal": "/path/to/personal",
			},
		}

		path, err := cfg.GetVaultPath("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/path/to/work" {
			t.Errorf("expected '/path/to/work', got %q", path)
		}
	})

	t.Run("default vault", func(t *testing.T) {
		cfg := &Config{
			DefaultVault: "personal",
			Vaults: map[string]string{
				"work":     "/path/to/work",
				"personal": "/path/to/personal",
			},
		}

		path, err := cfg.GetVaultPath("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/path/to/personal" {
			t.Errorf("expected '/path/to/personal', got %q", path)
		}
	})

	t.Run("legacy vault fallback", func(t *testing.T) {
		cfg := &Config{
			Vault: "/legacy/vault/path",
		}

		path, err := cfg.GetVaultPath("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/legacy/vault/path" {
			t.Errorf("expected '/legacy/vault/path', got %q", path)
		}
	})

	t.Run("legacy vault supports default alias", func(t *testing.T) {
		cfg := &Config{
			Vault: "/legacy/vault/path",
		}

		path, err := cfg.GetVaultPath("default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/legacy/vault/path" {
			t.Errorf("expected '/legacy/vault/path', got %q", path)
		}
	})

	t.Run("vault not found", func(t *testing.T) {
		cfg := &Config{
			Vaults: map[string]string{
				"work": "/path/to/work",
			},
		}

		_, err := cfg.GetVaultPath("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent vault")
		}
	})

	t.Run("no default configured", func(t *testing.T) {
		cfg := &Config{}

		_, err := cfg.GetVaultPath("")
		if err == nil {
			t.Error("expected error when no default configured")
		}
	})
}

func TestConfigGetDefaultVaultPath(t *testing.T) {
	cfg := &Config{
		DefaultVault: "main",
		Vaults: map[string]string{
			"main": "/path/to/main",
		},
	}

	path, err := cfg.GetDefaultVaultPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/path/to/main" {
		t.Errorf("expected '/path/to/main', got %q", path)
	}
}

func TestConfigListVaults(t *testing.T) {
	t.Run("named vaults", func(t *testing.T) {
		cfg := &Config{
			Vaults: map[string]string{
				"work":     "/path/to/work",
				"personal": "/path/to/personal",
			},
		}

		vaults := cfg.ListVaults()
		if len(vaults) != 2 {
			t.Errorf("expected 2 vaults, got %d", len(vaults))
		}
		if vaults["work"] != "/path/to/work" {
			t.Error("missing 'work' vault")
		}
		if vaults["personal"] != "/path/to/personal" {
			t.Error("missing 'personal' vault")
		}
	})

	t.Run("legacy vault as default", func(t *testing.T) {
		cfg := &Config{
			Vault: "/legacy/path",
		}

		vaults := cfg.ListVaults()
		if len(vaults) != 1 {
			t.Errorf("expected 1 vault, got %d", len(vaults))
		}
		if vaults["default"] != "/legacy/path" {
			t.Error("expected legacy vault as 'default'")
		}
	})

	t.Run("empty config", func(t *testing.T) {
		cfg := &Config{}

		vaults := cfg.ListVaults()
		if len(vaults) != 0 {
			t.Errorf("expected 0 vaults, got %d", len(vaults))
		}
	})

	t.Run("named vaults take precedence over legacy", func(t *testing.T) {
		cfg := &Config{
			Vault: "/legacy/path",
			Vaults: map[string]string{
				"main": "/named/path",
			},
		}

		vaults := cfg.ListVaults()
		if len(vaults) != 1 {
			t.Errorf("expected 1 vault, got %d", len(vaults))
		}
		if _, ok := vaults["default"]; ok {
			t.Error("legacy vault should not appear when named vaults exist")
		}
	})
}

func TestConfigGetEditor(t *testing.T) {
	t.Run("configured editor", func(t *testing.T) {
		cfg := &Config{Editor: "vim"}
		if cfg.GetEditor() != "vim" {
			t.Errorf("expected 'vim', got %q", cfg.GetEditor())
		}
	})

	t.Run("falls back to EDITOR env", func(t *testing.T) {
		cfg := &Config{}

		// Save and restore EDITOR
		oldEditor := os.Getenv("EDITOR")
		os.Setenv("EDITOR", "nano")
		defer os.Setenv("EDITOR", oldEditor)

		if cfg.GetEditor() != "nano" {
			t.Errorf("expected 'nano', got %q", cfg.GetEditor())
		}
	})

	t.Run("empty when no editor configured", func(t *testing.T) {
		cfg := &Config{}

		oldEditor := os.Getenv("EDITOR")
		os.Unsetenv("EDITOR")
		defer os.Setenv("EDITOR", oldEditor)

		if cfg.GetEditor() != "" {
			t.Errorf("expected empty string, got %q", cfg.GetEditor())
		}
	})
}

func TestLoadFrom(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Note: In TOML, keys after a [section] belong to that section.
	// editor needs to come before [vaults] or after vault definitions.
	content := `default_vault = "work"
state_file = "state.toml"
editor = "code"

[vaults]
work = "/path/to/work"
personal = "/path/to/personal"

[ui]
accent = "39"
code_theme = "dracula"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DefaultVault != "work" {
		t.Errorf("expected default_vault 'work', got %q", cfg.DefaultVault)
	}
	if cfg.StateFile != "state.toml" {
		t.Errorf("expected state_file 'state.toml', got %q", cfg.StateFile)
	}
	if cfg.Editor != "code" {
		t.Errorf("expected editor 'code', got %q", cfg.Editor)
	}
	if len(cfg.Vaults) != 2 {
		t.Errorf("expected 2 vaults, got %d: %v", len(cfg.Vaults), cfg.Vaults)
	}
	if cfg.UI.Accent != "39" {
		t.Errorf("expected ui.accent '39', got %q", cfg.UI.Accent)
	}
	if cfg.UI.CodeTheme != "dracula" {
		t.Errorf("expected ui.code_theme 'dracula', got %q", cfg.UI.CodeTheme)
	}
}

func TestLoadFromInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Invalid TOML
	content := `this is not valid toml {{{{`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadFrom(configPath)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoad(t *testing.T) {
	// Load should return empty config when file doesn't exist
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return a valid (possibly empty) config
	if cfg == nil {
		t.Error("expected non-nil config")
	}
}

func TestXDGPath(t *testing.T) {
	path, err := XDGPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain .config/raven/config.toml
	if filepath.Base(path) != "config.toml" {
		t.Errorf("expected config.toml, got %s", filepath.Base(path))
	}
}
