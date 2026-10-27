package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestEditorCommandName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "vim", want: "vim"},
		{name: "with args", input: "nvim -u ~/.config/nvim/init.lua", want: "nvim"},
		{name: "extra spaces", input: "  hx   ", want: "hx"},
		{name: "quoted path", input: "\"/Applications/Helix.app/Contents/MacOS/hx\" --config foo", want: "hx"},
		{name: "open app", input: "open -a Cursor", want: "open"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := editorCommandName(tt.input); got != tt.want {
				t.Fatalf("editorCommandName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTerminalEditor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "vim", input: "vim", want: true},
		{name: "nvim args", input: "nvim -u ~/.config/nvim/init.lua", want: true},
		{name: "helix", input: "hx", want: true},
		{name: "open app", input: "open -a VimR", want: false},
		{name: "gui editor", input: "code", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTerminalEditor(tt.input); got != tt.want {
				t.Fatalf("isTerminalEditor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEditorOpenArgsAtLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		editor string
		path   string
		line   int
		want   []string
	}{
		{name: "no line", editor: "code", path: "/tmp/note.md", line: 0, want: []string{"/tmp/note.md"}},
		{name: "cursor", editor: "cursor", path: "/tmp/note.md", line: 42, want: []string{"-g", "/tmp/note.md:42"}},
		{name: "code with args", editor: "code --reuse-window", path: "/tmp/note.md", line: 42, want: []string{"-g", "/tmp/note.md:42"}},
		{name: "vim", editor: "vim", path: "/tmp/note.md", line: 42, want: []string{"+42", "/tmp/note.md"}},
		{name: "helix", editor: "hx", path: "/tmp/note.md", line: 42, want: []string{"/tmp/note.md:42"}},
		{name: "unknown", editor: "unknown", path: "/tmp/note.md", line: 42, want: []string{"/tmp/note.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := editorOpenArgs(tt.editor, tt.path, tt.line)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("editorOpenArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolvedEditor(t *testing.T) {
	t.Run("config editor and gui mode", func(t *testing.T) {
		t.Setenv("EDITOR", "nano")
		cfg := &config.Config{Editor: " code --wait ", EditorMode: "gui"}
		editor, mode := ResolvedEditor(cfg)
		if editor != "code --wait" || mode != "gui" {
			t.Fatalf("ResolvedEditor() = %q, %q; want code --wait, gui", editor, mode)
		}
	})

	t.Run("falls back to EDITOR env", func(t *testing.T) {
		t.Setenv("EDITOR", "nano")
		editor, mode := ResolvedEditor(&config.Config{})
		if editor != "nano" || mode != "auto" {
			t.Fatalf("ResolvedEditor() = %q, %q; want nano, auto", editor, mode)
		}
	})

	t.Run("nil config still uses EDITOR env", func(t *testing.T) {
		t.Setenv("EDITOR", "hx")
		editor, mode := ResolvedEditor(nil)
		if editor != "hx" || mode != "auto" {
			t.Fatalf("ResolvedEditor(nil) = %q, %q; want hx, auto", editor, mode)
		}
	})

	t.Run("unknown mode falls back to auto", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		editor, mode := ResolvedEditor(&config.Config{EditorMode: "tui"})
		if editor != "" || mode != "auto" {
			t.Fatalf("ResolvedEditor(tui) = %q, %q; want \"\", auto", editor, mode)
		}
		_, mode = ResolvedEditor(&config.Config{EditorMode: "background"})
		if mode != "auto" {
			t.Fatalf("ResolvedEditor(background) mode = %q, want auto", mode)
		}
		_, mode = ResolvedEditor(&config.Config{EditorMode: "terminal"})
		if mode != "terminal" {
			t.Fatalf("ResolvedEditor(terminal) mode = %q, want terminal", mode)
		}
	})
}

func TestParseEditorMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string
		want editorMode
	}{
		{name: "default", mode: "", want: editorModeAuto},
		{name: "auto", mode: "auto", want: editorModeAuto},
		{name: "terminal", mode: "terminal", want: editorModeTerminal},
		{name: "former terminal alias", mode: "tui", want: editorModeAuto},
		{name: "gui", mode: "gui", want: editorModeGUI},
		{name: "former gui alias", mode: "background", want: editorModeAuto},
		{name: "unknown", mode: "whatever", want: editorModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{EditorMode: tt.mode}
			if got := parseEditorMode(cfg); got != tt.want {
				t.Fatalf("parseEditorMode(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestOpenInEditorDoesNotInvokeShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-injection fixture is POSIX")
	}

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "pwned")
	targetPath := filepath.Join(tmpDir, "target.md")
	if err := os.WriteFile(targetPath, []byte("note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("EDITOR", "true; printf pwned > '"+marker+"'")
	cfg := &config.Config{EditorMode: "terminal"}
	_ = OpenInEditor(cfg, targetPath)

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("editor string with ';' ran a shell command")
	}
}

func TestOpenInEditorPassesCompoundArgsWithoutShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("compound-editor fixture is POSIX")
	}

	tmpDir := t.TempDir()
	capturePath := filepath.Join(tmpDir, "opened.txt")
	editorPath := filepath.Join(tmpDir, "editor.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RAVEN_EDITOR_CAPTURE\"\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	targetPath := filepath.Join(tmpDir, "target.md")
	t.Setenv("EDITOR", editorPath+" --wait")
	t.Setenv("RAVEN_EDITOR_CAPTURE", capturePath)

	cfg := &config.Config{EditorMode: "terminal"}
	if !OpenInEditor(cfg, targetPath) {
		t.Fatal("expected editor to launch")
	}

	gotBytes, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	got := strings.TrimSpace(string(gotBytes))
	want := "--wait\n" + targetPath
	if got != want {
		t.Fatalf("editor argv = %q, want %q", got, want)
	}
}

func TestCommandForEditorRejectsUnclosedQuote(t *testing.T) {
	t.Parallel()
	if _, err := commandForEditor(`"vim`, []string{"note.md"}); err == nil {
		t.Fatal("expected error for unclosed quote")
	}
}

func TestOpenInEditorFallsBackToEditorEnv(t *testing.T) {
	tmpDir := t.TempDir()
	capturePath := filepath.Join(tmpDir, "opened.txt")

	var editorPath string
	if runtime.GOOS == "windows" {
		editorPath = filepath.Join(tmpDir, "editor.cmd")
		// Batch echoes %~1 (strips surrounding quotes) into the capture file.
		// strings.TrimSpace below tolerates the trailing CRLF that `echo` adds.
		script := "@echo off\r\n> \"%RAVEN_EDITOR_CAPTURE%\" echo %~1\r\n"
		if err := os.WriteFile(editorPath, []byte(script), 0o644); err != nil {
			t.Fatalf("write editor fixture: %v", err)
		}
	} else {
		editorPath = filepath.Join(tmpDir, "editor.sh")
		if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$RAVEN_EDITOR_CAPTURE\"\n"), 0o755); err != nil {
			t.Fatalf("write editor fixture: %v", err)
		}
	}

	targetPath := filepath.Join(tmpDir, "target.md")
	t.Setenv("EDITOR", editorPath)
	t.Setenv("RAVEN_EDITOR_CAPTURE", capturePath)

	cfg := &config.Config{EditorMode: "terminal"}
	if !OpenInEditor(cfg, targetPath) {
		t.Fatal("expected editor to launch")
	}

	gotBytes, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	if got := strings.TrimSpace(string(gotBytes)); got != targetPath {
		t.Fatalf("opened path = %q, want %q", got, targetPath)
	}
}
