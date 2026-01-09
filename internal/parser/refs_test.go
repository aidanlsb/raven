package parser

import (
	"testing"
)

func TestExtractRefs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string // target_raw values
	}{
		{
			name:    "basic refs",
			content: "Check out [[some/file]] and [[another|Display Text]]",
			want:    []string{"some/file", "another"},
		},
		{
			name:    "refs on multiple lines",
			content: "First [[ref1]] here\nSecond [[ref2]] there",
			want:    []string{"ref1", "ref2"},
		},
		{
			name:    "embedded object ref",
			content: "See [[daily/2025-02-01#standup]] for details",
			want:    []string{"daily/2025-02-01#standup"},
		},
		{
			name: "ignore refs inside fenced code blocks",
			content: "Outside [[ok]]\n\n```go\nthis [[nope]] should not be indexed\n```\n\nAfter [[ok2]]",
			want: []string{"ok", "ok2"},
		},
		{
			name: "ignore refs inside blockquoted fenced code blocks",
			content: "Outside [[ok]]\n\n> ```\n> [[nope]]\n> ```\n\nAfter [[ok2]]",
			want: []string{"ok", "ok2"},
		},
		{
			name: "ignore refs inside tilde fences",
			content: "Outside [[ok]]\n\n~~~\n[[nope]]\n~~~\n\nAfter [[ok2]]",
			want: []string{"ok", "ok2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRefs(tt.content, 1)

			if len(got) != len(tt.want) {
				t.Errorf("got %d refs, want %d", len(got), len(tt.want))
				return
			}

			for i, target := range tt.want {
				if got[i].TargetRaw != target {
					t.Errorf("ref[%d].TargetRaw = %q, want %q", i, got[i].TargetRaw, target)
				}
			}
		})
	}
}

func TestExtractEmbeddedRefs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{
			name:  "array of refs",
			value: "attendees=[[[freya]], [[thor]]]",
			want:  []string{"freya", "thor"},
		},
		{
			name:  "single ref",
			value: "[[people/freya]]",
			want:  []string{"people/freya"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractEmbeddedRefs(tt.value)

			if len(got) != len(tt.want) {
				t.Errorf("got %d refs, want %d", len(got), len(tt.want))
				return
			}

			for i, ref := range tt.want {
				if got[i] != ref {
					t.Errorf("ref[%d] = %q, want %q", i, got[i], ref)
				}
			}
		})
	}
}
