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

const (
	// StateVersion is the current state file schema version.
	StateVersion = 1
)

// State represents mutable machine-local runtime state.
type State struct {
	Version     int    `toml:"version"`
	ActiveVault string `toml:"active_vault,omitempty"`
}

// ResolveConfigPath resolves the effective config path from an optional override.
func ResolveConfigPath(explicitConfigPath string) string {
	if strings.TrimSpace(explicitConfigPath) != "" {
		return explicitConfigPath
	}
	return DefaultPath()
}

// ResolveStatePath resolves the state.toml path with precedence:
//  1. explicitStatePath flag
//  2. cfg.StateFile from config.toml (relative to config file dir when not absolute)
//  3. sibling state.toml next to config.toml
func ResolveStatePath(explicitStatePath, configPath string, cfg *Config) string {
	if strings.TrimSpace(explicitStatePath) != "" {
		return explicitStatePath
	}

	resolvedConfigPath := ResolveConfigPath(configPath)
	configDir := filepath.Dir(resolvedConfigPath)

	if cfg != nil {
		if fromConfig := strings.TrimSpace(cfg.StateFile); fromConfig != "" {
			if isAbsoluteStatePath(fromConfig) {
				return filepath.Clean(filepath.FromSlash(fromConfig))
			}
			return filepath.Join(configDir, filepath.FromSlash(fromConfig))
		}
	}

	return filepath.Join(configDir, "state.toml")
}

func isAbsoluteStatePath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	// Treat slash-rooted config values as absolute on every OS.
	return strings.HasPrefix(filepath.ToSlash(strings.TrimSpace(p)), "/")
}

// LoadState loads state.toml from a specific path.
// Returns a default state when the file does not exist.
func LoadState(path string) (*State, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("state path is required")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &State{Version: StateVersion}, nil
	}

	var state State
	if _, err := toml.DecodeFile(path, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state %s: %w", path, err)
	}

	if state.Version == 0 {
		state.Version = StateVersion
	}
	state.ActiveVault = strings.TrimSpace(state.ActiveVault)

	return &state, nil
}

// SaveState writes state.toml atomically.
func SaveState(path string, state *State) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("state path is required")
	}
	if state == nil {
		state = &State{}
	}

	normalized := *state
	if normalized.Version == 0 {
		normalized.Version = StateVersion
	}
	normalized.ActiveVault = strings.TrimSpace(normalized.ActiveVault)

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(normalized); err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	if err := atomicfile.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write state %s: %w", path, err)
	}

	return nil
}
