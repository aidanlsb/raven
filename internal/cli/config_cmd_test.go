package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
)

func resetConfigSetFlagsForTest() {
	resetStringFlag(configSetCmd, "editor")
	resetStringFlag(configSetCmd, "editor-mode")
	resetStringFlag(configSetCmd, "state-file")
	resetStringFlag(configSetCmd, "default-vault")
	resetStringFlag(configSetCmd, "ui-accent")
	resetStringFlag(configSetCmd, "ui-code-theme")
}

func resetConfigUnsetFlagsForTest() {
	resetBoolFlag(configUnsetCmd, "editor")
	resetBoolFlag(configUnsetCmd, "editor-mode")
	resetBoolFlag(configUnsetCmd, "state-file")
	resetBoolFlag(configUnsetCmd, "default-vault")
	resetBoolFlag(configUnsetCmd, "ui-accent")
	resetBoolFlag(configUnsetCmd, "ui-code-theme")
}

func resetStringFlag(cmd *cobra.Command, name string) {
	if err := cmd.Flags().Set(name, ""); err != nil {
		panic(err)
	}
	if f := cmd.Flags().Lookup(name); f != nil {
		f.Changed = false
	}
}

func resetBoolFlag(cmd *cobra.Command, name string) {
	if err := cmd.Flags().Set(name, "false"); err != nil {
		panic(err)
	}
	if f := cmd.Flags().Lookup(name); f != nil {
		f.Changed = false
	}
}

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
		resetConfigSetFlagsForTest()
	})

	configPath = cfgPath
	statePathFlag = ""
	jsonOutput = true
	resetConfigSetFlagsForTest()

	if err := configSetCmd.Flags().Set("editor", "code"); err != nil {
		t.Fatalf("set editor: %v", err)
	}
	if err := configSetCmd.Flags().Set("editor-mode", "terminal"); err != nil {
		t.Fatalf("set editor-mode: %v", err)
	}
	if err := configSetCmd.Flags().Set("default-vault", "work"); err != nil {
		t.Fatalf("set default-vault: %v", err)
	}
	if err := configSetCmd.Flags().Set("ui-accent", "39"); err != nil {
		t.Fatalf("set ui-accent: %v", err)
	}
	if err := configSetCmd.Flags().Set("ui-code-theme", "dracula"); err != nil {
		t.Fatalf("set ui-code-theme: %v", err)
	}

	if err := configSetCmd.RunE(configSetCmd, []string{}); err != nil {
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
	if cfg.DefaultVault != "work" {
		t.Fatalf("expected default_vault=work, got %q", cfg.DefaultVault)
	}
	if cfg.UI.Accent != "39" {
		t.Fatalf("expected ui.accent=39, got %q", cfg.UI.Accent)
	}
	if cfg.UI.CodeTheme != "dracula" {
		t.Fatalf("expected ui.code_theme=dracula, got %q", cfg.UI.CodeTheme)
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
accent = "39"
code_theme = "dracula"
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
		resetConfigUnsetFlagsForTest()
	})

	configPath = cfgPath
	statePathFlag = ""
	jsonOutput = true
	resetConfigUnsetFlagsForTest()

	if err := configUnsetCmd.Flags().Set("editor", "true"); err != nil {
		t.Fatalf("set editor: %v", err)
	}
	if err := configUnsetCmd.Flags().Set("editor-mode", "true"); err != nil {
		t.Fatalf("set editor-mode: %v", err)
	}
	if err := configUnsetCmd.Flags().Set("default-vault", "true"); err != nil {
		t.Fatalf("set default-vault: %v", err)
	}
	if err := configUnsetCmd.Flags().Set("ui-accent", "true"); err != nil {
		t.Fatalf("set ui-accent: %v", err)
	}
	if err := configUnsetCmd.Flags().Set("ui-code-theme", "true"); err != nil {
		t.Fatalf("set ui-code-theme: %v", err)
	}

	if err := configUnsetCmd.RunE(configUnsetCmd, []string{}); err != nil {
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
	if cfg.UI.Accent != "" {
		t.Fatalf("expected ui.accent to be cleared, got %q", cfg.UI.Accent)
	}
	if cfg.UI.CodeTheme != "" {
		t.Fatalf("expected ui.code_theme to be cleared, got %q", cfg.UI.CodeTheme)
	}
}

func TestConfigSetRejectsUnknownDefaultVault(t *testing.T) {
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
		resetConfigSetFlagsForTest()
	})

	configPath = cfgPath
	statePathFlag = ""
	jsonOutput = false
	resetConfigSetFlagsForTest()

	if err := configSetCmd.Flags().Set("default-vault", "missing"); err != nil {
		t.Fatalf("set default-vault: %v", err)
	}

	err := configSetCmd.RunE(configSetCmd, []string{})
	if err == nil {
		t.Fatalf("expected error for unknown default vault")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unknown vault error, got %v", err)
	}
}
