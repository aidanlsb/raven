package lsp

import (
	"path/filepath"
	"testing"
)

func TestCharacterByteConversions(t *testing.T) {
	t.Parallel()

	// "héllo 🌍!" — 'é' is 2 bytes / 1 UTF-16 unit, '🌍' is 4 bytes / 2 UTF-16 units.
	line := "héllo 🌍!"

	tests := []struct {
		name      string
		encoding  string
		character int
		wantByte  int
	}{
		{"utf-8 ascii", encodingUTF8, 1, 1},
		{"utf-8 past end clamps", encodingUTF8, 100, len(line)},
		{"utf-16 start", encodingUTF16, 0, 0},
		{"utf-16 after h", encodingUTF16, 1, 1},
		{"utf-16 after é", encodingUTF16, 2, 3},
		{"utf-16 before emoji", encodingUTF16, 6, 7},
		{"utf-16 after emoji surrogate pair", encodingUTF16, 8, 11},
		{"utf-16 past end clamps", encodingUTF16, 100, len(line)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotByte := characterToByte(line, tt.character, tt.encoding)
			if gotByte != tt.wantByte {
				t.Errorf("characterToByte(%d) = %d, want %d", tt.character, gotByte, tt.wantByte)
			}
			// Round-trip (skip clamped cases).
			if tt.character <= 8 {
				gotChar := byteToCharacter(line, gotByte, tt.encoding)
				if gotChar != tt.character {
					t.Errorf("byteToCharacter(%d) = %d, want %d", gotByte, gotChar, tt.character)
				}
			}
		})
	}
}

func TestURIPathConversions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		uri  string
	}{
		{"/vault/people/freya.md", "file:///vault/people/freya.md"},
		{"/vault/with space/note.md", "file:///vault/with%20space/note.md"},
	}
	for _, tt := range tests {
		if got := pathToURI(tt.path); got != tt.uri {
			t.Errorf("pathToURI(%q) = %q, want %q", tt.path, got, tt.uri)
		}
		// uriToPath returns OS-native separators (backslashes on Windows).
		if got := uriToPath(tt.uri); got != filepath.FromSlash(tt.path) {
			t.Errorf("uriToPath(%q) = %q, want %q", tt.uri, got, filepath.FromSlash(tt.path))
		}
	}

	if got := uriToPath("https://example.com/x"); got != "" {
		t.Errorf("uriToPath(non-file) = %q, want empty", got)
	}
}

func TestInFrontmatter(t *testing.T) {
	t.Parallel()

	lines := documentLines("---\ntype: person\nname: Freya\n---\n\nBody text\n")
	tests := []struct {
		line int
		want bool
	}{
		{0, false}, // opening delimiter
		{1, true},
		{2, true},
		{3, false}, // closing delimiter
		{5, false}, // body
	}
	for _, tt := range tests {
		if got := inFrontmatter(lines, tt.line); got != tt.want {
			t.Errorf("inFrontmatter(line %d) = %v, want %v", tt.line, got, tt.want)
		}
	}

	if inFrontmatter(documentLines("no frontmatter\n"), 0) {
		t.Error("inFrontmatter should be false without opening delimiter")
	}
}
