package parser

import (
	"testing"
)

func TestExtractHeadings(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		startLine int
		want      []Heading
	}{
		{
			name:      "basic headings",
			content:   "# Heading 1\n\nSome text\n\n## Heading 2\n\n### Heading 3",
			startLine: 1,
			want: []Heading{
				{Level: 1, Text: "Heading 1", Line: 1},
				{Level: 2, Text: "Heading 2", Line: 5},
				{Level: 3, Text: "Heading 3", Line: 7},
			},
		},
		{
			name:      "heading in code block ignored",
			content:   "# Real Heading\n\n```\n# Not a heading\n```\n\n## Another Real",
			startLine: 1,
			want: []Heading{
				{Level: 1, Text: "Real Heading", Line: 1},
				{Level: 2, Text: "Another Real", Line: 7},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHeadings(tt.content, tt.startLine)

			if len(got) != len(tt.want) {
				t.Errorf("got %d headings, want %d", len(got), len(tt.want))
				return
			}

			for i, h := range tt.want {
				if got[i].Level != h.Level {
					t.Errorf("heading[%d].Level = %d, want %d", i, got[i].Level, h.Level)
				}
				if got[i].Text != h.Text {
					t.Errorf("heading[%d].Text = %q, want %q", i, got[i].Text, h.Text)
				}
			}
		})
	}
}

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
