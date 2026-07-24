package commandimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
)

func TestHandleConfigShowReturnsEffectiveDefaults(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("# empty config\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result := HandleConfigShow(context.Background(), commandexec.Request{ConfigPath: configPath})
	if !result.OK {
		t.Fatalf("HandleConfigShow() failed: %+v", result.Error)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want map", result.Data)
	}
	if data["editor_mode"] != "auto" {
		t.Fatalf("editor_mode = %#v, want auto", data["editor_mode"])
	}
	uiData, ok := data["ui"].(map[string]interface{})
	if !ok || uiData["markdown_style"] != "auto" || len(uiData) != 1 {
		t.Fatalf("ui = %#v, want only markdown_style=auto", data["ui"])
	}
	if data["state_path"] != filepath.Join(filepath.Dir(configPath), "state.toml") {
		t.Fatalf("state_path = %#v, want resolved sibling state.toml", data["state_path"])
	}
	if _, exists := data["vault"]; exists {
		t.Fatalf("config show exposed removed vault key: %#v", data)
	}
	if _, exists := data["state_file"]; exists {
		t.Fatalf("config show exposed removed state_file key: %#v", data)
	}
}

func TestHandleVaultCurrentIncludesMissingActiveVaultName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")

	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "work",
		Vaults: map[string]string{
			"work": "/vault/work",
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:     config.StateVersion,
		ActiveVault: "personal",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	result := HandleVaultCurrent(context.Background(), commandexec.Request{
		ConfigPath: configPath,
		StatePath:  statePath,
	})
	if !result.OK {
		t.Fatalf("HandleVaultCurrent() failed: %+v", result.Error)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want map", result.Data)
	}
	if got := data["active_missing"]; got != true {
		t.Fatalf("active_missing = %#v, want true", got)
	}
	if got := data["active_vault"]; got != "personal" {
		t.Fatalf("active_vault = %#v, want %q", got, "personal")
	}
	if got := data["source"]; got != "default_vault_fallback" {
		t.Fatalf("source = %#v, want %q", got, "default_vault_fallback")
	}
}
