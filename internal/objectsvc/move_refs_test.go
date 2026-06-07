package objectsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/resolver"
)

func TestReplaceAllRefVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		oldID      string
		oldBase    string
		newRef     string
		objectRoot string
		pageRoot   string
		want       string
	}{
		{
			name:    "basic ref",
			content: "See [[people/tido]] for details",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "See [[person/tido]] for details",
		},
		{
			name:    "ref with display text",
			content: "Ask [[people/tido|Tido]] about this",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "Ask [[person/tido|Tido]] about this",
		},
		{
			name:    "ref with fragment",
			content: "See [[people/tido#notes]] for context",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "See [[person/tido#notes]] for context",
		},
		{
			name:       "ref with directory prefix",
			content:    "See [[objects/people/tido]] for details",
			oldID:      "people/tido",
			oldBase:    "objects/people/tido",
			newRef:     "person/tido",
			objectRoot: "objects/",
			want:       "See [[person/tido]] for details",
		},
		{
			name:       "multiple variants on same line",
			content:    "[[people/tido]] and [[objects/people/tido|Tido]]",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "objects/",
			want:       "[[person/tido]] and [[person/tido|Tido]]",
		},
		{
			name:    "ref on trait line",
			content: "- @todo(done) Check with [[people/tido]] about this",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "- @todo(done) Check with [[person/tido]] about this",
		},
		{
			name:       "ref with pages root",
			content:    "See [[pages/my-note]] for details",
			oldID:      "my-note",
			oldBase:    "pages/my-note",
			newRef:     "notes/my-note",
			objectRoot: "objects/",
			pageRoot:   "pages/",
			want:       "See [[notes/my-note]] for details",
		},
		{
			name:    "short wikilink ref",
			content: "See [[tido]] for details",
			oldID:   "people/tido",
			oldBase: "tido",
			newRef:  "person/tido",
			want:    "See [[person/tido]] for details",
		},
		{
			name:    "markdown link destination",
			content: "Read [paper](assets/pdfs/paper.pdf).",
			oldID:   "assets/pdfs/paper.pdf",
			oldBase: "assets/pdfs/paper.pdf",
			newRef:  "assets/archive/paper.pdf",
			want:    "Read [paper](assets/archive/paper.pdf).",
		},
		{
			name:    "bare frontmatter ref scalar",
			content: "---\ntype: project\nowner: people/tido\n---\n",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "---\ntype: project\nowner: person/tido\n---\n",
		},
		{
			name:    "bare frontmatter ref in inline array",
			content: "---\ntype: project\nowners: [people/tido, \"people/thor\"]\n---\n",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "---\ntype: project\nowners: [person/tido, \"people/thor\"]\n---\n",
		},
		{
			name:    "bare type declaration ref scalar",
			content: "# Notes\n::project(owner=people/tido)\n",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "# Notes\n::project(owner=person/tido)\n",
		},
		{
			name:    "bare type declaration ref in inline array",
			content: "# Notes\n::project(owners=[people/tido, \"people/thor\"])\n",
			oldID:   "people/tido",
			oldBase: "people/tido",
			newRef:  "person/tido",
			want:    "# Notes\n::project(owners=[person/tido, \"people/thor\"])\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ReplaceAllRefVariants(tt.content, tt.oldID, tt.oldBase, tt.newRef, tt.objectRoot, tt.pageRoot)
			if got != tt.want {
				t.Fatalf("ReplaceAllRefVariants() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyAllRefVariantsAtLine(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"# Daily",
		"- @todo(done) Check with [[projects/old]] about this",
		"- @todo Follow up on [[projects/old|Old Project]] status",
		"",
	}, "\n")

	got := ApplyAllRefVariantsAtLine(content, 2, "projects/old", "projects/old", "projects/new", "", "")
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[1], "[[projects/new]]") {
		t.Fatalf("expected line 2 to be updated, got:\n%s", got)
	}
	if !strings.Contains(lines[2], "[[projects/old|Old Project]]") {
		t.Fatalf("expected line 3 to remain unchanged, got:\n%s", got)
	}
}

func TestApplyAllRefVariantsAtLineFallsBackWhenLineHasNoMatch(t *testing.T) {
	t.Parallel()

	content := "No ref here.\nSee [[projects/old]].\n"
	got := ApplyAllRefVariantsAtLine(content, 1, "projects/old", "projects/old", "projects/new", "", "")
	if !strings.Contains(got, "[[projects/new]]") {
		t.Fatalf("expected fallback full-content rewrite, got:\n%s", got)
	}
}

func TestChooseReplacementRefBase(t *testing.T) {
	t.Parallel()

	t.Run("keeps short when unique", func(t *testing.T) {
		t.Parallel()

		ids := []string{"projects/website", "projects/new-site", "people/freya"}
		res := resolver.New(ids, resolver.Options{})
		got := ChooseReplacementRefBase("website", "projects/website", "projects/new-site", map[string]string{}, res)
		if got != "new-site" {
			t.Fatalf("expected short ref %q, got %q", "new-site", got)
		}
	})

	t.Run("falls back to full when ambiguous", func(t *testing.T) {
		t.Parallel()

		ids := []string{"projects/website", "projects/new-site", "notes/new-site"}
		res := resolver.New(ids, resolver.Options{})
		got := ChooseReplacementRefBase("website", "projects/website", "projects/new-site", map[string]string{}, res)
		if got != "projects/new-site" {
			t.Fatalf("expected fallback to full ref %q, got %q", "projects/new-site", got)
		}
	})

	t.Run("preserves alias", func(t *testing.T) {
		t.Parallel()

		ids := []string{"projects/website", "projects/new-site"}
		aliases := map[string]string{"goddess": "projects/website"}
		res := resolver.New(ids, resolver.Options{Aliases: aliases})
		aliasSlugToID := map[string]string{
			pages.SlugifyPath("goddess"): "projects/website",
		}
		got := ChooseReplacementRefBase("goddess", "projects/website", "projects/new-site", aliasSlugToID, res)
		if got != "goddess" {
			t.Fatalf("expected alias to remain %q, got %q", "goddess", got)
		}
	})
}
