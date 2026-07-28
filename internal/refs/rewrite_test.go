package refs

import (
	"strings"
	"testing"
)

// mapDecider rewrites an occurrence's base target using a lookup table.
func mapDecider(m map[string]string) Decider {
	return func(occ Occurrence) (string, bool) {
		if newBase, ok := m[occ.Base]; ok {
			return newBase, true
		}
		return "", false
	}
}

func TestRewriteContentWikilinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		mapping map[string]string
		want    string
	}{
		{
			name:    "basic",
			content: "See [[people/tido]] here",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "See [[person/tido]] here",
		},
		{
			name:    "display text preserved",
			content: "Ask [[people/tido|Tido Jones]] about it",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "Ask [[person/tido|Tido Jones]] about it",
		},
		{
			name:    "section fragment preserved",
			content: "See [[people/tido#notes]] for context",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "See [[person/tido#notes]] for context",
		},
		{
			name:    "fragment and display preserved",
			content: "See [[people/tido#notes|Notes]] here",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "See [[person/tido#notes|Notes]] here",
		},
		{
			name:    "multiple occurrences on a line",
			content: "[[people/tido]] and again [[people/tido|Tido]]",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "[[person/tido]] and again [[person/tido|Tido]]",
		},
		{
			name:    "inline code span is not rewritten",
			content: "`[[people/tido]]` but [[people/tido]] is real",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "`[[people/tido]]` but [[person/tido]] is real",
		},
		{
			name:    "non-matching target untouched",
			content: "See [[people/freya]] here",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "See [[people/freya]] here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed := RewriteContent(tt.content, mapDecider(tt.mapping))
			if got != tt.want {
				t.Fatalf("RewriteContent() = %q, want %q", got, tt.want)
			}
			if wantChanged := tt.content != tt.want; changed != wantChanged {
				t.Fatalf("changed = %v, want %v", changed, wantChanged)
			}
		})
	}
}

func TestRewriteContentSkipsFencedCodeBlocks(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"Before [[people/tido]]",
		"```",
		"[[people/tido]]",
		"```",
		"After [[people/tido]]",
		"",
	}, "\n")

	want := strings.Join([]string{
		"Before [[person/tido]]",
		"```",
		"[[people/tido]]",
		"```",
		"After [[person/tido]]",
		"",
	}, "\n")

	got, changed := RewriteContent(content, mapDecider(map[string]string{"people/tido": "person/tido"}))
	if !changed {
		t.Fatal("expected content to change")
	}
	if got != want {
		t.Fatalf("RewriteContent() = %q, want %q", got, want)
	}
}

func TestRewriteContentLeavesMarkdownLinksUntouched(t *testing.T) {
	t.Parallel()

	content := "Read [paper](files/paper.pdf) and [[notes/old]]."
	got, changed := RewriteContent(content, mapDecider(map[string]string{
		"files/paper.pdf": "files/archive/paper.pdf",
		"notes/old":       "notes/new",
	}))
	want := "Read [paper](files/paper.pdf) and [[notes/new]]."
	if !changed || got != want {
		t.Fatalf("RewriteContent() = %q, changed=%v, want %q", got, changed, want)
	}
}

func TestRewriteContentFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		mapping map[string]string
		want    string
	}{
		{
			name:    "bare scalar",
			content: "---\ntype: project\nowner: people/tido\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\ntype: project\nowner: person/tido\n---\n",
		},
		{
			name:    "double quoted scalar",
			content: "---\nowner: \"people/tido\"\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: \"person/tido\"\n---\n",
		},
		{
			name:    "single quoted scalar",
			content: "---\nowner: 'people/tido'\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: 'person/tido'\n---\n",
		},
		{
			name:    "inline array",
			content: "---\nowners: [people/tido, \"people/thor\"]\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowners: [person/tido, \"people/thor\"]\n---\n",
		},
		{
			name:    "block sequence",
			content: "---\nowners:\n  - people/tido\n  - people/thor\n---\n",
			mapping: map[string]string{"people/tido": "person/tido", "people/thor": "person/thor"},
			want:    "---\nowners:\n  - person/tido\n  - person/thor\n---\n",
		},
		{
			name:    "wikilink value",
			content: "---\nowner: [[people/tido]]\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: [[person/tido]]\n---\n",
		},
		{
			name:    "quoted wikilink value",
			content: "---\nowner: \"[[people/tido]]\"\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: \"[[person/tido]]\"\n---\n",
		},
		{
			name:    "trailing comment preserved",
			content: "---\nowner: people/tido # the boss\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: person/tido # the boss\n---\n",
		},
		{
			name:    "fragment preserved in bare scalar",
			content: "---\nowner: people/tido#notes\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: person/tido#notes\n---\n",
		},
		{
			name:    "body is untouched when only frontmatter matches",
			content: "---\nowner: people/tido\n---\nBody mentions people/tido as plain text.\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: person/tido\n---\nBody mentions people/tido as plain text.\n",
		},
		{
			name:    "non-matching value untouched",
			content: "---\nowner: people/freya\n---\n",
			mapping: map[string]string{"people/tido": "person/tido"},
			want:    "---\nowner: people/freya\n---\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := RewriteContent(tt.content, mapDecider(tt.mapping))
			if got != tt.want {
				t.Fatalf("RewriteContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRewriteContentAtLine(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"# Daily",
		"- Check with [[projects/old]] today",
		"- Follow up on [[projects/old|Old]] later",
		"",
	}, "\n")

	got, changed := RewriteContentAtLine(content, 2, mapDecider(map[string]string{"projects/old": "projects/new"}))
	if !changed {
		t.Fatal("expected change")
	}
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[1], "[[projects/new]]") {
		t.Fatalf("line 2 not rewritten:\n%s", got)
	}
	if !strings.Contains(lines[2], "[[projects/old|Old]]") {
		t.Fatalf("line 3 should be untouched:\n%s", got)
	}
}

func TestRewriteContentAtLineFallsBackToWholeContent(t *testing.T) {
	t.Parallel()

	content := "No ref here.\nSee [[projects/old]].\n"
	got, changed := RewriteContentAtLine(content, 1, mapDecider(map[string]string{"projects/old": "projects/new"}))
	if !changed {
		t.Fatal("expected fallback change")
	}
	if !strings.Contains(got, "[[projects/new]]") {
		t.Fatalf("fallback rewrite missing:\n%s", got)
	}
}

func TestRewriteContentAtLineFrontmatterFallback(t *testing.T) {
	t.Parallel()

	content := "---\nowner: people/tido\n---\nBody.\n"
	// Line 2 is a bare frontmatter scalar; the targeted body-line pass finds no
	// wikilink, so it must fall back to the whole-document rewrite.
	got, changed := RewriteContentAtLine(content, 2, mapDecider(map[string]string{"people/tido": "person/tido"}))
	if !changed {
		t.Fatal("expected change via fallback")
	}
	if !strings.Contains(got, "owner: person/tido") {
		t.Fatalf("frontmatter fallback rewrite missing:\n%s", got)
	}
}

func TestRewriteContentNilDecider(t *testing.T) {
	t.Parallel()

	content := "See [[people/tido]]."
	got, changed := RewriteContent(content, nil)
	if changed || got != content {
		t.Fatalf("nil decider must be a no-op, got %q changed=%v", got, changed)
	}
}
