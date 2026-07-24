package config

import (
	"os"
	"path/filepath"
	"strings"
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

	t.Run("default name requires registry entry", func(t *testing.T) {
		cfg := &Config{DefaultVault: "default"}

		_, err := cfg.GetVaultPath("")
		if err == nil {
			t.Error("expected error when default is not present in vaults")
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

	t.Run("empty config", func(t *testing.T) {
		cfg := &Config{}

		vaults := cfg.ListVaults()
		if len(vaults) != 0 {
			t.Errorf("expected 0 vaults, got %d", len(vaults))
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
markdown_style = "dark"
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
	if cfg.Editor != "code" {
		t.Errorf("expected editor 'code', got %q", cfg.Editor)
	}
	if len(cfg.Vaults) != 2 {
		t.Errorf("expected 2 vaults, got %d: %v", len(cfg.Vaults), cfg.Vaults)
	}
	if cfg.UI.MarkdownStyle != "dark" {
		t.Errorf("expected ui.markdown_style 'dark', got %q", cfg.UI.MarkdownStyle)
	}

	if err := SaveTo(configPath, cfg); err != nil {
		t.Fatalf("SaveTo() error = %v", err)
	}
	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if strings.Contains(string(saved), "state_file") {
		t.Fatalf("saved config retained removed state_file key:\n%s", saved)
	}
}

func TestLoadFromMigratesRemovedSinglePathToVaultRegistry(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(`vault = "/path/to/notes"`+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.DefaultVault != "default" || cfg.Vaults["default"] != "/path/to/notes" {
		t.Fatalf("migrated config = %#v, want default registry entry", cfg)
	}

	if err := SaveTo(configPath, cfg); err != nil {
		t.Fatalf("SaveTo() error = %v", err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if strings.Contains(string(content), "\nvault = ") || strings.HasPrefix(string(content), "vault = ") {
		t.Fatalf("saved config retained removed key:\n%s", content)
	}
	if !strings.Contains(string(content), `default_vault = "default"`) ||
		!strings.Contains(string(content), `default = "/path/to/notes"`) {
		t.Fatalf("saved config did not persist canonical registry:\n%s", content)
	}
}

func TestLoadFromIgnoresRemovedSinglePathWhenRegistryExists(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `vault = "/path/to/old"
default_vault = "work"

[vaults]
work = "/path/to/work"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.DefaultVault != "work" || len(cfg.Vaults) != 1 || cfg.Vaults["work"] != "/path/to/work" {
		t.Fatalf("loaded config = %#v, want existing registry unchanged", cfg)
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

func TestCreateDefaultAt(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nested", "config.toml")

	createdPath, err := CreateDefaultAt(configPath)
	if err != nil {
		t.Fatalf("CreateDefaultAt returned error: %v", err)
	}
	if createdPath != configPath {
		t.Fatalf("expected created path %q, got %q", configPath, createdPath)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read created config: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("expected non-empty default config content")
	}

	createdPath, err = CreateDefaultAt(configPath)
	if err != nil {
		t.Fatalf("CreateDefaultAt second call returned error: %v", err)
	}
	if createdPath != configPath {
		t.Fatalf("expected second created path %q, got %q", configPath, createdPath)
	}
}
