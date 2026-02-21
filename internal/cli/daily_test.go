package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestDailyJSONDoesNotOpenEditorByDefault(t *testing.T) {
	tests := []struct {
		name        string
		precreate   bool
		wantCreated bool
	}{
		{
			name:        "creates missing daily note",
			precreate:   false,
			wantCreated: true,
		},
		{
			name:        "reuses existing daily note",
			precreate:   true,
			wantCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vaultPath := t.TempDir()
			const date = "2026-02-14"
			relPath := filepath.Join("daily", date+".md")
			absPath := filepath.Join(vaultPath, relPath)

			if tt.precreate {
				if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
					t.Fatalf("mkdir daily dir: %v", err)
				}
				if err := os.WriteFile(absPath, []byte("# existing\n"), 0o644); err != nil {
					t.Fatalf("write existing daily note: %v", err)
				}
			}

			markerPath := filepath.Join(vaultPath, "editor-called.marker")
			editorPath := writeFakeEditor(t, vaultPath, markerPath)

			prevVault := resolvedVaultPath
			prevJSON := jsonOutput
			prevCfg := cfg
			prevEdit := dailyEdit
			t.Cleanup(func() {
				resolvedVaultPath = prevVault
				jsonOutput = prevJSON
				cfg = prevCfg
				dailyEdit = prevEdit
			})

			resolvedVaultPath = vaultPath
			jsonOutput = true
			cfg = &config.Config{Editor: editorPath}
			dailyEdit = false

			out := captureStdout(t, func() {
				if err := dailyCmd.RunE(dailyCmd, []string{date}); err != nil {
					t.Fatalf("dailyCmd.RunE: %v", err)
				}
			})

			var resp struct {
				OK   bool `json:"ok"`
				Data struct {
					File    string `json:"file"`
					Date    string `json:"date"`
					Created bool   `json:"created"`
					Opened  bool   `json:"opened"`
					Editor  string `json:"editor"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(out), &resp); err != nil {
				t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
			}
			if !resp.OK {
				t.Fatalf("expected ok=true; out=%s", out)
			}
			if resp.Data.File != relPath {
				t.Fatalf("file = %q, want %q", resp.Data.File, relPath)
			}
			if resp.Data.Date != date {
				t.Fatalf("date = %q, want %q", resp.Data.Date, date)
			}
			if resp.Data.Created != tt.wantCreated {
				t.Fatalf("created = %v, want %v", resp.Data.Created, tt.wantCreated)
			}
			if resp.Data.Opened {
				t.Fatalf("opened = true, want false")
			}
			if resp.Data.Editor != editorPath {
				t.Fatalf("editor = %q, want %q", resp.Data.Editor, editorPath)
			}

			if _, err := os.Stat(markerPath); err == nil {
				t.Fatalf("editor was launched unexpectedly in JSON mode without --edit")
			} else if !os.IsNotExist(err) {
				t.Fatalf("checking marker file: %v", err)
			}

			if _, err := os.Stat(absPath); err != nil {
				t.Fatalf("daily note does not exist at %s: %v", absPath, err)
			}
		})
	}
}

func TestDailyJSONOpensEditorWhenEditEnabled(t *testing.T) {
	vaultPath := t.TempDir()
	const date = "2026-02-15"
	markerPath := filepath.Join(vaultPath, "editor-called.marker")
	editorPath := writeFakeEditor(t, vaultPath, markerPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevCfg := cfg
	prevEdit := dailyEdit
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		cfg = prevCfg
		dailyEdit = prevEdit
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	cfg = &config.Config{
		Editor:     editorPath,
		EditorMode: "terminal",
	}
	dailyEdit = true

	out := captureStdout(t, func() {
		if err := dailyCmd.RunE(dailyCmd, []string{date}); err != nil {
			t.Fatalf("dailyCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Created bool `json:"created"`
			Opened  bool `json:"opened"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if !resp.Data.Created {
		t.Fatalf("created = false, want true")
	}
	if !resp.Data.Opened {
		t.Fatalf("opened = false, want true")
	}

	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected editor marker file to exist: %v", err)
	}
}

func TestDailyHumanModeOpensEditorByDefault(t *testing.T) {
	vaultPath := t.TempDir()
	const date = "2026-02-16"
	relPath := filepath.Join("daily", date+".md")
	absPath := filepath.Join(vaultPath, relPath)

	markerPath := filepath.Join(vaultPath, "editor-called.marker")
	editorPath := writeFakeEditor(t, vaultPath, markerPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevCfg := cfg
	prevEdit := dailyEdit
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		cfg = prevCfg
		dailyEdit = prevEdit
	})

	resolvedVaultPath = vaultPath
	jsonOutput = false
	cfg = &config.Config{
		Editor:     editorPath,
		EditorMode: "terminal",
	}
	dailyEdit = false

	captureStdout(t, func() {
		if err := dailyCmd.RunE(dailyCmd, []string{date}); err != nil {
			t.Fatalf("dailyCmd.RunE: %v", err)
		}
	})

	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected editor marker file to exist: %v", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		t.Fatalf("daily note does not exist at %s: %v", absPath, err)
	}
}

func writeFakeEditor(t *testing.T, dir, markerPath string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		editorPath := filepath.Join(dir, "fake-editor.cmd")
		script := "@echo off\r\necho opened > \"" + markerPath + "\"\r\n"
		if err := os.WriteFile(editorPath, []byte(script), 0o644); err != nil {
			t.Fatalf("write fake editor: %v", err)
		}
		return editorPath
	}

	editorPath := filepath.Join(dir, "fake-editor.sh")
	script := "#!/bin/sh\necho opened > \"" + markerPath + "\"\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	return editorPath
}
