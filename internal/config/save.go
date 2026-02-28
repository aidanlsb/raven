package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/aidanlsb/raven/internal/atomicfile"
)

type persistedConfig struct {
	DefaultVault *string              `toml:"default_vault,omitempty"`
	StateFile    *string              `toml:"state_file,omitempty"`
	Vault        *string              `toml:"vault,omitempty"`
	Vaults       map[string]string    `toml:"vaults,omitempty"`
	Editor       *string              `toml:"editor,omitempty"`
	EditorMode   *string              `toml:"editor_mode,omitempty"`
	UI           *persistedUISettings `toml:"ui,omitempty"`
}

type persistedUISettings struct {
	Accent    *string `toml:"accent,omitempty"`
	CodeTheme *string `toml:"code_theme,omitempty"`
}

func nonEmptyPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// Save writes the global config to the default config path.
func Save(cfg *Config) error {
	return SaveTo(DefaultPath(), cfg)
}

// SaveTo writes the global config to a specific path atomically.
func SaveTo(path string, cfg *Config) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("config path is required")
	}
	if cfg == nil {
		cfg = &Config{}
	}

	out := persistedConfig{
		DefaultVault: nonEmptyPtr(cfg.DefaultVault),
		StateFile:    nonEmptyPtr(cfg.StateFile),
		Vault:        nonEmptyPtr(cfg.Vault),
		Editor:       nonEmptyPtr(cfg.Editor),
		EditorMode:   nonEmptyPtr(cfg.EditorMode),
	}
	if len(cfg.Vaults) > 0 {
		out.Vaults = cfg.Vaults
	}

	accent := nonEmptyPtr(cfg.UI.Accent)
	codeTheme := nonEmptyPtr(cfg.UI.CodeTheme)
	if accent != nil || codeTheme != nil {
		out.UI = &persistedUISettings{
			Accent:    accent,
			CodeTheme: codeTheme,
		}
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(out); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := atomicfile.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write config %s: %w", path, err)
	}

	return nil
}
