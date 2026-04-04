package mcpclient

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	home := "/fakehome"

	t.Run("codex", func(t *testing.T) {
		got, err := ConfigPath(Codex, home)
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(home, ".codex", "config.toml") {
			t.Fatalf("unexpected path: %s", got)
		}
	})

	t.Run("claude-code", func(t *testing.T) {
		got, err := ConfigPath(ClaudeCode, home)
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(home, ".claude.json") {
			t.Fatalf("unexpected path: %s", got)
		}
	})

	t.Run("cursor", func(t *testing.T) {
		got, err := ConfigPath(Cursor, home)
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(home, ".cursor", "mcp.json") {
			t.Fatalf("unexpected path: %s", got)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		_, err := ConfigPath(Client("nope"), home)
		if err == nil {
			t.Fatal("expected error for unknown client")
		}
	})
}

func TestBuildServerEntry(t *testing.T) {
	t.Run("no vault", func(t *testing.T) {
		e := BuildServerEntry("", "", "", "")
		if e.Command == "" {
			t.Fatal("expected non-empty command")
		}
		if len(e.Args) != 1 || e.Args[0] != "serve" {
			t.Fatalf("expected args=[serve], got %v", e.Args)
		}
	})

	t.Run("vault name", func(t *testing.T) {
		e := BuildServerEntry("", "", "work", "")
		if len(e.Args) != 3 || e.Args[1] != "--vault" || e.Args[2] != "work" {
			t.Fatalf("expected [serve --vault work], got %v", e.Args)
		}
	})

	t.Run("vault path", func(t *testing.T) {
		e := BuildServerEntry("", "", "", "/my/vault")
		if len(e.Args) != 3 || e.Args[1] != "--vault-path" || e.Args[2] != "/my/vault" {
			t.Fatalf("expected [serve --vault-path /my/vault], got %v", e.Args)
		}
	})

	t.Run("vault path takes precedence", func(t *testing.T) {
		e := BuildServerEntry("", "", "work", "/my/vault")
		if len(e.Args) != 3 || e.Args[1] != "--vault-path" {
			t.Fatalf("expected vault-path to take precedence, got %v", e.Args)
		}
	})

	t.Run("config and state", func(t *testing.T) {
		e := BuildServerEntry("/tmp/config.toml", "/tmp/state.toml", "", "")
		want := []string{"serve", "--config", "/tmp/config.toml", "--state", "/tmp/state.toml"}
		if len(e.Args) != len(want) {
			t.Fatalf("expected %v, got %v", want, e.Args)
		}
		for i := range want {
			if e.Args[i] != want[i] {
				t.Fatalf("expected %v, got %v", want, e.Args)
			}
		}
	})
}

func TestResolveCommand(t *testing.T) {
	prevLookPath := lookPath
	prevArg0 := arg0
	prevExecutablePath := executablePath
	prevAbsPath := absPath
	t.Cleanup(func() {
		lookPath = prevLookPath
		arg0 = prevArg0
		executablePath = prevExecutablePath
		absPath = prevAbsPath
	})

	t.Run("prefers path lookup for rvn", func(t *testing.T) {
		lookPath = func(file string) (string, error) {
			if file == "rvn" {
				return "/opt/homebrew/bin/rvn", nil
			}
			return "", exec.ErrNotFound
		}
		arg0 = func() string { return "rvn" }
		executablePath = func() (string, error) {
			return "/opt/homebrew/Cellar/rvn/0.0.11/bin/rvn", nil
		}

		if got := ResolveCommand(); got != "/opt/homebrew/bin/rvn" {
			t.Fatalf("ResolveCommand() = %q, want %q", got, "/opt/homebrew/bin/rvn")
		}
	})

	t.Run("preserves invoked absolute path", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		invokedPath := filepath.Join(t.TempDir(), "rvn")
		arg0 = func() string { return invokedPath }
		executablePath = func() (string, error) {
			return "/tmp/other/bin/rvn", nil
		}

		if got := ResolveCommand(); got != filepath.Clean(invokedPath) {
			t.Fatalf("ResolveCommand() = %q, want %q", got, filepath.Clean(invokedPath))
		}
	})

	t.Run("falls back to executable path", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		arg0 = func() string { return "rvn" }
		executablePath = func() (string, error) {
			return "/tmp/go/bin/rvn", nil
		}

		if got := ResolveCommand(); got != "/tmp/go/bin/rvn" {
			t.Fatalf("ResolveCommand() = %q, want %q", got, "/tmp/go/bin/rvn")
		}
	})
}

func TestInstallFreshFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	entry := BuildServerEntry("", "", "", "")
	result, err := Install(ClaudeCode, cfgPath, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result != Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	data := readJSON(t, cfgPath)
	servers := data["mcpServers"].(map[string]interface{})
	raven := servers["raven"].(map[string]interface{})
	if raven["command"] != ResolveCommand() {
		t.Fatalf("unexpected command: %v", raven["command"])
	}
}

func TestInstallPreservesExistingKeys(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	initial := map[string]interface{}{
		"someKey": "someValue",
		"mcpServers": map[string]interface{}{
			"other-server": map[string]interface{}{
				"command": "other",
				"args":    []interface{}{"run"},
			},
		},
	}
	writeJSON(t, cfgPath, initial)

	entry := BuildServerEntry("", "", "", "")
	result, err := Install(ClaudeCode, cfgPath, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result != Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	data := readJSON(t, cfgPath)
	if data["someKey"] != "someValue" {
		t.Fatal("existing key lost")
	}
	servers := data["mcpServers"].(map[string]interface{})
	if _, ok := servers["other-server"]; !ok {
		t.Fatal("other-server lost")
	}
	if _, ok := servers["raven"]; !ok {
		t.Fatal("raven not added")
	}
}

func TestInstallIdempotent(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	entry := BuildServerEntry("", "", "", "")
	if _, err := Install(ClaudeCode, cfgPath, entry); err != nil {
		t.Fatal(err)
	}

	result, err := Install(ClaudeCode, cfgPath, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result != AlreadyInstalled {
		t.Fatalf("expected AlreadyInstalled, got %s", result)
	}
}

func TestInstallUpdate(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	entry1 := BuildServerEntry("", "", "", "")
	if _, err := Install(ClaudeCode, cfgPath, entry1); err != nil {
		t.Fatal(err)
	}

	entry2 := BuildServerEntry("", "", "work", "")
	result, err := Install(ClaudeCode, cfgPath, entry2)
	if err != nil {
		t.Fatal(err)
	}
	if result != Updated {
		t.Fatalf("expected Updated, got %s", result)
	}

	data := readJSON(t, cfgPath)
	servers := data["mcpServers"].(map[string]interface{})
	raven := servers["raven"].(map[string]interface{})
	args := raven["args"].([]interface{})
	if len(args) != 3 || args[1] != "--vault" {
		t.Fatalf("unexpected args after update: %v", args)
	}
}

func TestInstallCreatesParentDirs(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "deep", "nested", "config.json")

	entry := BuildServerEntry("", "", "", "")
	result, err := Install(ClaudeCode, cfgPath, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result != Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestRemove(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	entry := BuildServerEntry("", "", "", "")
	if _, err := Install(ClaudeCode, cfgPath, entry); err != nil {
		t.Fatal(err)
	}

	removed, err := Remove(ClaudeCode, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	data := readJSON(t, cfgPath)
	if _, ok := data["mcpServers"]; ok {
		t.Fatal("mcpServers should be removed when empty")
	}
}

func TestRemovePreservesOtherServers(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	initial := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"raven": map[string]interface{}{
				"command": "rvn",
				"args":    []interface{}{"serve"},
			},
			"other": map[string]interface{}{
				"command": "other",
				"args":    []interface{}{"run"},
			},
		},
	}
	writeJSON(t, cfgPath, initial)

	removed, err := Remove(ClaudeCode, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	data := readJSON(t, cfgPath)
	servers := data["mcpServers"].(map[string]interface{})
	if _, ok := servers["raven"]; ok {
		t.Fatal("raven should be removed")
	}
	if _, ok := servers["other"]; !ok {
		t.Fatal("other server should be preserved")
	}
}

func TestRemoveNoFile(t *testing.T) {
	removed, err := Remove(ClaudeCode, "/nonexistent/config.json")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected removed=false for missing file")
	}
}

func TestRemoveNoRaven(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	writeJSON(t, cfgPath, map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"other": map[string]interface{}{
				"command": "other",
			},
		},
	})

	removed, err := Remove(ClaudeCode, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected removed=false when raven not present")
	}
}

