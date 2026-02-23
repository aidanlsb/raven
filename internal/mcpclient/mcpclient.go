// Package mcpclient manages MCP server entries in client config files
// (Claude Code, Claude Desktop, Cursor).
package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aidanlsb/raven/internal/atomicfile"
)

// Client identifies an MCP client application.
type Client string

const (
	ClaudeCode    Client = "claude-code"
	ClaudeDesktop Client = "claude-desktop"
	Cursor        Client = "cursor"
)

// AllClients returns all supported MCP clients.
func AllClients() []Client {
	return []Client{ClaudeCode, ClaudeDesktop, Cursor}
}

// ValidClient returns true if c is a recognized client name.
func ValidClient(c string) bool {
	switch Client(c) {
	case ClaudeCode, ClaudeDesktop, Cursor:
		return true
	}
	return false
}

// ServerEntry describes an MCP server configuration.
type ServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// ClientStatus reports whether raven is configured for a given client.
type ClientStatus struct {
	Client     Client       `json:"client"`
	ConfigPath string       `json:"config_path"`
	Exists     bool         `json:"exists"`
	Installed  bool         `json:"installed"`
	Entry      *ServerEntry `json:"entry,omitempty"`
}

// ConfigPath returns the config file path for the given client.
// homeDir can be overridden for testing; pass "" to use os.UserHomeDir.
func ConfigPath(client Client, homeDir string) (string, error) {
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
	}

	switch client {
	case ClaudeCode:
		return filepath.Join(homeDir, ".claude.json"), nil
	case ClaudeDesktop:
		if runtime.GOOS == "darwin" {
			return filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
		}
		// Windows / Linux – best effort
		return filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json"), nil
	case Cursor:
		return filepath.Join(homeDir, ".cursor", "mcp.json"), nil
	default:
		return "", fmt.Errorf("unknown client: %s", client)
	}
}

// ResolveCommand returns the absolute path to the running rvn binary.
// Falls back to "rvn" if the path cannot be determined.
func ResolveCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "rvn"
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}

// BuildServerEntry creates a ServerEntry for the raven MCP server.
// vaultName and vaultPath are optional vault pinning args.
func BuildServerEntry(vaultName, vaultPath string) ServerEntry {
	args := []string{"serve"}
	if vaultPath != "" {
		args = append(args, "--vault-path", vaultPath)
	} else if vaultName != "" {
		args = append(args, "--vault", vaultName)
	}
	return ServerEntry{
		Command: ResolveCommand(),
		Args:    args,
	}
}

// InstallResult describes what happened during an install.
type InstallResult int

const (
	Installed InstallResult = iota
	Updated
	AlreadyInstalled
)

func (r InstallResult) String() string {
	switch r {
	case Installed:
		return "installed"
	case Updated:
		return "updated"
	case AlreadyInstalled:
		return "already_installed"
	default:
		return "unknown"
	}
}

// Install adds or updates the raven MCP server entry in the client config.
func Install(configPath string, entry ServerEntry) (InstallResult, error) {
	data, err := readOrCreateConfig(configPath)
	if err != nil {
		return 0, err
	}

	mcpServers := ensureMCPServers(data)

	existing, hasRaven := mcpServers["raven"]
	if hasRaven && entriesEqual(existing, entry) {
		return AlreadyInstalled, nil
	}

	result := Installed
	if hasRaven {
		result = Updated
	}

	mcpServers["raven"] = map[string]interface{}{
		"command": entry.Command,
		"args":    entry.Args,
	}

	return result, writeConfig(configPath, data)
}

// Remove deletes the raven MCP server entry from the client config.
// Returns true if raven was present and removed.
func Remove(configPath string) (bool, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read config: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false, fmt.Errorf("parse config: %w", err)
	}

	mcpServersRaw, ok := data["mcpServers"]
	if !ok {
		return false, nil
	}
	mcpServers, ok := mcpServersRaw.(map[string]interface{})
	if !ok {
		return false, nil
	}

	if _, hasRaven := mcpServers["raven"]; !hasRaven {
		return false, nil
	}

	delete(mcpServers, "raven")

	// Remove mcpServers key if empty
	if len(mcpServers) == 0 {
		delete(data, "mcpServers")
	}

	return true, writeConfig(configPath, data)
}

// Status checks whether raven is configured in the given client config.
func Status(client Client, configPath string) (*ClientStatus, error) {
	cs := &ClientStatus{
		Client:     client,
		ConfigPath: configPath,
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cs, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	cs.Exists = true

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	mcpServersRaw, ok := data["mcpServers"]
	if !ok {
		return cs, nil
	}
	mcpServers, ok := mcpServersRaw.(map[string]interface{})
	if !ok {
		return cs, nil
	}

	ravenRaw, ok := mcpServers["raven"]
	if !ok {
		return cs, nil
	}
	ravenMap, ok := ravenRaw.(map[string]interface{})
	if !ok {
		return cs, nil
	}

	cs.Installed = true
	cs.Entry = &ServerEntry{}

	if cmd, ok := ravenMap["command"].(string); ok {
		cs.Entry.Command = cmd
	}
	if argsRaw, ok := ravenMap["args"].([]interface{}); ok {
		for _, a := range argsRaw {
			if s, ok := a.(string); ok {
				cs.Entry.Args = append(cs.Entry.Args, s)
			}
		}
	}

	return cs, nil
}

// readOrCreateConfig reads an existing JSON config or returns an empty map.
// Creates parent directories if needed.
func readOrCreateConfig(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return data, nil
}

func ensureMCPServers(data map[string]interface{}) map[string]interface{} {
	raw, ok := data["mcpServers"]
	if ok {
		if m, ok := raw.(map[string]interface{}); ok {
			return m
		}
	}
	m := map[string]interface{}{}
	data["mcpServers"] = m
	return m
}

func entriesEqual(existing interface{}, want ServerEntry) bool {
	m, ok := existing.(map[string]interface{})
	if !ok {
		return false
	}

	cmd, _ := m["command"].(string)
	if cmd != want.Command {
		return false
	}

	argsRaw, ok := m["args"].([]interface{})
	if !ok {
		return len(want.Args) == 0
	}
	if len(argsRaw) != len(want.Args) {
		return false
	}
	for i, a := range argsRaw {
		s, ok := a.(string)
		if !ok || s != want.Args[i] {
			return false
		}
	}
	return true
}

func writeConfig(path string, data map[string]interface{}) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	out = append(out, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	return atomicfile.WriteFile(path, out, 0)
}
