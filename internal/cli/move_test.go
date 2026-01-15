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
