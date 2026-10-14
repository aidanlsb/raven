package parser

import (
	"strings"
	"testing"
)

func TestExtractWikilinkSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantTexts []string
		wantTgt   []string
	}{
		{
			name: "frontmatter and body",
			content: `---
type: project
owner: "[[people/alice]]"
---
See [[projects/raven]] and [[people/alice|Alice]].
`,
			wantTexts: []string{"[[people/alice]]", "[[projects/raven]]", "[[people/alice|Alice]]"},
			wantTgt:   []string{"people/alice", "projects/raven", "people/alice"},
		},
		{
			name:      "multibyte prefix",
			content:   "café 😀 [[people/alice]]\n",
			wantTexts: []string{"[[people/alice]]"},
			wantTgt:   []string{"people/alice"},
		},
		{
			name:      "skips fenced code",
			content:   "See [[real]]\n\n```\n[[fake]]\n```\n\nAnd [[also-real]].\n",
			wantTexts: []string{"[[real]]", "[[also-real]]"},
			wantTgt:   []string{"real", "also-real"},
		},
		{
			name:      "skips inline code",
			content:   "See [[ok]] but not `[[ignored]]` here.\n",
			wantTexts: []string{"[[ok]]"},
			wantTgt:   []string{"ok"},
		},
		{
			name:      "skips indented code",
			content:   "See [[real]]\n\n    [[indented]]\n\nDone.\n",
			wantTexts: []string{"[[real]]"},
			wantTgt:   []string{"real"},
		},
		{
			name:      "section slice without frontmatter",
			content:   "# Tasks\n\nShip [[people/alice]].\n",
			wantTexts: []string{"[[people/alice]]"},
			wantTgt:   []string{"people/alice"},
		},
		{
			name:      "no links",
			content:   "# Notes\n\nNo wikilinks here.\n",
			wantTexts: nil,
			wantTgt:   nil,
		},
		{
			name:      "two identical targets on one line",
			content:   "See [[dup]] then [[dup]] again.\n",
			wantTexts: []string{"[[dup]]", "[[dup]]"},
			wantTgt:   []string{"dup", "dup"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spans, err := ExtractWikilinkSpans(tt.content)
			if err != nil {
				t.Fatalf("ExtractWikilinkSpans: %v", err)
			}
			if len(spans) != len(tt.wantTexts) {
				t.Fatalf("got %d spans %#v, want %d", len(spans), spans, len(tt.wantTexts))
			}
			for i, span := range spans {
				if span.Literal != tt.wantTexts[i] {
					t.Errorf("span[%d].Literal = %q, want %q", i, span.Literal, tt.wantTexts[i])
				}
				if span.Target != tt.wantTgt[i] {
					t.Errorf("span[%d].Target = %q, want %q", i, span.Target, tt.wantTgt[i])
				}
				got := tt.content[span.ByteStart:span.ByteEnd]
				if got != span.Literal {
					t.Errorf("span[%d] slice = %q, want literal %q", i, got, span.Literal)
				}
			}
		})
	}
}

func TestExtractWikilinkSpansMultibyteOffsets(t *testing.T) {
	t.Parallel()

	content := "café [[x]]"
	spans, err := ExtractWikilinkSpans(content)
	if err != nil {
		t.Fatalf("ExtractWikilinkSpans: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %#v, want 1", spans)
	}
	// "café " is c,a,f,é(2 bytes),space = 6 bytes; "café" is 5 runes.
	if spans[0].ByteStart != 6 || spans[0].ByteEnd != 11 {
		t.Fatalf("byte range = [%d, %d), want [6, 11)", spans[0].ByteStart, spans[0].ByteEnd)
	}
	if content[spans[0].ByteStart:spans[0].ByteEnd] != "[[x]]" {
		t.Fatalf("slice = %q", content[spans[0].ByteStart:spans[0].ByteEnd])
	}
}

func TestExtractWikilinkSpansFrontmatterDoesNotShiftBody(t *testing.T) {
	t.Parallel()

	bodyLink := "[[projects/raven]]"
	content := "---\ntype: note\n---\nSee " + bodyLink + ".\n"
	spans, err := ExtractWikilinkSpans(content)
	if err != nil {
		t.Fatalf("ExtractWikilinkSpans: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %#v, want 1", spans)
	}
	wantStart := strings.Index(content, bodyLink)
	if wantStart < 0 {
		t.Fatal("missing body link in fixture")
	}
	if spans[0].ByteStart != wantStart || spans[0].ByteEnd != wantStart+len(bodyLink) {
		t.Fatalf("byte range = [%d, %d), want [%d, %d)", spans[0].ByteStart, spans[0].ByteEnd, wantStart, wantStart+len(bodyLink))
	}
}
