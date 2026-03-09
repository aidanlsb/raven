package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigGetKeepPath(t *testing.T) {
	t.Run("named keep", func(t *testing.T) {
		cfg := &Config{
			Keeps: map[string]string{
				"work":     "/path/to/work",
				"personal": "/path/to/personal",
			},
		}

		path, err := cfg.GetKeepPath("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/path/to/work" {
			t.Errorf("expected '/path/to/work', got %q", path)
		}
	})

	t.Run("default keep", func(t *testing.T) {
		cfg := &Config{
			DefaultKeep: "personal",
			Keeps: map[string]string{
				"work":     "/path/to/work",
				"personal": "/path/to/personal",
			},
		}

		path, err := cfg.GetKeepPath("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/path/to/personal" {
			t.Errorf("expected '/path/to/personal', got %q", path)
		}
	})

	t.Run("legacy keep fallback", func(t *testing.T) {
		cfg := &Config{
			Keep: "/legacy/keep/path",
		}

		path, err := cfg.GetKeepPath("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/legacy/keep/path" {
			t.Errorf("expected '/legacy/keep/path', got %q", path)
		}
	})

	t.Run("legacy keep supports default alias", func(t *testing.T) {
		cfg := &Config{
			Keep: "/legacy/keep/path",
		}

		path, err := cfg.GetKeepPath("default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/legacy/keep/path" {
			t.Errorf("expected '/legacy/keep/path', got %q", path)
		}
	})

	t.Run("keep not found", func(t *testing.T) {
		cfg := &Config{
			Keeps: map[string]string{
				"work": "/path/to/work",
			},
		}

		_, err := cfg.GetKeepPath("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent keep")
		}
	})

	t.Run("no default configured", func(t *testing.T) {
		cfg := &Config{}

		_, err := cfg.GetKeepPath("")
		if err == nil {
			t.Error("expected error when no default configured")
		}
	})
}

func TestConfigGetDefaultKeepPath(t *testing.T) {
	cfg := &Config{
		DefaultKeep: "main",
		Keeps: map[string]string{
			"main": "/path/to/main",
		},
	}

	path, err := cfg.GetDefaultKeepPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/path/to/main" {
		t.Errorf("expected '/path/to/main', got %q", path)
	}
}

func TestConfigListKeeps(t *testing.T) {
	t.Run("named keeps", func(t *testing.T) {
		cfg := &Config{
			Keeps: map[string]string{
				"work":     "/path/to/work",
				"personal": "/path/to/personal",
			},
		}

		keeps := cfg.ListKeeps()
		if len(keeps) != 2 {
			t.Errorf("expected 2 keeps, got %d", len(keeps))
		}
		if keeps["work"] != "/path/to/work" {
			t.Error("missing 'work' keep")
		}
		if keeps["personal"] != "/path/to/personal" {
			t.Error("missing 'personal' keep")
		}
	})

	t.Run("legacy keep as default", func(t *testing.T) {
		cfg := &Config{
			Keep: "/legacy/path",
		}

		keeps := cfg.ListKeeps()
		if len(keeps) != 1 {
			t.Errorf("expected 1 keep, got %d", len(keeps))
		}
		if keeps["default"] != "/legacy/path" {
			t.Error("expected legacy keep as 'default'")
		}
	})

	t.Run("empty config", func(t *testing.T) {
		cfg := &Config{}

		keeps := cfg.ListKeeps()
		if len(keeps) != 0 {
			t.Errorf("expected 0 keeps, got %d", len(keeps))
		}
	})

	t.Run("named keeps take precedence over legacy", func(t *testing.T) {
		cfg := &Config{
			Keep: "/legacy/path",
			Keeps: map[string]string{
				"main": "/named/path",
			},
		}

		keeps := cfg.ListKeeps()
		if len(keeps) != 1 {
			t.Errorf("expected 1 keep, got %d", len(keeps))
		}
		if _, ok := keeps["default"]; ok {
			t.Error("legacy keep should not appear when named keeps exist")
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
	// editor needs to come before [keeps] or after keep definitions.
	content := `default_keep = "work"
state_file = "state.toml"
editor = "code"

[keeps]
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

	if cfg.DefaultKeep != "work" {
		t.Errorf("expected default_keep 'work', got %q", cfg.DefaultKeep)
	}
	if cfg.StateFile != "state.toml" {
		t.Errorf("expected state_file 'state.toml', got %q", cfg.StateFile)
	}
	if cfg.Editor != "code" {
		t.Errorf("expected editor 'code', got %q", cfg.Editor)
	}
	if len(cfg.Keeps) != 2 {
		t.Errorf("expected 2 keeps, got %d: %v", len(cfg.Keeps), cfg.Keeps)
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
