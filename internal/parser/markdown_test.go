package parser

import (
	"testing"
)

// NOTE: Heading extraction is tested via ExtractFromAST in ast_test.go.
// The Slugify function is tested below.

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"1:1 Topics", "1-1-topics"},
		{"Thor Odinson", "thor-odinson"},
		{"UPPERCASE", "uppercase"},
		{"multiple   spaces", "multiple-spaces"},
		{"special!@#chars", "specialchars"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
