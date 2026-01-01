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

func TestExtractInlineTags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "basic tags",
			content: "Some text with #tag1 and #tag2, also (#tag3)",
			want:    []string{"tag1", "tag2", "tag3"},
		},
		{
			name:    "tags in code block ignored",
			content: "Real #tag here\n\n```\n#not-a-tag\n```\n\nAnd `#also-not-tag` inline",
			want:    []string{"tag"},
		},
		{
			name:    "issue numbers not tags",
			content: "Fix #123 and add #feature",
			want:    []string{"feature"},
		},
		{
			name:    "hyphenated tags",
			content: "This is #my-tag and #another-tag",
			want:    []string{"my-tag", "another-tag"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractInlineTags(tt.content)

			// Check all expected tags are present
			for _, tag := range tt.want {
				found := false
				for _, g := range got {
					if g == tag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing tag %q in %v", tag, got)
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
		{"Alice Chen", "alice-chen"},
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
