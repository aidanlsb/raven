// Package mcpclient manages MCP server entries in client config files
// (Claude Code, Claude Desktop, Cursor).
package mcpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/aidanlsb/raven/internal/atomicfile"
)

// Client identifies an MCP client application.
type Client string

const (
	Codex         Client = "codex"
	ClaudeCode    Client = "claude-code"
	ClaudeDesktop Client = "claude-desktop"
	Cursor        Client = "cursor"
)

// AllClients returns all supported MCP clients.
func AllClients() []Client {
	return []Client{Codex, ClaudeCode, ClaudeDesktop, Cursor}
}

// ValidClient returns true if c is a recognized client name.
func ValidClient(c string) bool {
	switch Client(c) {
	case Codex, ClaudeCode, ClaudeDesktop, Cursor:
		return true
	}
	return false
}

// ServerEntry describes an MCP server configuration.
type ServerEntry struct {
	Command string   `json:"command" toml:"command"`
	Args    []string `json:"args" toml:"args"`
}

// ClientStatus reports whether raven is configured for a given client.
type ClientStatus struct {
	Client     Client       `json:"client"`
	ConfigPath string       `json:"config_path"`
	Exists     bool         `json:"exists"`
	Installed  bool         `json:"installed"`
	Entry      *ServerEntry `json:"entry,omitempty"`
}

var (
	lookPath = exec.LookPath
	arg0     = func() string {
		if len(os.Args) == 0 {
			return ""
		}
		return os.Args[0]
	}
	executablePath = os.Executable
	absPath        = filepath.Abs
)

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
	case Codex:
		return filepath.Join(homeDir, ".codex", "config.toml"), nil
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

func IsTOMLClient(client Client) bool {
	return client == Codex
}

// ResolveCommand returns the command path that should be written into MCP
// client config. It prefers the shell-visible rvn path from the current
// invocation context and only falls back to os.Executable when needed.
func ResolveCommand() string {
	if command := resolveInvokedCommand(arg0()); command != "" {
		return command
	}

	exe, err := executablePath()
	if err != nil {
		return "rvn"
	}
	return exe
}

