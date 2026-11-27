package commandimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
)

func TestHandleConfigShowReturnsEffectiveDefaults(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("# empty config\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result := HandleConfigShow(context.Background(), commandexec.Request{ConfigPath: configPath})
	if !result.OK {
		t.Fatalf("HandleConfigShow() failed: %+v", result.Error)
	}
	data, ok := result.Data.(configsvc.ShowResult)
	if !ok {
		t.Fatalf("data = %#v, want ShowResult", result.Data)
	}
	if data.EditorMode != "auto" {
		t.Fatalf("editor_mode = %#v, want auto", data.EditorMode)
	}
	if data.UI.MarkdownStyle != "auto" {
		t.Fatalf("ui = %#v, want markdown_style=auto", data.UI)
	}
	if data.StatePath != filepath.Join(filepath.Dir(configPath), "state.toml") {
		t.Fatalf("state_path = %#v, want resolved sibling state.toml", data.StatePath)
	}
}

func TestHandleVaultListSurfacesMissingActiveVault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")

	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "work",
		Vaults: map[string]string{
			"work": "/vault/work",
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:     config.StateVersion,
		ActiveVault: "personal",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	result := HandleVaultList(context.Background(), commandexec.Request{
		CommandID:  "vault_list",
		ConfigPath: configPath,
		StatePath:  statePath,
	})
	if !result.OK {
		t.Fatalf("HandleVaultList() failed: %+v", result.Error)
	}
	data, ok := result.Data.(configsvc.VaultListResult)
	if !ok {
		t.Fatalf("data = %#v, want VaultListResult", result.Data)
	}
	if !data.ActiveMissing {
		t.Fatalf("active_missing = %#v, want true", data.ActiveMissing)
	}
	if data.ActiveVault != "personal" {
		t.Fatalf("active_vault = %#v, want personal", data.ActiveVault)
	}
	if data.Current != nil {
		t.Fatalf("current_vault = %#v, want nil", data.Current)
	}
}

func TestHandleVaultListPathOnly(t *testing.T) {
	t.Parallel()

	rawPath := filepath.Join(t.TempDir(), "parent", "..", "vault")
	wantPath := filepath.Clean(rawPath)
	result := HandleVaultList(context.Background(), commandexec.Request{
		CommandID: "vault_list",
		VaultPath: rawPath,
		Args:      map[string]interface{}{"path-only": true},
	})
	if !result.OK {
		t.Fatalf("HandleVaultList() failed: %+v", result.Error)
	}
	data, ok := result.Data.(configsvc.VaultPathResult)
	if !ok {
		t.Fatalf("data = %#v, want VaultPathResult", result.Data)
	}
	if data.Path != wantPath {
		t.Fatalf("HandleVaultList() data = %#v, want only path %q", data, wantPath)
	}
}

func TestHandleVaultFocusDescribesCLIAndMCPApplication(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	vaultPath := filepath.Join(root, "work")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	for _, tt := range []struct {
		name        string
		caller      commandexec.Caller
		wantApplied bool
		wantHint    bool
	}{
		{name: "CLI validates only", caller: commandexec.CallerCLI, wantApplied: false, wantHint: true},
		{name: "MCP applies in adapter", caller: commandexec.CallerMCP, wantApplied: true, wantHint: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := HandleVaultFocus(context.Background(), commandexec.Request{
				Caller: tt.caller,
				Args:   map[string]interface{}{"path": vaultPath},
			})
			if !result.OK {
				t.Fatalf("HandleVaultFocus() failed: %+v", result.Error)
			}
			data, ok := result.Data.(configsvc.VaultFocusPayload)
			if !ok {
				t.Fatalf("data = %#v, want VaultFocusPayload", result.Data)
			}
			if data.Applied != tt.wantApplied {
				t.Fatalf("applied = %#v, want %v", data.Applied, tt.wantApplied)
			}
			hasHint := data.Hint != ""
			if hasHint != tt.wantHint {
				t.Fatalf("hint presence = %v, want %v", hasHint, tt.wantHint)
			}
			if data.Path != vaultPath || data.Scope != "mcp_session" || !data.SessionOnly {
				t.Fatalf("focus data = %#v", data)
			}
		})
	}
}
