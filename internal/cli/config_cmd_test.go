package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func resetConfigSetFlagsForTest() {
	configSetEditor = ""
	configSetEditorMode = ""
	configSetStateFile = ""
	configSetDefaultKeep = ""
	configSetUIAccent = ""
	configSetUICodeTheme = ""

	if f := configSetCmd.Flags().Lookup("editor"); f != nil {
		f.Changed = false
	}
	if f := configSetCmd.Flags().Lookup("editor-mode"); f != nil {
		f.Changed = false
	}
	if f := configSetCmd.Flags().Lookup("state-file"); f != nil {
		f.Changed = false
	}
	if f := configSetCmd.Flags().Lookup("default-keep"); f != nil {
		f.Changed = false
	}
	if f := configSetCmd.Flags().Lookup("ui-accent"); f != nil {
		f.Changed = false
	}
	if f := configSetCmd.Flags().Lookup("ui-code-theme"); f != nil {
		f.Changed = false
	}
}

func resetConfigUnsetFlagsForTest() {
	configUnsetEditor = false
	configUnsetEditorMode = false
	configUnsetStateFile = false
	configUnsetDefaultKeep = false
	configUnsetUIAccent = false
	configUnsetUICodeTheme = false
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

	content := `[keeps]
work = "/keep/work"
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

	configSetEditor = "code"
	configSetEditorMode = "terminal"
	configSetDefaultKeep = "work"
	configSetUIAccent = "39"
	configSetUICodeTheme = "dracula"

	configSetCmd.Flags().Lookup("editor").Changed = true
	configSetCmd.Flags().Lookup("editor-mode").Changed = true
	configSetCmd.Flags().Lookup("default-keep").Changed = true
	configSetCmd.Flags().Lookup("ui-accent").Changed = true
	configSetCmd.Flags().Lookup("ui-code-theme").Changed = true

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
	if cfg.DefaultKeep != "work" {
		t.Fatalf("expected default_keep=work, got %q", cfg.DefaultKeep)
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

	content := `default_keep = "work"
editor = "code"
editor_mode = "gui"

[keeps]
work = "/keep/work"

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

	configUnsetEditor = true
	configUnsetEditorMode = true
	configUnsetDefaultKeep = true
	configUnsetUIAccent = true
	configUnsetUICodeTheme = true

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
	if cfg.DefaultKeep != "" {
		t.Fatalf("expected default_keep to be cleared, got %q", cfg.DefaultKeep)
	}
	if cfg.UI.Accent != "" {
		t.Fatalf("expected ui.accent to be cleared, got %q", cfg.UI.Accent)
	}
	if cfg.UI.CodeTheme != "" {
		t.Fatalf("expected ui.code_theme to be cleared, got %q", cfg.UI.CodeTheme)
	}
}

func TestConfigSetRejectsUnknownDefaultKeep(t *testing.T) {
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

	configSetDefaultKeep = "missing"
	configSetCmd.Flags().Lookup("default-keep").Changed = true

	err := configSetCmd.RunE(configSetCmd, []string{})
	if err == nil {
		t.Fatalf("expected error for unknown default keep")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unknown keep error, got %v", err)
	}
}
