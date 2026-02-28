package config

import (
	"path/filepath"
	"testing"
)

func TestSaveToPersistsHooksPolicy(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")

	enabled := true
	cfg := &Config{
		DefaultVault: "work",
		Vaults: map[string]string{
			"work": "/tmp/work-vault",
		},
		Hooks: HooksPolicyConfig{
			DefaultEnabled: &enabled,
			Vaults: map[string]bool{
				"work": true,
				"home": false,
			},
		},
	}

	if err := SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo returned error: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}

	if loaded.Hooks.DefaultEnabled == nil || !*loaded.Hooks.DefaultEnabled {
		t.Fatalf("expected hooks.default_enabled=true, got %#v", loaded.Hooks.DefaultEnabled)
	}
	if !loaded.Hooks.Vaults["work"] {
		t.Fatal("expected hooks.vaults.work=true")
	}
	if loaded.Hooks.Vaults["home"] {
		t.Fatal("expected hooks.vaults.home=false")
	}
}