func TestStatusNotInstalled(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	cs, err := Status(ClaudeCode, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cs.Exists {
		t.Fatal("expected exists=false")
	}
	if cs.Installed {
		t.Fatal("expected installed=false")
	}
}

func TestStatusInstalled(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	entry := BuildServerEntry("", "", "work", "")
	if _, err := Install(ClaudeCode, cfgPath, entry); err != nil {
		t.Fatal(err)
	}

	cs, err := Status(ClaudeCode, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cs.Exists {
		t.Fatal("expected exists=true")
	}
	if !cs.Installed {
		t.Fatal("expected installed=true")
	}
	if cs.Entry.Command != ResolveCommand() {
		t.Fatalf("unexpected command: %s", cs.Entry.Command)
	}
	if len(cs.Entry.Args) != 3 || cs.Entry.Args[1] != "--vault" || cs.Entry.Args[2] != "work" {
		t.Fatalf("unexpected args: %v", cs.Entry.Args)
	}
}

func TestValidClient(t *testing.T) {
	if !ValidClient("codex") {
		t.Fatal("expected codex to be valid")
	}
	if !ValidClient("claude-code") {
		t.Fatal("expected claude-code to be valid")
	}
	if !ValidClient("claude-desktop") {
		t.Fatal("expected claude-desktop to be valid")
	}
	if !ValidClient("cursor") {
		t.Fatal("expected cursor to be valid")
	}
	if ValidClient("vscode") {
		t.Fatal("expected vscode to be invalid")
	}
}

func TestInstallTOMLFreshFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	entry := BuildServerEntry("", "", "", "")
	result, err := Install(Codex, cfgPath, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result != Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, "[mcp_servers.raven]") {
		t.Fatalf("expected raven table, got %q", content)
	}

	cs, err := Status(Codex, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cs.Exists || !cs.Installed || cs.Entry == nil {
		t.Fatalf("expected installed raven entry, got %+v", cs)
	}
	if cs.Entry.Command != entry.Command {
		t.Fatalf("command = %q, want %q", cs.Entry.Command, entry.Command)
	}
	if !slices.Equal(cs.Entry.Args, entry.Args) {
		t.Fatalf("args = %v, want %v", cs.Entry.Args, entry.Args)
	}
}

func TestInstallTOMLPreservesExistingConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	initial := "model = \"gpt-5\"\n\n[mcp_servers.other]\ncommand = \"other\"\nargs = [\"run\"]\n"
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := BuildServerEntry("", "", "work", "")
	result, err := Install(Codex, cfgPath, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result != Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, "model = \"gpt-5\"") {
		t.Fatal("expected existing key to be preserved")
	}
	if !strings.Contains(content, "[mcp_servers.other]") {
		t.Fatal("expected other MCP server to be preserved")
	}
	if !strings.Contains(content, "[mcp_servers.raven]") {
		t.Fatal("expected raven MCP server to be added")
	}
}

func TestInstallTOMLUpdate(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	entry1 := BuildServerEntry("", "", "", "")
	if _, err := Install(Codex, cfgPath, entry1); err != nil {
		t.Fatal(err)
	}

	entry2 := BuildServerEntry("", "", "work", "")
	result, err := Install(Codex, cfgPath, entry2)
	if err != nil {
		t.Fatal(err)
	}
	if result != Updated {
		t.Fatalf("expected Updated, got %s", result)
	}

	cs, err := Status(Codex, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cs.Installed || cs.Entry == nil {
		t.Fatal("expected installed raven entry")
	}
	if len(cs.Entry.Args) != 3 || cs.Entry.Args[1] != "--vault" || cs.Entry.Args[2] != "work" {
		t.Fatalf("unexpected args after TOML update: %v", cs.Entry.Args)
	}
}

func TestRemoveTOMLPreservesOtherConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	initial := "approval_policy = \"never\"\n\n[mcp_servers.raven]\ncommand = \"rvn\"\nargs = [\"serve\"]\n\n[mcp_servers.other]\ncommand = \"other\"\nargs = [\"run\"]\n"
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := Remove(Codex, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if strings.Contains(content, "[mcp_servers.raven]") {
		t.Fatal("expected raven table to be removed")
	}
	if !strings.Contains(content, "[mcp_servers.other]") {
		t.Fatal("expected other MCP server to be preserved")
	}
	if !strings.Contains(content, "approval_policy = \"never\"") {
		t.Fatal("expected other top-level config to be preserved")
	}
}

func TestStatusInstalledTOML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	content := "[mcp_servers.raven]\ncommand = \"/usr/local/bin/rvn\"\nargs = [\"serve\", \"--vault\", \"work\"]\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cs, err := Status(Codex, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cs.Exists {
		t.Fatal("expected exists=true")
	}
	if !cs.Installed {
		t.Fatal("expected installed=true")
	}
	if cs.Entry == nil || cs.Entry.Command != "/usr/local/bin/rvn" {
		t.Fatalf("unexpected entry: %+v", cs.Entry)
	}
}

// helpers

func readJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return data
}

func writeJSON(t *testing.T, path string, data map[string]interface{}) {
	t.Helper()
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
