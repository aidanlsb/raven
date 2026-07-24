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
		Editor:       "code",
		EditorMode:   "gui",
		Vaults: map[string]string{
			"work": "/tmp/work-vault",
		},
		UI: UIConfig{
			MarkdownStyle: "dark",
		},
	}

	if err := SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo returned error: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}

	if loaded.Editor != "code" {
		t.Fatalf("expected editor=code, got %q", loaded.Editor)
	}
	if loaded.EditorMode != "gui" {
		t.Fatalf("expected editor_mode=gui, got %q", loaded.EditorMode)
	}
	if loaded.UI.MarkdownStyle != "dark" {
		t.Fatalf("expected ui.markdown_style=dark, got %q", loaded.UI.MarkdownStyle)
	}
}
