package config

import (
	"path/filepath"
	"testing"
)

func TestSaveToPersistsConfigFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")

	cfg := &Config{
		DefaultVault: "work",
		StateFile:    "state.toml",
		Editor:       "code",
		EditorMode:   "gui",
		Vaults: map[string]string{
			"work": "/tmp/work-vault",
		},
		UI: UIConfig{
			Accent:    "39",
			CodeTheme: "dracula",
		},
	}

	if err := SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo returned error: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}

	if loaded.StateFile != "state.toml" {
		t.Fatalf("expected state_file=state.toml, got %q", loaded.StateFile)
	}
	if loaded.Editor != "code" {
		t.Fatalf("expected editor=code, got %q", loaded.Editor)
	}
	if loaded.EditorMode != "gui" {
		t.Fatalf("expected editor_mode=gui, got %q", loaded.EditorMode)
	}
	if loaded.UI.Accent != "39" {
		t.Fatalf("expected ui.accent=39, got %q", loaded.UI.Accent)
	}
	if loaded.UI.CodeTheme != "dracula" {
		t.Fatalf("expected ui.code_theme=dracula, got %q", loaded.UI.CodeTheme)
	}
}
