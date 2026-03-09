package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestKeepRows(t *testing.T) {
	cfg := &config.Config{
		DefaultKeep: "work",
		Keeps: map[string]string{
			"work":     "/keep/work",
			"personal": "/keep/personal",
		},
	}
	state := &config.State{ActiveKeep: "personal"}

	rows, defaultName, activeName, activeMissing := keepRows(cfg, state)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if defaultName != "work" {
		t.Fatalf("expected default_name=work, got %q", defaultName)
	}
	if activeName != "personal" {
		t.Fatalf("expected active_name=personal, got %q", activeName)
	}
	if activeMissing {
		t.Fatalf("expected active_missing=false")
	}
}

func TestResolveCurrentKeep(t *testing.T) {
	t.Run("prefers active keep", func(t *testing.T) {
		cfg := &config.Config{
			DefaultKeep: "work",
			Keeps: map[string]string{
				"work":     "/keep/work",
				"personal": "/keep/personal",
			},
		}
		state := &config.State{ActiveKeep: "personal"}

		got, err := resolveCurrentKeep(cfg, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "personal" {
			t.Fatalf("expected name=personal, got %q", got.Name)
		}
		if got.Path != "/keep/personal" {
			t.Fatalf("expected path=/keep/personal, got %q", got.Path)
		}
		if got.Source != "active_keep" {
			t.Fatalf("expected source=active_keep, got %q", got.Source)
		}
	})

	t.Run("falls back to default when active missing", func(t *testing.T) {
		cfg := &config.Config{
			DefaultKeep: "work",
			Keeps: map[string]string{
				"work": "/keep/work",
			},
		}
		state := &config.State{ActiveKeep: "personal"}

		got, err := resolveCurrentKeep(cfg, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "work" {
			t.Fatalf("expected name=work, got %q", got.Name)
		}
		if got.Source != "default_keep_fallback" {
			t.Fatalf("expected source=default_keep_fallback, got %q", got.Source)
		}
		if !got.ActiveMissing {
			t.Fatalf("expected active_missing=true")
		}
	})

	t.Run("errors when neither active nor default is available", func(t *testing.T) {
		cfg := &config.Config{}
		state := &config.State{ActiveKeep: "missing"}

		_, err := resolveCurrentKeep(cfg, state)
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestKeepUseCreatesStateFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")

	content := `default_keep = "work"

[keeps]
work = "/keep/work"
personal = "/keep/personal"
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
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("expected state file to not exist yet")
	}

	if err := keepUseCmd.RunE(keepUseCmd, []string{"personal"}); err != nil {
		t.Fatalf("keepUseCmd.RunE: %v", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveKeep != "personal" {
		t.Fatalf("expected active_keep=personal, got %q", state.ActiveKeep)
	}
}

func TestKeepPinUpdatesDefaultKeep(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")

	content := `default_keep = "work"

[keeps]
work = "/keep/work"
personal = "/keep/personal"
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
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true

	if err := keepPinCmd.RunE(keepPinCmd, []string{"personal"}); err != nil {
		t.Fatalf("keepPinCmd.RunE: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.DefaultKeep != "personal" {
		t.Fatalf("expected default_keep=personal, got %q", cfg.DefaultKeep)
	}
}

func TestKeepClearRemovesActiveKeep(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")

	cfgContent := `default_keep = "work"

[keeps]
work = "/keep/work"
personal = "/keep/personal"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stateContent := `version = 1
active_keep = "personal"
`
	if err := os.WriteFile(statePath, []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true

	if err := keepClearCmd.RunE(keepClearCmd, []string{}); err != nil {
		t.Fatalf("keepClearCmd.RunE: %v", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if strings.TrimSpace(state.ActiveKeep) != "" {
		t.Fatalf("expected empty active_keep, got %q", state.ActiveKeep)
	}
}

func TestKeepAddAddsAndPinsKeep(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	keepPath := filepath.Join(tmp, "keep-personal")

	if err := os.WriteFile(cfgPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(keepPath, 0o755); err != nil {
		t.Fatalf("create keep path: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevReplace := keepAddReplace
	prevPin := keepAddPin
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		keepAddReplace = prevReplace
		keepAddPin = prevPin
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true
	keepAddReplace = false
	keepAddPin = true

	if err := keepAddCmd.RunE(keepAddCmd, []string{"personal", keepPath}); err != nil {
		t.Fatalf("keepAddCmd.RunE: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := cfg.DefaultKeep; got != "personal" {
		t.Fatalf("expected default_keep=personal, got %q", got)
	}
	if got := cfg.Keeps["personal"]; got != keepPath {
		t.Fatalf("expected keep path %q, got %q", keepPath, got)
	}
}

func TestKeepAddRejectsDuplicateWithoutReplace(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	oldPath := filepath.Join(tmp, "keep-work")
	newPath := filepath.Join(tmp, "keep-work-new")

	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("create old path: %v", err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("create new path: %v", err)
	}

	if err := config.SaveTo(cfgPath, &config.Config{
		DefaultKeep: "work",
		Keeps: map[string]string{
			"work": oldPath,
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevReplace := keepAddReplace
	prevPin := keepAddPin
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		keepAddReplace = prevReplace
		keepAddPin = prevPin
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = false
	keepAddReplace = false
	keepAddPin = false

	err := keepAddCmd.RunE(keepAddCmd, []string{"work", newPath})
	if err == nil {
		t.Fatalf("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate-name error message, got %v", err)
	}

	cfg, loadErr := config.LoadFrom(cfgPath)
	if loadErr != nil {
		t.Fatalf("reload config: %v", loadErr)
	}
	if got := cfg.Keeps["work"]; got != oldPath {
		t.Fatalf("expected existing path to remain %q, got %q", oldPath, got)
	}
}

func TestKeepRemoveRequiresClearFlagsForDefaultAndActive(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	workPath := filepath.Join(tmp, "keep-work")
	personalPath := filepath.Join(tmp, "keep-personal")

	if err := os.MkdirAll(workPath, 0o755); err != nil {
		t.Fatalf("create work path: %v", err)
	}
	if err := os.MkdirAll(personalPath, 0o755); err != nil {
		t.Fatalf("create personal path: %v", err)
	}

	if err := config.SaveTo(cfgPath, &config.Config{
		DefaultKeep: "work",
		Keeps: map[string]string{
			"work":     workPath,
			"personal": personalPath,
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:    config.StateVersion,
		ActiveKeep: "work",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevClearDefault := keepRemoveClearDefault
	prevClearActive := keepRemoveClearActive
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		keepRemoveClearDefault = prevClearDefault
		keepRemoveClearActive = prevClearActive
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = false
	keepRemoveClearDefault = false
	keepRemoveClearActive = false

	err := keepRemoveCmd.RunE(keepRemoveCmd, []string{"work"})
	if err == nil {
		t.Fatalf("expected confirmation-required error")
	}
	if !strings.Contains(err.Error(), "default keep") {
		t.Fatalf("expected default keep error, got %v", err)
	}
}

func TestKeepRemoveClearsDefaultAndActive(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	workPath := filepath.Join(tmp, "keep-work")
	personalPath := filepath.Join(tmp, "keep-personal")

	if err := os.MkdirAll(workPath, 0o755); err != nil {
		t.Fatalf("create work path: %v", err)
	}
	if err := os.MkdirAll(personalPath, 0o755); err != nil {
		t.Fatalf("create personal path: %v", err)
	}

	if err := config.SaveTo(cfgPath, &config.Config{
		DefaultKeep: "work",
		Keeps: map[string]string{
			"work":     workPath,
			"personal": personalPath,
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:    config.StateVersion,
		ActiveKeep: "work",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevJSON := jsonOutput
	prevClearDefault := keepRemoveClearDefault
	prevClearActive := keepRemoveClearActive
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		jsonOutput = prevJSON
		keepRemoveClearDefault = prevClearDefault
		keepRemoveClearActive = prevClearActive
	})

	configPath = cfgPath
	statePathFlag = statePath
	jsonOutput = true
	keepRemoveClearDefault = true
	keepRemoveClearActive = true

	if err := keepRemoveCmd.RunE(keepRemoveCmd, []string{"work"}); err != nil {
		t.Fatalf("keepRemoveCmd.RunE: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := cfg.DefaultKeep; got != "" {
		t.Fatalf("expected default_keep to be cleared, got %q", got)
	}
	if _, ok := cfg.Keeps["work"]; ok {
		t.Fatalf("expected work keep to be removed")
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if strings.TrimSpace(state.ActiveKeep) != "" {
		t.Fatalf("expected active_keep to be cleared, got %q", state.ActiveKeep)
	}
}
