package vault

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestEditorCommandName(t *testing.T) {
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

func TestParseEditorMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want editorMode
	}{
		{name: "default", mode: "", want: editorModeAuto},
		{name: "auto", mode: "auto", want: editorModeAuto},
		{name: "terminal", mode: "terminal", want: editorModeTerminal},
		{name: "terminal alias", mode: "tui", want: editorModeTerminal},
		{name: "gui", mode: "gui", want: editorModeGUI},
		{name: "background alias", mode: "background", want: editorModeGUI},
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
