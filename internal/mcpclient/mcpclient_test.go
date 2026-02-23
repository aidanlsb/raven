package mcpclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPath(t *testing.T) {
	home := "/fakehome"

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
		e := BuildServerEntry("", "")
		if e.Command == "" {
			t.Fatal("expected non-empty command")
		}
		if len(e.Args) != 1 || e.Args[0] != "serve" {
			t.Fatalf("expected args=[serve], got %v", e.Args)
		}
	})

	t.Run("vault name", func(t *testing.T) {
		e := BuildServerEntry("work", "")
		if len(e.Args) != 3 || e.Args[1] != "--vault" || e.Args[2] != "work" {
			t.Fatalf("expected [serve --vault work], got %v", e.Args)
		}
	})

	t.Run("vault path", func(t *testing.T) {
		e := BuildServerEntry("", "/my/vault")
		if len(e.Args) != 3 || e.Args[1] != "--vault-path" || e.Args[2] != "/my/vault" {
			t.Fatalf("expected [serve --vault-path /my/vault], got %v", e.Args)
		}
	})

	t.Run("vault path takes precedence", func(t *testing.T) {
		e := BuildServerEntry("work", "/my/vault")
		if len(e.Args) != 3 || e.Args[1] != "--vault-path" {
			t.Fatalf("expected vault-path to take precedence, got %v", e.Args)
		}
	})
}

func TestInstallFreshFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	entry := BuildServerEntry("", "")
	result, err := Install(cfgPath, entry)
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

	entry := BuildServerEntry("", "")
	result, err := Install(cfgPath, entry)
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

	entry := BuildServerEntry("", "")
	if _, err := Install(cfgPath, entry); err != nil {
		t.Fatal(err)
	}

	result, err := Install(cfgPath, entry)
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

	entry1 := BuildServerEntry("", "")
	if _, err := Install(cfgPath, entry1); err != nil {
		t.Fatal(err)
	}

	entry2 := BuildServerEntry("work", "")
	result, err := Install(cfgPath, entry2)
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

	entry := BuildServerEntry("", "")
	result, err := Install(cfgPath, entry)
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

	entry := BuildServerEntry("", "")
	if _, err := Install(cfgPath, entry); err != nil {
		t.Fatal(err)
	}

	removed, err := Remove(cfgPath)
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

	removed, err := Remove(cfgPath)
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
	removed, err := Remove("/nonexistent/config.json")
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

	removed, err := Remove(cfgPath)
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

	entry := BuildServerEntry("work", "")
	if _, err := Install(cfgPath, entry); err != nil {
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
