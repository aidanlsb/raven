package commandimpl

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
)

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
