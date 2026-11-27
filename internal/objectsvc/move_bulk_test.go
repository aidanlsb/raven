package objectsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMoveBulkRejectsSectionSources(t *testing.T) {
	t.Parallel()

	rt := testRuntime(t, t.TempDir())
	request := MoveBulkRequest{
		ObjectIDs:      []string{"projects/site", "projects/site#tasks"},
		DestinationDir: "archive/",
	}

	previewReq := request
	previewReq.Preview = true
	if _, _, err := MoveBulk(rt, previewReq); err == nil || !strings.Contains(err.Error(), "does not accept section sources") {
		t.Fatalf("MoveBulk(preview) error = %v, want section-source rejection", err)
	}
	if _, _, err := MoveBulk(rt, request); err == nil || !strings.Contains(err.Error(), "does not accept section sources") {
		t.Fatalf("MoveBulk() error = %v, want section-source rejection", err)
	}
}

func TestApplyMoveBulkRewritesRefsBetweenMovedFiles(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alpha.md", "---\ntype: person\nname: Alpha\n---\n\nSee [[people/beta]].\n").
		WithFile("people/beta.md", "---\ntype: person\nname: Beta\n---\n\nSee [[people/alpha]].\n").
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "people/alpha.md", "people/beta.md")

	_, summary, err := MoveBulk(rt, MoveBulkRequest{
		ObjectIDs:      []string{"people/alpha", "people/beta"},
		DestinationDir: "archive/",
		UpdateRefs:     true,
	})
	if err != nil {
		t.Fatalf("MoveBulk() error = %v", err)
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

	if len(summary.ChangeSet.Moved) != 2 {
		t.Fatalf("ChangeSet.Moved = %#v, want two moves", summary.ChangeSet.Moved)
	}
}
