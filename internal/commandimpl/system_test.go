package commandimpl

import (
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestBuildInitPostInitDataSuggestsRegistration(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "My Notes")

	data := buildInitPostInitData(vaultPath, configPath, statePath)

	if got := data["suggested_name"]; got != "my-notes" {
		t.Fatalf("suggested_name = %#v, want %q", got, "my-notes")
	}
	if got := data["already_registered"]; got != false {
		t.Fatalf("already_registered = %#v, want false", got)
	}

	commands, ok := data["commands"].(map[string]interface{})
	if !ok {
		t.Fatalf("commands = %#v, want map", data["commands"])
	}
	register, _ := commands["register"].(string)
	wantPath := filepath.Clean(vaultPath)
	if got, want := register, `rvn vault add my-notes "`+wantPath+`" --json`; got != want {
		t.Fatalf("register command = %q, want %q", got, want)
	}

	nextSteps, ok := data["next_steps"].([]string)
	if !ok {
		rawSteps, ok := data["next_steps"].([]interface{})
		if !ok {
			t.Fatalf("next_steps = %#v, want slice", data["next_steps"])
		}
		nextSteps = make([]string, 0, len(rawSteps))
		for _, step := range rawSteps {
			if text, ok := step.(string); ok {
				nextSteps = append(nextSteps, text)
			}
		}
	}
	if len(nextSteps) == 0 {
		t.Fatal("expected next_steps guidance")
	}
}

func TestBuildInitPostInitDataReflectsRegisteredVault(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "notes")

	cfg := &config.Config{
		DefaultVault: "notes",
		Vaults: map[string]string{
			"notes": vaultPath,
		},
	}
	if err := config.SaveTo(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{ActiveVault: "notes"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	data := buildInitPostInitData(vaultPath, configPath, statePath)

	if got := data["registered_name"]; got != "notes" {
		t.Fatalf("registered_name = %#v, want %q", got, "notes")
	}
	if got := data["already_registered"]; got != true {
		t.Fatalf("already_registered = %#v, want true", got)
	}
	if got := data["is_default"]; got != true {
		t.Fatalf("is_default = %#v, want true", got)
	}
	if got := data["is_active"]; got != true {
		t.Fatalf("is_active = %#v, want true", got)
	}

	switch steps := data["next_steps"].(type) {
	case []string:
		if len(steps) != 0 {
			t.Fatalf("next_steps len = %d, want 0", len(steps))
		}
	case []interface{}:
		if len(steps) != 0 {
			t.Fatalf("next_steps len = %d, want 0", len(steps))
		}
	default:
		t.Fatalf("next_steps = %#v, want empty slice", data["next_steps"])
	}
}
