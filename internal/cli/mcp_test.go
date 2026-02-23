package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/mcpclient"
)

func TestMcpInstallAndRemove(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "claude.json")

	// Install
	entry := mcpclient.BuildServerEntry("", "")
	result, err := mcpclient.Install(cfgPath, entry)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if result != mcpclient.Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	// Verify file contents
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	servers := data["mcpServers"].(map[string]interface{})
	raven := servers["raven"].(map[string]interface{})
	if raven["command"] != mcpclient.ResolveCommand() {
		t.Fatalf("unexpected command: %v", raven["command"])
	}
	args := raven["args"].([]interface{})
	if len(args) != 1 || args[0] != "serve" {
		t.Fatalf("unexpected args: %v", args)
	}

	// Idempotent re-install
	result, err = mcpclient.Install(cfgPath, entry)
	if err != nil {
		t.Fatalf("re-install: %v", err)
	}
	if result != mcpclient.AlreadyInstalled {
		t.Fatalf("expected AlreadyInstalled, got %s", result)
	}

	// Status
	cs, err := mcpclient.Status(mcpclient.ClaudeCode, cfgPath)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !cs.Installed {
		t.Fatal("expected installed=true")
	}

	// Remove
	removed, err := mcpclient.Remove(cfgPath)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	// Verify removed
	cs, err = mcpclient.Status(mcpclient.ClaudeCode, cfgPath)
	if err != nil {
		t.Fatalf("status after remove: %v", err)
	}
	if cs.Installed {
		t.Fatal("expected installed=false after remove")
	}
}

func TestMcpInstallWithVault(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "claude.json")

	entry := mcpclient.BuildServerEntry("work", "")
	result, err := mcpclient.Install(cfgPath, entry)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if result != mcpclient.Installed {
		t.Fatalf("expected Installed, got %s", result)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	servers := data["mcpServers"].(map[string]interface{})
	raven := servers["raven"].(map[string]interface{})
	args := raven["args"].([]interface{})
	if len(args) != 3 || args[1] != "--vault" || args[2] != "work" {
		t.Fatalf("expected [serve --vault work], got %v", args)
	}
}

func TestMcpShowBuildsCorrectSnippet(t *testing.T) {
	entry := mcpclient.BuildServerEntry("", "")
	if entry.Command == "" {
		t.Fatal("expected non-empty command")
	}
	if len(entry.Args) != 1 || entry.Args[0] != "serve" {
		t.Fatalf("expected args=[serve], got %v", entry.Args)
	}

	entryWithVault := mcpclient.BuildServerEntry("personal", "")
	if len(entryWithVault.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(entryWithVault.Args))
	}
}
