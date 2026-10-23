package shellquote

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "simple", input: "vim", want: []string{"vim"}},
		{name: "args", input: "open -a Cursor", want: []string{"open", "-a", "Cursor"}},
		{name: "code wait", input: "code --wait", want: []string{"code", "--wait"}},
		{name: "quoted path", input: `"/path/with spaces/vim" -y`, want: []string{"/path/with spaces/vim", "-y"}},
		{name: "single quotes", input: `'/path/with spaces/vim' +10`, want: []string{"/path/with spaces/vim", "+10"}},
		{name: "extra spaces", input: "  nvim   -u  init.lua  ", want: []string{"nvim", "-u", "init.lua"}},
		{name: "semicolon is literal", input: "true; printf pwned", want: []string{"true;", "printf", "pwned"}},
		{name: "substitution is literal", input: "vim $(id)", want: []string{"vim", "$(id)"}},
		{name: "pipe is literal", input: "cat | sh", want: []string{"cat", "|", "sh"}},
		{name: "empty", input: "   ", wantErr: true},
		{name: "unclosed single", input: "'vim", wantErr: true},
		{name: "unclosed double", input: `"vim`, wantErr: true},
		{name: "trailing backslash", input: `vim\`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Split(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Split(%q) = %#v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Split(%q) = %v", tt.input, err)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("Split(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}
