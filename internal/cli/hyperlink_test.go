package cli

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestBuildEditorURL(t *testing.T) {
	tests := []struct {
		name     string
		editor   string
		absPath  string
		line     int
		wantURL  string
	}{
		{
			name:    "cursor editor",
			editor:  "cursor",
			absPath: "/Users/test/vault/file.md",
			line:    42,
			wantURL: "cursor://file/Users/test/vault/file.md:42:1",
		},
		{
			name:    "cursor via open command",
			editor:  "open -a Cursor",
			absPath: "/Users/test/vault/file.md",
			line:    10,
			wantURL: "cursor://file/Users/test/vault/file.md:10:1",
		},
		{
			name:    "vscode",
			editor:  "code",
			absPath: "/Users/test/vault/file.md",
			line:    5,
			wantURL: "vscode://file/Users/test/vault/file.md:5:1",
		},
		{
			name:    "sublime text",
			editor:  "subl",
			absPath: "/Users/test/vault/file.md",
			line:    15,
			wantURL: "subl://open?url=file:///Users/test/vault/file.md&line=15",
		},
		{
			name:    "jetbrains idea",
			editor:  "idea",
			absPath: "/Users/test/vault/file.md",
			line:    20,
			wantURL: "idea://open?file=/Users/test/vault/file.md&line=20",
		},
		{
			name:    "goland",
			editor:  "goland",
			absPath: "/Users/test/vault/file.md",
			line:    25,
			wantURL: "idea://open?file=/Users/test/vault/file.md&line=25",
		},
		{
			name:    "zed",
			editor:  "zed",
			absPath: "/Users/test/vault/file.md",
			line:    30,
			wantURL: "zed://file/Users/test/vault/file.md:30",
		},
		{
			name:    "vim fallback to file://",
			editor:  "vim",
			absPath: "/Users/test/vault/file.md",
			line:    1,
			wantURL: "file:///Users/test/vault/file.md",
		},
		{
			name:    "unknown editor fallback",
			editor:  "nano",
			absPath: "/Users/test/vault/file.md",
			line:    1,
			wantURL: "file:///Users/test/vault/file.md",
		},
		{
			name:    "no editor configured",
			editor:  "",
			absPath: "/Users/test/vault/file.md",
			line:    1,
			wantURL: "file:///Users/test/vault/file.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Editor: tt.editor}
			gotURL := buildEditorURL(cfg, tt.absPath, tt.line)
			if gotURL != tt.wantURL {
				t.Errorf("buildEditorURL() = %q, want %q", gotURL, tt.wantURL)
			}
		})
	}
}

func TestBuildEditorURLNilConfig(t *testing.T) {
	// Should not panic with nil config
	url := buildEditorURL(nil, "/path/to/file.md", 10)
	if url != "file:///path/to/file.md" {
		t.Errorf("buildEditorURL(nil) = %q, want file URL", url)
	}
}
