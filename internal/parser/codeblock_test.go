package parser

import (
	"testing"
)

func TestFenceState(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		wantOpen []bool // For each line, is it inside fence after processing?
	}{
		{
			name: "simple fenced block",
			lines: []string{
				"before",
				"```python",
				"@decorator",
				"def foo():",
				"```",
				"after",
			},
			wantOpen: []bool{false, true, true, true, false, false},
		},
		{
			name: "tilde fence",
			lines: []string{
				"before",
				"~~~",
				"@trait inside",
				"~~~",
				"after",
			},
			wantOpen: []bool{false, true, true, false, false},
		},
		{
			name: "nested backticks require more",
			lines: []string{
				"before",
				"````",
				"```",
				"still inside",
				"````",
				"after",
			},
			wantOpen: []bool{false, true, true, true, false, false},
		},
		{
			name: "blockquote with fence",
			lines: []string{
				"> ```python",
				"> @decorator",
				"> ```",
				"outside",
			},
			wantOpen: []bool{true, true, false, false},
		},
		{
			name: "list item with fence",
			lines: []string{
				"- ```python",
				"  @decorator",
				"  ```",
				"after",
			},
			wantOpen: []bool{true, true, false, false},
		},
		{
			name: "nested list with fence",
			lines: []string{
				"- Item",
				"  - ```",
				"    @trait",
				"    ```",
				"  - after",
			},
			wantOpen: []bool{false, true, true, false, false},
		},
		{
			name: "asterisk list marker",
			lines: []string{
				"* ```",
				"  @trait",
				"* ```",
				"after",
			},
			wantOpen: []bool{true, true, false, false},
		},
		{
			name: "plus list marker",
			lines: []string{
				"+ ```",
				"  @trait",
				"+ ```",
				"after",
			},
			wantOpen: []bool{true, true, false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := FenceState{}
			for i, line := range tt.lines {
				state.UpdateFenceState(line)
				if state.InFence != tt.wantOpen[i] {
					t.Errorf("line %d %q: InFence = %v, want %v",
						i, line, state.InFence, tt.wantOpen[i])
				}
			}
		})
	}
}

func TestRemoveInlineCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple inline code",
			input: "text `@trait` more",
			want:  "text          more",
		},
		{
			name:  "double backticks",
			input: "text ``@trait with `backtick` inside`` more",
			want:  "text                                   more",
		},
		{
			name:  "multiple inline code spans",
			input: "`@foo` and `@bar` text",
			want:  "       and        text",
		},
		{
			name:  "no inline code",
			input: "@trait without code",
			want:  "@trait without code",
		},
		{
			name:  "ref inside inline code",
			input: "see `[[link]]` for details",
			want:  "see            for details",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveInlineCode(tt.input)
			if got != tt.want {
				t.Errorf("RemoveInlineCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestListMarkerNotConfusedWithFence(t *testing.T) {
	// A list item with content should NOT be detected as a fence
	tests := []struct {
		line      string
		wantFence bool
	}{
		{"* This is just a list item", false},
		{"- @todo regular trait", false},
		{"+ some content", false},
		{"* ```", true},       // This IS a fence
		{"- ```python", true}, // This IS a fence
	}

	for _, tt := range tests {
		state := FenceState{}
		isFence := state.UpdateFenceState(tt.line)
		if isFence != tt.wantFence {
			t.Errorf("UpdateFenceState(%q) = %v, want %v", tt.line, isFence, tt.wantFence)
		}
	}
}
