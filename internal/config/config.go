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
	// DefaultKeep is the name of the default keep (from Keeps map).
	DefaultKeep string `toml:"default_keep"`

	// StateFile is an optional path to state.toml.
	// If relative, it's resolved relative to config.toml's directory.
	StateFile string `toml:"state_file"`

	// Keep is the legacy single keep path (for backwards compatibility).
	// Deprecated: Use DefaultKeep + Keeps instead.
	Keep string `toml:"keep"`

	// Keeps is a map of keep names to paths.
	Keeps map[string]string `toml:"keeps"`

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

// GetKeepPath returns the path for a named keep.
// If name is empty, returns the default keep path.
func (c *Config) GetKeepPath(name string) (string, error) {
	// If no name specified, use default
	if name == "" {
		name = c.DefaultKeep
	}

	// If still no name but legacy keep is set, use that
	if name == "" && c.Keep != "" {
		return c.Keep, nil
	}

	// Legacy compatibility: when only legacy keep is configured, allow "default" as the name.
	if name == "default" && c.Keep != "" && len(c.Keeps) == 0 {
		return c.Keep, nil
	}

	// Look up named keep
	if c.Keeps != nil {
		if path, ok := c.Keeps[name]; ok {
			return path, nil
		}
	}

	// If name matches default and legacy keep exists
	if name == "" && c.Keep != "" {
		return c.Keep, nil
	}

	if name == "" {
		return "", fmt.Errorf("no default keep configured")
	}

	return "", fmt.Errorf("keep '%s' not found in config", name)
}

// GetDefaultKeepPath returns the default keep path.
func (c *Config) GetDefaultKeepPath() (string, error) {
	return c.GetKeepPath("")
}

// ListKeeps returns all configured keeps with their paths.
func (c *Config) ListKeeps() map[string]string {
	result := make(map[string]string)

	// Add named keeps
	for name, path := range c.Keeps {
		result[name] = path
	}

	// If legacy keep and no named keeps, add as "default"
	if len(result) == 0 && c.Keep != "" {
		result["default"] = c.Keep
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
	return CreateDefaultAt(DefaultPath())
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

# Default keep name (must exist in [keeps] below)
# default_keep = "personal"

# Optional state file location (default: sibling state.toml next to config.toml)
# state_file = "state.toml"

# Named keeps
# [keeps]
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
