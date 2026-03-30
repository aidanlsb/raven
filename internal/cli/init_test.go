package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/configsvc"
)

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
