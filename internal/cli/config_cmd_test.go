package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestConfigInitCreatesConfigFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "nested", "config.toml")

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
	})

	configPath = cfgPath
	statePathFlag = ""
	jsonOutput = true

	if err := configInitCmd.RunE(configInitCmd, []string{}); err != nil {
		t.Fatalf("configInitCmd.RunE returned error: %v", err)
	}

	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read created config: %v", err)
	}
	if !strings.Contains(string(content), "# Raven Configuration") {
		t.Fatalf("expected default config header in file, got:\n%s", string(content))
	}
}

func TestConfigSetUpdatesFields(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	content := `[vaults]
work = "/vault/work"
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
	statePathFlag = ""
	jsonOutput = true

	if err := configSetCmd.RunE(configSetCmd, []string{
		"editor=code",
		"editor_mode=terminal",
		"ui.markdown_style=dark",
	}); err != nil {
		t.Fatalf("configSetCmd.RunE returned error: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.Editor != "code" {
		t.Fatalf("expected editor=code, got %q", cfg.Editor)
	}
	if cfg.EditorMode != "terminal" {
		t.Fatalf("expected editor_mode=terminal, got %q", cfg.EditorMode)
	}
	if cfg.UI.MarkdownStyle != "dark" {
		t.Fatalf("expected ui.markdown_style=dark, got %q", cfg.UI.MarkdownStyle)
	}
}

func TestConfigUnsetClearsFields(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	content := `default_vault = "work"
editor = "code"
editor_mode = "gui"

[vaults]
work = "/vault/work"

[ui]
markdown_style = "dark"
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
	statePathFlag = ""
	jsonOutput = true

	if err := configUnsetCmd.RunE(configUnsetCmd, []string{
		"editor",
		"editor_mode",
		"default_vault",
		"ui.markdown_style",
	}); err != nil {
		t.Fatalf("configUnsetCmd.RunE returned error: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.Editor != "" {
		t.Fatalf("expected editor to be cleared, got %q", cfg.Editor)
	}
	if cfg.EditorMode != "" {
		t.Fatalf("expected editor_mode to be cleared, got %q", cfg.EditorMode)
	}
	if cfg.DefaultVault != "" {
		t.Fatalf("expected default_vault to be cleared, got %q", cfg.DefaultVault)
	}
	if cfg.UI.MarkdownStyle != "" {
		t.Fatalf("expected ui.markdown_style to be cleared, got %q", cfg.UI.MarkdownStyle)
	}
}

func TestConfigSetRejectsDefaultVaultWithPinGuidance(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(""), 0o644); err != nil {
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
	statePathFlag = ""
	jsonOutput = false

	err := configSetCmd.RunE(configSetCmd, []string{"default_vault=missing"})
	if err == nil {
		t.Fatalf("expected error for default_vault set")
	}
	if !strings.Contains(err.Error(), "rvn vault pin") {
		t.Fatalf("expected vault pin guidance, got %v", err)
	}
}

func TestConfigCommandsDoNotExposeLegacyFieldFlags(t *testing.T) {
	for _, name := range []string{"editor", "editor-mode", "state-file", "default-vault", "ui-markdown-style"} {
		if flag := configSetCmd.Flags().Lookup(name); flag != nil {
			t.Fatalf("config set unexpectedly exposes --%s", name)
		}
		if flag := configUnsetCmd.Flags().Lookup(name); flag != nil {
			t.Fatalf("config unset unexpectedly exposes --%s", name)
		}
	}
}
