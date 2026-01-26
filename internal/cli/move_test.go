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
	aliases := map[string]string{}
	res := resolver.NewWithAliases(ids, aliases, "daily")

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
	aliases := map[string]string{}
	res := resolver.NewWithAliases(ids, aliases, "daily")

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
		"# Daily",                                                    // Line 1
		"",                                                           // Line 2
		"- @todo(done) Check with [[projects/old]] about this",       // Line 3
		"- @todo Follow up on [[projects/old|Old Project]] status",   // Line 4
		"- @highlight Important note about [[projects/old#tasks]]",   // Line 5
		"- Regular text with [[projects/old]]",                       // Line 6
		"",                                                           // Line 7
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
		"---",                                                                    // Line 1
		"type: date",                                                             // Line 2
		"---",                                                                    // Line 3
		"",                                                                       // Line 4
		"# meeting-notes",                                                        // Line 5
		"::meeting",                                                              // Line 6
		"- @todo(done) Check the [[projects/old]] status",                        // Line 7
		"- @todo Follow up on [[projects/old|Old Project]]",                      // Line 8
		"",                                                                       // Line 9
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
	res := resolver.NewWithAliases(ids, aliases, "daily")

	aliasSlugToID := map[string]string{
		pages.SlugifyPath("goddess"): "projects/website",
	}
	got := chooseReplacementRefBase("goddess", "projects/website", "projects/new-site", aliasSlugToID, res)
	if got != "goddess" {
		t.Fatalf("expected alias to remain %q, got %q", "goddess", got)
	}
}