func resolveInvokedCommand(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = "rvn"
	}

	if strings.ContainsAny(name, `/\`) {
		if filepath.IsAbs(name) {
			return filepath.Clean(name)
		}
		absolute, err := absPath(name)
		if err != nil {
			return filepath.Clean(name)
		}
		return absolute
	}

	if path, err := lookPath(name); err == nil && strings.TrimSpace(path) != "" {
		return path
	}
	if name != "rvn" {
		if path, err := lookPath("rvn"); err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}

	return ""
}

// BuildServerEntry creates a ServerEntry for the raven MCP server.
// configPath/statePath preserve CLI config resolution context; vaultName and
// vaultPath are optional vault pinning args.
func BuildServerEntry(configPath, statePath, vaultName, vaultPath string) ServerEntry {
	args := []string{"serve"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	if statePath != "" {
		args = append(args, "--state", statePath)
	}
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
func Install(client Client, configPath string, entry ServerEntry) (InstallResult, error) {
	if IsTOMLClient(client) {
		return installTOML(configPath, entry)
	}

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
func Remove(client Client, configPath string) (bool, error) {
	if IsTOMLClient(client) {
		return removeTOML(configPath)
	}

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
	if IsTOMLClient(client) {
		return statusTOML(client, configPath)
	}

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

// ShowSnippet returns the client-specific snippet for manual configuration.
func ShowSnippet(client Client, entry ServerEntry) (string, error) {
	if IsTOMLClient(client) {
		return renderTOMLServerSnippet(entry)
	}

	snippet := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"raven": map[string]interface{}{
				"command": entry.Command,
				"args":    entry.Args,
			},
		},
	}

	out, err := json.MarshalIndent(snippet, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal json snippet: %w", err)
	}
	return string(out), nil
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

type tomlClientConfig struct {
	MCPServers map[string]ServerEntry `toml:"mcp_servers"`
}

var tomlRavenTable = regexp.MustCompile(`(?m)^[ \t]*\[mcp_servers\.raven\][ \t]*(?:#.*)?$`)

func statusTOML(client Client, configPath string) (*ClientStatus, error) {
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

	var data tomlClientConfig
	if _, err := toml.Decode(string(raw), &data); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	entry, ok := data.MCPServers["raven"]
	if !ok {
		return cs, nil
	}

	cs.Installed = true
	cs.Entry = &ServerEntry{
		Command: entry.Command,
		Args:    append([]string(nil), entry.Args...),
	}
	return cs, nil
}

func installTOML(configPath string, entry ServerEntry) (InstallResult, error) {
	cs, err := statusTOML(Codex, configPath)
	if err != nil {
		return 0, err
	}
	if cs.Installed && cs.Entry != nil && cs.Entry.Command == entry.Command && stringSlicesEqual(cs.Entry.Args, entry.Args) {
		return AlreadyInstalled, nil
	}

	snippet, err := renderTOMLServerSnippet(entry)
	if err != nil {
		return 0, err
	}

	result := Installed
	raw, err := os.ReadFile(configPath)
	switch {
	case os.IsNotExist(err):
		raw = nil
	case err != nil:
		return 0, fmt.Errorf("read config: %w", err)
	default:
		if cs.Installed {
			result = Updated
		}
	}

	var out []byte
	if cs.Installed {
		start, end, found := findTableBounds(raw, tomlRavenTable)
		if !found {
			return 0, fmt.Errorf("parse config: missing [mcp_servers.raven] table")
		}
		out = make([]byte, 0, len(raw)-end+start+len(snippet)+1)
		out = append(out, raw[:start]...)
		if start > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, snippet...)
		if !strings.HasSuffix(snippet, "\n") {
			out = append(out, '\n')
		}
		out = append(out, raw[end:]...)
	} else {
		out = appendTOMLSnippet(raw, snippet)
	}

	if err := writeRawConfig(configPath, out); err != nil {
		return 0, err
	}
	return result, nil
}

func removeTOML(configPath string) (bool, error) {
	cs, err := statusTOML(Codex, configPath)
	if err != nil {
		return false, err
	}
	if !cs.Exists || !cs.Installed {
		return false, nil
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}

	start, end, found := findTableBounds(raw, tomlRavenTable)
	if !found {
		return false, fmt.Errorf("parse config: missing [mcp_servers.raven] table")
	}

	out := make([]byte, 0, len(raw)-(end-start))
	out = append(out, raw[:start]...)
	out = append(out, raw[end:]...)
	out = bytes.TrimLeft(out, "\n")
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}

	if err := writeRawConfig(configPath, out); err != nil {
		return false, err
	}
	return true, nil
}

func renderTOMLServerSnippet(entry ServerEntry) (string, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(entry); err != nil {
		return "", fmt.Errorf("marshal toml snippet: %w", err)
	}
	return "[mcp_servers.raven]\n" + strings.TrimRight(buf.String(), "\n") + "\n", nil
}

func findTableBounds(raw []byte, header *regexp.Regexp) (int, int, bool) {
	loc := header.FindIndex(raw)
	if loc == nil {
		return 0, 0, false
	}

	start := loc[0]
	for start > 0 && raw[start-1] != '\n' {
		start--
	}

	searchFrom := loc[1]
	if searchFrom < len(raw) && raw[searchFrom] == '\r' {
		searchFrom++
	}
	if searchFrom < len(raw) && raw[searchFrom] == '\n' {
		searchFrom++
	}

	end := len(raw)
	tableLoc := regexp.MustCompile(`(?m)^[ \t]*\[[^\]]+\]`).FindIndex(raw[searchFrom:])
	if tableLoc != nil {
		end = searchFrom + tableLoc[0]
		for end > start && (raw[end-1] == '\n' || raw[end-1] == '\r') {
			end--
		}
		if end < len(raw) {
			end++
		}
	}

	return start, end, true
}

func appendTOMLSnippet(raw []byte, snippet string) []byte {
	if len(raw) == 0 {
		return []byte(snippet)
	}

	out := append([]byte(nil), raw...)
	out = bytes.TrimRight(out, "\n")
	out = append(out, '\n', '\n')
	out = append(out, []byte(snippet)...)
	return out
}

func writeRawConfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	return atomicfile.WriteFile(path, data, 0o644)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
