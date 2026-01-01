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
	// Vault is the default vault path.
	Vault string `toml:"vault"`

	// Editor is the editor to use for opening files (defaults to $EDITOR).
	Editor string `toml:"editor"`
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
# See: https://github.com/yourusername/raven

# Default vault path (uncomment and set your path)
# vault = "/path/to/your/vault"

# Editor for opening files (defaults to $EDITOR)
# editor = "code"
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
