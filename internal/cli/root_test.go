package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/ui"
)

func TestPersistentPreRunEAppliesMarkdownStyleForNonVaultCommands(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.toml")
	content := []byte("[ui]\nmarkdown_style = \"dark\"\n")
	if err := os.WriteFile(configFile, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevConfigPath := configPath
	prevStatePathFlag := statePathFlag
	prevVaultName := vaultName
	prevVaultPathFlag := vaultPathFlag
	prevResolvedVaultPath := resolvedVaultPath
	prevResolvedConfigPath := resolvedConfigPath
	prevResolvedStatePath := resolvedStatePath
	prevCfg := cfg
	prevAccent := ui.Accent
	prevBold := ui.Bold
	prevMuted := ui.Muted
	prevSyntax := ui.Syntax
	prevSyntaxSubtle := ui.SyntaxSubtle
	t.Cleanup(func() {
		configPath = prevConfigPath
		statePathFlag = prevStatePathFlag
		vaultName = prevVaultName
		vaultPathFlag = prevVaultPathFlag
		resolvedVaultPath = prevResolvedVaultPath
		resolvedConfigPath = prevResolvedConfigPath
		resolvedStatePath = prevResolvedStatePath
		cfg = prevCfg
		ui.Accent = prevAccent
		ui.Bold = prevBold
		ui.Muted = prevMuted
		ui.Syntax = prevSyntax
		ui.SyntaxSubtle = prevSyntaxSubtle
		ui.ConfigureStyles()
		ui.ConfigureMarkdownStyle("")
	})

	configPath = configFile
	statePathFlag = ""
	vaultName = ""
	vaultPathFlag = ""
	resolvedVaultPath = ""
	resolvedConfigPath = ""
	resolvedStatePath = ""
	cfg = &config.Config{}

	if err := rootCmd.PersistentPreRunE(configCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}

	if _, err := ui.RenderMarkdown("# configured", 80); err != nil {
		t.Fatalf("RenderMarkdown() with configured style error = %v", err)
	}
}
