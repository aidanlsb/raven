package objectsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMoveBulkRejectsSectionSources(t *testing.T) {
	t.Parallel()

	request := MoveBulkRequest{
		VaultPath:      t.TempDir(),
		VaultConfig:    config.DefaultVaultConfig(),
		ObjectIDs:      []string{"projects/site", "projects/site#tasks"},
		DestinationDir: "archive/",
	}

	if _, err := PreviewMoveBulk(request); err == nil || !strings.Contains(err.Error(), "does not accept section sources") {
		t.Fatalf("PreviewMoveBulk() error = %v, want section-source rejection", err)
	}
	if _, err := ApplyMoveBulk(request); err == nil || !strings.Contains(err.Error(), "does not accept section sources") {
		t.Fatalf("ApplyMoveBulk() error = %v, want section-source rejection", err)
	}
}

func TestApplyMoveBulkRewritesRefsBetweenMovedFiles(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alpha.md", "---\ntype: person\nname: Alpha\n---\n\nSee [[people/beta]].\n").
		WithFile("people/beta.md", "---\ntype: person\nname: Beta\n---\n\nSee [[people/alpha]].\n").
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/alpha.md", "people/beta.md")

	summary, err := ApplyMoveBulk(MoveBulkRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		ObjectIDs:      []string{"people/alpha", "people/beta"},
		DestinationDir: "archive/",
		UpdateRefs:     true,
	})
	if err != nil {
		t.Fatalf("ApplyMoveBulk() error = %v", err)
	}
	if summary.Moved != 2 || summary.Errors != 0 {
		t.Fatalf("summary = %#v, want two successful moves", summary)
	}

	alpha := v.ReadFile("archive/alpha.md")
	if strings.Contains(alpha, "[[people/beta]]") || !strings.Contains(alpha, "[[archive/beta]]") {
		t.Fatalf("alpha ref was not rewritten to moved beta:\n%s", alpha)
	}
	beta := v.ReadFile("archive/beta.md")
	if strings.Contains(beta, "[[people/alpha]]") || !strings.Contains(beta, "[[archive/alpha]]") {
		t.Fatalf("beta ref was not rewritten to moved alpha:\n%s", beta)
	}

	for _, relPath := range summary.ChangeSet.IndexPaths() {
		if strings.HasPrefix(relPath, "people/") {
			t.Fatalf("IndexPaths() contains moved-away path %q: %#v", relPath, summary.ChangeSet.IndexPaths())
		}
	}
}
