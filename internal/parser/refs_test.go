package parser

import (
	"testing"
)

func TestExtractRefs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		start   int
		want    []string // target_raw values
		wantLn  []int
	}{
		{
			name:    "basic refs",
			content: "Check out [[some/file]] and [[another|Display Text]]",
			want:    []string{"some/file", "another"},
			wantLn:  []int{1, 1},
		},
		{
			name:    "refs on multiple lines",
			content: "First [[ref1]] here\nSecond [[ref2]] there",
			want:    []string{"ref1", "ref2"},
			wantLn:  []int{1, 2},
		},
		{
			name:    "embedded object ref",
			content: "See [[daily/2025-02-01#standup]] for details",
			want:    []string{"daily/2025-02-01#standup"},
			wantLn:  []int{1},
		},
		{
			name:    "start line offset is applied",
			content: "First [[ref1]] here\nSecond [[ref2]] there",
			start:   10,
			want:    []string{"ref1", "ref2"},
			wantLn:  []int{10, 11},
		},
		{
			name:    "ignore refs inside inline code",
			content: "See [[ok]] but not `[[ignored]]` for details",
			want:    []string{"ok"},
			wantLn:  []int{1},
		},
		{
			name:    "ignore refs inside double backticks",
			content: "See [[ok]] and ``[[also ignored `with` backtick]]`` here",
			want:    []string{"ok"},
			wantLn:  []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := tt.start
			if start == 0 {
				start = 1
			}
			got := ExtractRefs(tt.content, start)

			if len(got) != len(tt.want) {
				t.Errorf("got %d refs, want %d", len(got), len(tt.want))
				return
			}

			for i, target := range tt.want {
				if got[i].TargetRaw != target {
					t.Errorf("ref[%d].TargetRaw = %q, want %q", i, got[i].TargetRaw, target)
				}
				if got[i].Line != tt.wantLn[i] {
					t.Errorf("ref[%d].Line = %d, want %d", i, got[i].Line, tt.wantLn[i])
				}
			}
		})
	}
}

func TestExtractEmbeddedRefs(t *testing.T) {
	t.Parallel()
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
