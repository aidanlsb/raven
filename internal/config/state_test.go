package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStatePath(t *testing.T) {
	configPath := "/tmp/raven/config.toml"

	t.Run("explicit state path wins", func(t *testing.T) {
		got := ResolveStatePath("/tmp/custom/state.toml", configPath, &Config{
			StateFile: "state-from-config.toml",
		})
		if got != "/tmp/custom/state.toml" {
			t.Fatalf("expected explicit state path, got %q", got)
		}
	})

	t.Run("config state_file absolute", func(t *testing.T) {
		got := ResolveStatePath("", configPath, &Config{
			StateFile: "/var/tmp/raven-state.toml",
		})
		if got != "/var/tmp/raven-state.toml" {
			t.Fatalf("expected absolute state path, got %q", got)
		}
	})

	t.Run("config state_file relative to config dir", func(t *testing.T) {
		got := ResolveStatePath("", "/Users/me/.config/raven/config.toml", &Config{
			StateFile: "runtime/state.toml",
		})
		want := "/Users/me/.config/raven/runtime/state.toml"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("fallback sibling state.toml", func(t *testing.T) {
		got := ResolveStatePath("", "/Users/me/.config/raven/config.toml", &Config{})
		want := "/Users/me/.config/raven/state.toml"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})
}

func TestLoadStateMissingReturnsDefault(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.toml")

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Version != StateVersion {
		t.Fatalf("expected version %d, got %d", StateVersion, state.Version)
	}
	if state.ActiveVault != "" {
		t.Fatalf("expected empty active vault, got %q", state.ActiveVault)
	}
}

func TestSaveStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.toml")

	err := SaveState(path, &State{
		ActiveVault: "work",
	})
	if err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loaded.Version != StateVersion {
		t.Fatalf("expected version %d, got %d", StateVersion, loaded.Version)
	}
	if loaded.ActiveVault != "work" {
		t.Fatalf("expected active_vault=work, got %q", loaded.ActiveVault)
	}
}

func TestSaveToWritesConfiguredFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")

	err := SaveTo(path, &Config{
		DefaultVault: "work",
		StateFile:    "state.toml",
		Vaults: map[string]string{
			"work": "/vault/work",
		},
	})
	if err != nil {
		t.Fatalf("save config: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `default_vault = "work"`) {
		t.Fatalf("expected default_vault in output, got:\n%s", content)
	}
	if !strings.Contains(content, `state_file = "state.toml"`) {
		t.Fatalf("expected state_file in output, got:\n%s", content)
	}
	if !strings.Contains(content, "[vaults]") {
		t.Fatalf("expected vaults table in output, got:\n%s", content)
	}
}
