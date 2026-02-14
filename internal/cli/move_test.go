package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/resolver"
)

func TestUpdateReferenceUpdatesFragments(t *testing.T) {
	vaultPath := t.TempDir()

	// Create a source file that contains references to an object and its sections.
	sourceRel := filepath.Join("daily", "2026-01-01.md")
	sourceAbs := filepath.Join(vaultPath, sourceRel)
	if err := os.MkdirAll(filepath.Dir(sourceAbs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	before := strings.Join([]string{
		"# Daily",
		"",
		"See [[projects/old]] and [[projects/old|Old Project]].",
		"Also see [[projects/old#tasks]] and [[projects/old#tasks|Task list]].",
		"",
	}, "\n")
	if err := os.WriteFile(sourceAbs, []byte(before), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := updateReference(vaultPath, &config.VaultConfig{}, "daily/2026-01-01", "projects/old", "projects/new"); err != nil {
		t.Fatalf("updateReference: %v", err)
	}

	afterBytes, err := os.ReadFile(sourceAbs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	after := string(afterBytes)

	// All old forms should be gone.
	if strings.Contains(after, "[[projects/old") {
		t.Fatalf("expected old refs to be removed, got:\n%s", after)
	}

	// And new forms should exist.
	wantSnippets := []string{
		"[[projects/new]]",
		"[[projects/new|Old Project]]",
		"[[projects/new#tasks]]",
		"[[projects/new#tasks|Task list]]",
	}
	for _, s := range wantSnippets {
		if !strings.Contains(after, s) {
			t.Fatalf("expected %q in updated content, got:\n%s", s, after)
		}
	}
}

func TestChooseReplacementRefBaseKeepsShortWhenUnique(t *testing.T) {
	ids := []string{
		"projects/website",
		"projects/new-site",
		"people/freya",
	}
	res := resolver.New(ids, resolver.Options{})

	aliasSlugToID := map[string]string{}
	got := chooseReplacementRefBase("website", "projects/website", "projects/new-site", aliasSlugToID, res)
	if got != "new-site" {
		t.Fatalf("expected short ref %q, got %q", "new-site", got)
	}
}

func TestChooseReplacementRefBaseFallsBackToFullWhenAmbiguous(t *testing.T) {
	ids := []string{
		"projects/website",
		"projects/new-site",
		"notes/new-site", // makes [[new-site]] ambiguous
	}
	res := resolver.New(ids, resolver.Options{})

	aliasSlugToID := map[string]string{}
	got := chooseReplacementRefBase("website", "projects/website", "projects/new-site", aliasSlugToID, res)
	if got != "projects/new-site" {
		t.Fatalf("expected fallback to full ref %q, got %q", "projects/new-site", got)
	}
}

func TestUpdateReferenceUpdatesRefsOnTraitLines(t *testing.T) {
	vaultPath := t.TempDir()

	// Create a source file that contains references on trait lines.
	sourceRel := filepath.Join("daily", "2026-01-02.md")
	sourceAbs := filepath.Join(vaultPath, sourceRel)
	if err := os.MkdirAll(filepath.Dir(sourceAbs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	before := strings.Join([]string{
		"# Daily",
		"",
		"- @todo(done) Check with [[projects/old]] about this",
		"- @todo Follow up on [[projects/old|Old Project]] status",
		"- @highlight Important note about [[projects/old#tasks]]",
		"- Regular text with [[projects/old]]",
		"",
	}, "\n")
	if err := os.WriteFile(sourceAbs, []byte(before), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := updateReference(vaultPath, &config.VaultConfig{}, "daily/2026-01-02", "projects/old", "projects/new"); err != nil {
		t.Fatalf("updateReference: %v", err)
	}

	afterBytes, err := os.ReadFile(sourceAbs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	after := string(afterBytes)

	// All old forms should be gone.
	if strings.Contains(after, "[[projects/old") {
		t.Fatalf("expected old refs to be removed, got:\n%s", after)
	}

	// And new forms should exist - including those on trait lines.
	wantSnippets := []string{
		"@todo(done) Check with [[projects/new]]",
		"@todo Follow up on [[projects/new|Old Project]]",
		"@highlight Important note about [[projects/new#tasks]]",
		"Regular text with [[projects/new]]",
	}
	for _, s := range wantSnippets {
		if !strings.Contains(after, s) {
			t.Fatalf("expected %q in updated content, got:\n%s", s, after)
		}
	}
}

func TestUpdateReferenceAtLineUpdatesRefsOnTraitLines(t *testing.T) {
	vaultPath := t.TempDir()

	// Create a source file that contains references on trait lines.
	sourceRel := filepath.Join("daily", "2026-01-03.md")
	sourceAbs := filepath.Join(vaultPath, sourceRel)
	if err := os.MkdirAll(filepath.Dir(sourceAbs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	before := strings.Join([]string{
		"# Daily", // Line 1
		"",        // Line 2
		"- @todo(done) Check with [[projects/old]] about this",     // Line 3
		"- @todo Follow up on [[projects/old|Old Project]] status", // Line 4
		"- @highlight Important note about [[projects/old#tasks]]", // Line 5
		"- Regular text with [[projects/old]]",                     // Line 6
		"",                                                         // Line 7
	}, "\n")
	if err := os.WriteFile(sourceAbs, []byte(before), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Update ref on line 3 (trait line)
	if err := updateReferenceAtLine(vaultPath, &config.VaultConfig{}, "daily/2026-01-03", 3, "projects/old", "projects/new"); err != nil {
		t.Fatalf("updateReferenceAtLine line 3: %v", err)
	}

	afterBytes, err := os.ReadFile(sourceAbs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	after := string(afterBytes)

	// Line 3 should be updated
	if !strings.Contains(after, "@todo(done) Check with [[projects/new]]") {
		t.Fatalf("expected line 3 to be updated, got:\n%s", after)
	}

	// Lines 4, 5, 6 should still have old refs (we only updated line 3)
	lines := strings.Split(after, "\n")
	if !strings.Contains(lines[3], "[[projects/old|Old Project]]") {
		t.Fatalf("expected line 4 to still have old ref, got: %s", lines[3])
	}
}

func TestReplaceAllRefVariants(t *testing.T) {
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
			name:       "basic ref",
			content:    "See [[people/tido]] for details",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "See [[person/tido]] for details",
		},
		{
			name:       "ref with display text",
			content:    "Ask [[people/tido|Tido]] about this",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "Ask [[person/tido|Tido]] about this",
		},
		{
			name:       "ref with fragment",
			content:    "See [[people/tido#notes]] for context",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "See [[person/tido#notes]] for context",
		},
		{
			name:       "ref with directory prefix",
			content:    "See [[objects/people/tido]] for details",
			oldID:      "people/tido",
			oldBase:    "objects/people/tido",
			newRef:     "person/tido",
			objectRoot: "objects/",
			pageRoot:   "",
			want:       "See [[person/tido]] for details",
		},
		{
			name:       "multiple variants on same line",
			content:    "[[people/tido]] and [[objects/people/tido|Tido]]",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "objects/",
			pageRoot:   "",
			want:       "[[person/tido]] and [[person/tido|Tido]]",
		},
		{
			name:       "ref on trait line",
			content:    "- @todo(done) Check with [[people/tido]] about this",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "- @todo(done) Check with [[person/tido]] about this",
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
			name:       "short wikilink ref",
			content:    "See [[tido]] for details",
			oldID:      "people/tido",
			oldBase:    "tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "See [[person/tido]] for details",
		},
		{
			name:       "bare frontmatter ref scalar",
			content:    "---\ntype: project\nowner: people/tido\n---\n",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "---\ntype: project\nowner: person/tido\n---\n",
		},
		{
			name:       "bare frontmatter ref in inline array",
			content:    "---\ntype: project\nowners: [people/tido, \"people/thor\"]\n---\n",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "---\ntype: project\nowners: [person/tido, \"people/thor\"]\n---\n",
		},
		{
			name:       "bare type declaration ref scalar",
			content:    "# Notes\n::project(owner=people/tido)\n",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "# Notes\n::project(owner=person/tido)\n",
		},
		{
			name:       "bare type declaration ref in inline array",
			content:    "# Notes\n::project(owners=[people/tido, \"people/thor\"])\n",
			oldID:      "people/tido",
			oldBase:    "people/tido",
			newRef:     "person/tido",
			objectRoot: "",
			pageRoot:   "",
			want:       "# Notes\n::project(owners=[person/tido, \"people/thor\"])\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceAllRefVariants(tt.content, tt.oldID, tt.oldBase, tt.newRef, tt.objectRoot, tt.pageRoot)
			if got != tt.want {
				t.Errorf("replaceAllRefVariants() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateReferenceAtLineWithSectionSourceID(t *testing.T) {
	// This tests the real-world case where the backlink's source_id includes a section
	// fragment (e.g., "daily/2026-01-05#meeting-notes"), which is what happens when
	// a ref appears within an embedded object/section.
	vaultPath := t.TempDir()

	// Create a source file with sections containing trait lines with refs.
	sourceRel := filepath.Join("daily", "2026-01-05.md")
	sourceAbs := filepath.Join(vaultPath, sourceRel)
	if err := os.MkdirAll(filepath.Dir(sourceAbs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	before := strings.Join([]string{
		"---",             // Line 1
		"type: date",      // Line 2
		"---",             // Line 3
		"",                // Line 4
		"# meeting-notes", // Line 5
		"::meeting",       // Line 6
		"- @todo(done) Check the [[projects/old]] status",   // Line 7
		"- @todo Follow up on [[projects/old|Old Project]]", // Line 8
		"", // Line 9
	}, "\n")
	if err := os.WriteFile(sourceAbs, []byte(before), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The backlink's source_id includes the section: "daily/2026-01-05#meeting-notes"
	// This should still correctly resolve to the file and update line 7.
	err := updateReferenceAtLine(vaultPath, &config.VaultConfig{}, "daily/2026-01-05#meeting-notes", 7, "projects/old", "projects/new")
	if err != nil {
		t.Fatalf("updateReferenceAtLine with section ID: %v", err)
	}

	afterBytes, err := os.ReadFile(sourceAbs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	after := string(afterBytes)

	// Line 7 should be updated
	if !strings.Contains(after, "@todo(done) Check the [[projects/new]] status") {
		t.Fatalf("expected line 7 to be updated, got:\n%s", after)
	}

	// Line 8 should still have old ref (we only updated line 7)
	if !strings.Contains(after, "[[projects/old|Old Project]]") {
		t.Fatalf("expected line 8 to still have old ref, got:\n%s", after)
	}
}

func TestChooseReplacementRefBasePreservesAlias(t *testing.T) {
	ids := []string{
		"projects/website",
		"projects/new-site",
	}
	aliases := map[string]string{
		"goddess": "projects/website",
	}
	res := resolver.New(ids, resolver.Options{Aliases: aliases})

	aliasSlugToID := map[string]string{
		pages.SlugifyPath("goddess"): "projects/website",
	}
	got := chooseReplacementRefBase("goddess", "projects/website", "projects/new-site", aliasSlugToID, res)
	if got != "goddess" {
		t.Fatalf("expected alias to remain %q, got %q", "goddess", got)
	}
}
