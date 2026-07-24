package commandimpl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
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
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want map", result.Data)
	}
	if data["editor_mode"] != "auto" {
		t.Fatalf("editor_mode = %#v, want auto", data["editor_mode"])
	}
	uiData, ok := data["ui"].(map[string]interface{})
	if !ok || uiData["markdown_style"] != "auto" || len(uiData) != 1 {
		t.Fatalf("ui = %#v, want only markdown_style=auto", data["ui"])
	}
	if data["state_path"] != filepath.Join(filepath.Dir(configPath), "state.toml") {
		t.Fatalf("state_path = %#v, want resolved sibling state.toml", data["state_path"])
	}
	if _, exists := data["vault"]; exists {
		t.Fatalf("config show exposed removed vault key: %#v", data)
	}
	if _, exists := data["state_file"]; exists {
		t.Fatalf("config show exposed removed state_file key: %#v", data)
	}
}

func TestHandleVaultCurrentRejectsMissingActiveVault(t *testing.T) {
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

	result := HandleVaultCurrent(context.Background(), commandexec.Request{
		ConfigPath: configPath,
		StatePath:  statePath,
	})
	if result.OK {
		t.Fatalf("HandleVaultCurrent() succeeded: %#v", result.Data)
	}
	if result.Error == nil {
		t.Fatal("HandleVaultCurrent() returned no error details")
	}
	if got := result.Error.Code; got != codes.ErrVaultNotFound {
		t.Fatalf("error code = %q, want %q", got, codes.ErrVaultNotFound)
	}
	if got := result.Error.Message; !strings.Contains(got, "active vault 'personal' not found in config") {
		t.Fatalf("error message = %q, want missing active vault name", got)
	}
	if got := result.Error.Suggestion; !strings.Contains(got, "rvn vault clear") || !strings.Contains(got, "rvn vault list") {
		t.Fatalf("error suggestion = %q, want clear/list guidance", got)
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
			data, ok := result.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("data = %#v, want map", result.Data)
			}
			if data["applied"] != tt.wantApplied {
				t.Fatalf("applied = %#v, want %v", data["applied"], tt.wantApplied)
			}
			_, hasHint := data["hint"]
			if hasHint != tt.wantHint {
				t.Fatalf("hint presence = %v, want %v", hasHint, tt.wantHint)
			}
			if data["path"] != vaultPath || data["scope"] != "mcp_session" || data["session_only"] != true {
				t.Fatalf("focus data = %#v", data)
			}
		})
	}
}
