// Package config handles global Raven configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the global Raven configuration.
type Config struct {
	// DefaultVault is the name of the default vault (from Vaults map).
	DefaultVault string `toml:"default_vault"`

	// Vault is the legacy single vault path (for backwards compatibility).
	// Deprecated: Use DefaultVault + Vaults instead.
	Vault string `toml:"vault"`

	// Vaults is a map of vault names to paths.
	Vaults map[string]string `toml:"vaults"`

	// Editor is the editor to use for opening files (defaults to $EDITOR).
	Editor string `toml:"editor"`

	// EditorMode controls how the editor is launched: auto, terminal, or gui.
	EditorMode string `toml:"editor_mode"`

	// UI controls optional CLI theming preferences.
	UI UIConfig `toml:"ui"`
}

// UIConfig represents optional CLI theming preferences.
type UIConfig struct {
	// Accent is an optional accent color for CLI output and markdown rendering.
	// Supported values are ANSI color codes ("0" to "255") or hex colors ("#RRGGBB").
	Accent string `toml:"accent"`

	// CodeTheme sets the Glamour/Chroma theme used for rendered markdown code blocks.
	// Example values: "monokai", "dracula", "github", "nord".
	CodeTheme string `toml:"code_theme"`
}

// GetVaultPath returns the path for a named vault.
// If name is empty, returns the default vault path.
func (c *Config) GetVaultPath(name string) (string, error) {
	// If no name specified, use default
	if name == "" {
		name = c.DefaultVault
	}

	// If still no name but legacy vault is set, use that
	if name == "" && c.Vault != "" {
		return c.Vault, nil
	}

	// Look up named vault
	if c.Vaults != nil {
		if path, ok := c.Vaults[name]; ok {
			return path, nil
		}
	}

	// If name matches default and legacy vault exists
	if name == "" && c.Vault != "" {
		return c.Vault, nil
	}

	if name == "" {
		return "", fmt.Errorf("no default vault configured")
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

	// Add named vaults
	for name, path := range c.Vaults {
		result[name] = path
	}

	// If legacy vault and no named vaults, add as "default"
	if len(result) == 0 && c.Vault != "" {
		result["default"] = c.Vault
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
	return &config, nil
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

// CreateDefault creates a default config file if it doesn't exist.
func CreateDefault() (string, error) {
	configPath := DefaultPath()

	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil // Already exists
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	defaultConfig := `# Raven Configuration
# See: https://github.com/aidanlsb/raven

# Default vault name (must exist in [vaults] below)
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
# Optional UI accent color for headers/links in terminal output.
# Supports ANSI color codes (0-255) or hex (#RRGGBB).
# [ui]
# accent = "39"
# code_theme = "monokai"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
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
