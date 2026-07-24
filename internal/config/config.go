// Package config handles global Raven configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the global Raven configuration.
type Config struct {
	// DefaultVault is the name of the default vault (from Vaults map).
	DefaultVault string `toml:"default_vault"`

	// Vaults is a map of vault names to paths.
	Vaults map[string]string `toml:"vaults"`

	// Editor is the editor to use for opening files (defaults to $EDITOR).
	Editor string `toml:"editor"`

	// EditorMode controls how the editor is launched: auto, terminal, or gui.
	EditorMode string `toml:"editor_mode"`

	// UI controls optional Markdown rendering preferences.
	UI UIConfig `toml:"ui"`
}

// UIConfig represents optional CLI theming preferences.
type UIConfig struct {
	// MarkdownStyle sets the full Glamour markdown style.
	// Empty or "auto" uses Glamour's automatic light/dark style.
	MarkdownStyle string `toml:"markdown_style"`
}

// GetVaultPath returns the path for a named vault.
// If name is empty, returns the default vault path.
func (c *Config) GetVaultPath(name string) (string, error) {
	// If no name specified, use default
	if name == "" {
		name = c.DefaultVault
	}

	if name == "" {
		return "", fmt.Errorf("no default vault configured")
	}

	// Look up named vault
	if c.Vaults != nil {
		if path, ok := c.Vaults[name]; ok {
			return path, nil
		}
	}

	return "", fmt.Errorf("vault '%s' not found in config", name)
}

// GetDefaultVaultPath returns the default vault path.
func (c *Config) GetDefaultVaultPath() (string, error) {
	return c.GetVaultPath("")
}

// ListVaults returns all configured vaults with their paths.
func (c *Config) ListVaults() map[string]string {
	result := make(map[string]string)

	for name, path := range c.Vaults {
		result[name] = path
	}
	return result
}

// Load loads the configuration from the default location.
// Returns a default config if the file doesn't exist.
func Load() (*Config, error) {
	configPath := DefaultPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{}, nil
	}

	return LoadFrom(configPath)
}

// LoadFrom loads the configuration from a specific path.
func LoadFrom(path string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	migrateLegacyVault(path, &config)
	return &config, nil
}

// migrateLegacyVault folds the removed top-level single-path setting into the
// canonical named vault registry. The compatibility key is never retained on
// Config and disappears the next time the config is saved.
func migrateLegacyVault(path string, cfg *Config) {
	if cfg == nil || len(cfg.Vaults) != 0 {
		return
	}

	var raw map[string]interface{}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return
	}
	legacyPath, ok := raw["vault"].(string)
	legacyPath = strings.TrimSpace(legacyPath)
	if !ok || legacyPath == "" {
		return
	}

	cfg.Vaults = map[string]string{"default": legacyPath}
	cfg.DefaultVault = "default"
}

// DefaultPath returns the default config file path.
// Checks ~/.config/raven/config.toml first (XDG style),
// then falls back to OS-specific location.
func DefaultPath() string {
	// Prefer XDG-style ~/.config/raven/config.toml
	if home, err := os.UserHomeDir(); err == nil {
		xdgPath := filepath.Join(home, ".config", "raven", "config.toml")
		if _, err := os.Stat(xdgPath); err == nil {
			return xdgPath
		}
	}

	// Fall back to XDG config dir or OS-specific location
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "raven", "config.toml")
	}

	// Last resort fallback
	return filepath.Join(".", "config.toml")
}

// XDGPath returns the XDG-style config path (~/.config/raven/config.toml).
func XDGPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "raven", "config.toml"), nil
}

// CreateDefaultAt creates a default config file at a specific path if it doesn't exist.
func CreateDefaultAt(path string) (string, error) {
	configPath := strings.TrimSpace(path)
	if configPath == "" {
		return "", fmt.Errorf("config path is required")
	}

	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil // Already exists
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	defaultConfig := `# Raven Configuration
# See: https://github.com/aidanlsb/raven

# Default vault name (must exist in [vaults] below).
# Set this with: rvn vault pin <name>
# default_vault = "personal"

# Named vaults
# [vaults]
# personal = "/path/to/your/notes"
# work = "/path/to/work/notes"

# Editor for opening files (defaults to $EDITOR)
# editor = "code"
#
# How to launch the editor:
#   auto     - detect common terminal editors
#   terminal - always run in the foreground with TTY attached
#   gui      - always run in the background (non-blocking)
# editor_mode = "auto"
#
# Optional Glamour markdown style. Use "auto", a stock built-in style name
# (such as "dark", "light", "notty", or "ascii"), or a style JSON path.
# [ui]
# markdown_style = "auto"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return configPath, nil
}

// GetEditor returns the editor to use, falling back to $EDITOR.
func (c *Config) GetEditor() string {
	if c.Editor != "" {
		return c.Editor
	}
	return os.Getenv("EDITOR")
}
