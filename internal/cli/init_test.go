package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
)

// TestRunInitFollowUpRegistersPinsAndActivatesVault exercises the manual-register
// fallback used when auto-registration did not already register the vault.
func TestRunInitFollowUpRegistersPinsAndActivatesVault(t *testing.T) {
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevIn := initPromptIn
	prevOut := initPromptOut
	prevShould := initShouldPrompt
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		initPromptIn = prevIn
		initPromptOut = prevOut
		initShouldPrompt = prevShould
	})

	configPath = configFile
	statePathFlag = stateFile
	initPromptIn = strings.NewReader("y\nnotes\ny\ny\n")
	initPromptOut = &bytes.Buffer{}
	initShouldPrompt = func() bool { return true }

	info := initPostInitInfo{
		Path:          vaultPath,
		SuggestedName: "notes",
	}
	runInitFollowUp(&info)

	ctx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{
		ConfigPathOverride: configFile,
		StatePathOverride:  stateFile,
	})
	if err != nil {
		t.Fatalf("load vault context: %v", err)
	}

	if got := ctx.Cfg.Vaults["notes"]; got != filepath.Clean(vaultPath) {
		t.Fatalf("vault path = %q, want %q", got, filepath.Clean(vaultPath))
	}
	if got := ctx.Cfg.DefaultVault; got != "notes" {
		t.Fatalf("default_vault = %q, want %q", got, "notes")
	}
	if got := ctx.State.ActiveVault; got != "notes" {
		t.Fatalf("active_vault = %q, want %q", got, "notes")
	}
	if !info.AlreadyRegistered || !info.IsDefault || !info.IsActive {
		t.Fatalf("follow-up info = %+v, want registered/default/active", info)
	}
}

func TestRunInitFollowUpNonFirstVaultPromptsOnlyForDefault(t *testing.T) {
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "notes")
	existingPath := filepath.Join(root, "existing")
	for _, p := range []string{vaultPath, existingPath} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}

	if err := config.SaveTo(configFile, &config.Config{
		DefaultVault: "existing",
		Vaults:       map[string]string{"existing": existingPath, "notes": vaultPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(stateFile, &config.State{ActiveVault: "notes"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevIn := initPromptIn
	prevOut := initPromptOut
	prevShould := initShouldPrompt
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		initPromptIn = prevIn
		initPromptOut = prevOut
		initShouldPrompt = prevShould
	})

	configPath = configFile
	statePathFlag = stateFile
	initPromptIn = strings.NewReader("y\n")
	out := &bytes.Buffer{}
	initPromptOut = out
	initShouldPrompt = func() bool { return true }

	info := initPostInitInfo{
		Path:               vaultPath,
		SuggestedName:      "notes",
		RegisteredName:     "notes",
		AlreadyRegistered:  true,
		Registered:         true,
		HasExistingDefault: true,
		IsActive:           true,
		Activated:          true,
		NeedsDefaultChoice: true,
	}
	runInitFollowUp(&info)

	ctx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{
		ConfigPathOverride: configFile,
		StatePathOverride:  stateFile,
	})
	if err != nil {
		t.Fatalf("load vault context: %v", err)
	}
	if ctx.Cfg.DefaultVault != "notes" {
		t.Fatalf("default_vault = %q, want %q", ctx.Cfg.DefaultVault, "notes")
	}
	if ctx.State.ActiveVault != "notes" {
		t.Fatalf("active_vault = %q, want %q", ctx.State.ActiveVault, "notes")
	}
	if !info.IsDefault || !info.IsActive {
		t.Fatalf("info after prompts = %+v, want default+active", info)
	}
	if strings.Contains(out.String(), "active vault") {
		t.Fatalf("unexpected activation prompt after automatic switch: %q", out.String())
	}
}

func TestRunInitFollowUpFirstVaultDoesNotPrompt(t *testing.T) {
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	prevConfig := configPath
	prevState := statePathFlag
	prevIn := initPromptIn
	prevOut := initPromptOut
	prevShould := initShouldPrompt
	t.Cleanup(func() {
		configPath = prevConfig
		statePathFlag = prevState
		initPromptIn = prevIn
		initPromptOut = prevOut
		initShouldPrompt = prevShould
	})

	out := &bytes.Buffer{}
	configPath = configFile
	statePathFlag = stateFile
	// Any prompt would consume this input and mutate state; a first vault must not prompt.
	initPromptIn = strings.NewReader("y\ny\ny\ny\n")
	initPromptOut = out
	initShouldPrompt = func() bool { return true }

	info := initPostInitInfo{
		Path:              vaultPath,
		SuggestedName:     "notes",
		RegisteredName:    "notes",
		AlreadyRegistered: true,
		Registered:        true,
		IsFirstVault:      true,
		IsDefault:         true,
		IsActive:          true,
	}
	runInitFollowUp(&info)

	if out.Len() != 0 {
		t.Fatalf("expected no interactive prompts for first vault, got output: %q", out.String())
	}
}

func TestFormatInitSuggestedPathNormalizesWindowsSeparators(t *testing.T) {
	got := formatInitSuggestedPath(`C:\Users\me\New Notes`)
	want := `"C:/Users/me/New Notes"`
	if got != want {
		t.Fatalf("formatInitSuggestedPath() = %q, want %q", got, want)
	}
}
