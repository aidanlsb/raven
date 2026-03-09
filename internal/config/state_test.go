package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStatePath(t *testing.T) {
	configPath := filepath.FromSlash("/tmp/raven/config.toml")

	t.Run("explicit state path wins", func(t *testing.T) {
		explicit := filepath.FromSlash("/tmp/custom/state.toml")
		got := ResolveStatePath(explicit, configPath, &Config{
			StateFile: "state-from-config.toml",
		})
		if got != explicit {
			t.Fatalf("expected explicit state path, got %q", got)
		}
	})

	t.Run("config state_file absolute", func(t *testing.T) {
		got := ResolveStatePath("", configPath, &Config{
			StateFile: "/var/tmp/raven-state.toml",
		})
		want := filepath.Clean(filepath.FromSlash("/var/tmp/raven-state.toml"))
		if got != want {
			t.Fatalf("expected absolute state path, got %q", got)
		}
	})

	t.Run("config state_file relative to config dir", func(t *testing.T) {
		cfgPath := filepath.FromSlash("/Users/me/.config/raven/config.toml")
		got := ResolveStatePath("", cfgPath, &Config{
			StateFile: "runtime/state.toml",
		})
		want := filepath.Join(filepath.Dir(cfgPath), "runtime", "state.toml")
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("fallback sibling state.toml", func(t *testing.T) {
		cfgPath := filepath.FromSlash("/Users/me/.config/raven/config.toml")
		got := ResolveStatePath("", cfgPath, &Config{})
		want := filepath.Join(filepath.Dir(cfgPath), "state.toml")
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
	if state.ActiveKeep != "" {
		t.Fatalf("expected empty active keep, got %q", state.ActiveKeep)
	}
}

func TestSaveStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.toml")

	err := SaveState(path, &State{
		ActiveKeep: "work",
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
	if loaded.ActiveKeep != "work" {
		t.Fatalf("expected active_keep=work, got %q", loaded.ActiveKeep)
	}
}

func TestSaveToWritesConfiguredFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")

	err := SaveTo(path, &Config{
		DefaultKeep: "work",
		StateFile:   "state.toml",
		Keeps: map[string]string{
			"work": "/keep/work",
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
	if !strings.Contains(content, `default_keep = "work"`) {
		t.Fatalf("expected default_keep in output, got:\n%s", content)
	}
	if !strings.Contains(content, `state_file = "state.toml"`) {
		t.Fatalf("expected state_file in output, got:\n%s", content)
	}
	if !strings.Contains(content, "[keeps]") {
		t.Fatalf("expected keeps table in output, got:\n%s", content)
	}
}
