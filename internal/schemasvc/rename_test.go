package schemasvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

const eventTypeSchema = `version: 2
types:
  event:
    fields:
      title: { type: string }
traits: {}
`

const eventProjectSchema = `version: 2
types:
  event:
    default_path: events/
    fields:
      title: { type: string }
  project:
    default_path: projects/
    fields:
      kickoff:
        type: ref
        target: event
traits: {}
`

func countTypeChanges(changes []TypeRenameChange, changeType string) int {
	n := 0
	for _, c := range changes {
		if c.ChangeType == changeType {
			n++
		}
	}
	return n
}

func hasTypeChangeForFile(changes []TypeRenameChange, changeType, filePath string) bool {
	for _, c := range changes {
		if c.ChangeType == changeType && c.FilePath == filePath {
			return true
		}
	}
	return false
}

// TestRenameType_PreviewCountMatchesApplyForQuotedTypes is the core anti-drift
// regression test. The old apply path used a whole-file regex
// (`^type:\s*event\s*$`) that silently skipped YAML the parser accepts, such as
// `type: "event"`. Preview (parser-based) would therefore count files that apply
// never touched. This test fails on that old behavior and passes once preview
// and apply share a single plan built with structured frontmatter editing.
func TestRenameType_PreviewCountMatchesApplyForQuotedTypes(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(eventTypeSchema).
		WithFile("notes/quoted.md", "---\ntype: \"event\"\ntitle: Quoted\n---\n# Quoted\n").
		WithFile("notes/single.md", "---\ntype: 'event'\ntitle: Single\n---\n# Single\n").
		WithFile("notes/plain.md", "---\ntype: event\ntitle: Plain\n---\n# Plain\n\ntype: event\n").
		WithFile("notes/other.md", "---\ntype: page\ntitle: Other\n---\n# Other\n").
		Build()

	preview, err := RenameType(RenameTypeRequest{VaultPath: v.Path, OldName: "event", NewName: "meeting"})
	if err != nil {
		t.Fatalf("preview RenameType: %v", err)
	}
	if !preview.Preview {
		t.Fatalf("expected preview result")
	}
	if got := countTypeChanges(preview.Changes, "frontmatter"); got != 3 {
		t.Fatalf("expected preview to count 3 frontmatter changes (quoted, single, plain), got %d\n%+v", got, preview.Changes)
	}

	apply, err := RenameType(RenameTypeRequest{VaultPath: v.Path, OldName: "event", NewName: "meeting", Confirm: true})
	if err != nil {
		t.Fatalf("apply RenameType: %v", err)
	}

	// With no default-path plan, every previewed change maps to exactly one
	// applied mutation, so the two tallies must be identical.
	if preview.TotalChanges != apply.ChangesApplied {
		t.Fatalf("preview total (%d) != apply applied (%d): counts drifted", preview.TotalChanges, apply.ChangesApplied)
	}

	// Quoted and single-quoted values must be renamed, not silently skipped.
	for _, f := range []string{"notes/quoted.md", "notes/single.md"} {
		content := v.ReadFile(f)
		if !strings.Contains(content, "type: meeting") {
			t.Fatalf("expected %s frontmatter to become meeting, got:\n%s", f, content)
		}
		if strings.Contains(content, "event") {
			t.Fatalf("expected %s to no longer mention event, got:\n%s", f, content)
		}
	}

	// The plain file's frontmatter is renamed, but a body occurrence of
	// `type: event` must be left untouched (structured editing only touches the
	// frontmatter block, unlike the old whole-file regex).
	plain := v.ReadFile("notes/plain.md")
	if !strings.Contains(plain, "type: meeting") {
		t.Fatalf("expected plain frontmatter to become meeting, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\ntype: event\n") {
		t.Fatalf("expected plain body occurrence of `type: event` to be preserved, got:\n%s", plain)
	}

	// A file of a different type is never counted or modified.
	if got := v.ReadFile("notes/other.md"); !strings.Contains(got, "type: page") {
		t.Fatalf("expected other.md to stay type page, got:\n%s", got)
	}
}

// TestRenameType_PreviewListsReferenceUpdatesFromDirectoryMove verifies that the
// reference rewrites triggered by a default-path directory move appear in the
// preview change list (they previously only happened at apply time and were
// invisible to preview).
func TestRenameType_PreviewListsReferenceUpdatesFromDirectoryMove(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(eventProjectSchema).
		WithFile("events/kickoff.md", "---\ntype: event\ntitle: Kickoff\n---\n# Kickoff\n").
		WithFile("projects/roadmap.md", "---\ntype: project\nkickoff: events/kickoff\n---\n# Roadmap\n\nKickoff: [[events/kickoff]]\n").
		Build()

	beforeRoadmap := v.ReadFile("projects/roadmap.md")

	preview, err := RenameType(RenameTypeRequest{VaultPath: v.Path, OldName: "event", NewName: "meeting"})
	if err != nil {
		t.Fatalf("preview RenameType: %v", err)
	}
	if !preview.DefaultPathRenameAvailable {
		t.Fatalf("expected default-path rename to be available")
	}
	if got := countTypeChanges(preview.OptionalChanges, "reference_update"); got < 1 {
		t.Fatalf("expected preview optional changes to list reference updates, got %d\n%+v", got, preview.OptionalChanges)
	}
	if !hasTypeChangeForFile(preview.OptionalChanges, "reference_update", "projects/roadmap.md") {
		t.Fatalf("expected reference_update change for projects/roadmap.md, got:\n%+v", preview.OptionalChanges)
	}
	if preview.OptionalTotalChanges != len(preview.OptionalChanges) {
		t.Fatalf("optional_total_changes (%d) != len(optional_changes) (%d)", preview.OptionalTotalChanges, len(preview.OptionalChanges))
	}

	// Preview must not touch any files.
	if got := v.ReadFile("projects/roadmap.md"); got != beforeRoadmap {
		t.Fatalf("expected roadmap unchanged during preview")
	}
	v.AssertFileExists("events/kickoff.md")
	v.AssertFileNotExists("meetings/kickoff.md")
}

// TestRenameType_DefaultPathRenameHandlesQuotedTypeAndRefs exercises the full
// flow: a file whose frontmatter uses a quoted type is renamed, moved to the new
// default directory, and references to it are rewritten. The quoted-type file
// would have been moved with a stale `type: "event"` under the old regex apply.
func TestRenameType_DefaultPathRenameHandlesQuotedTypeAndRefs(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(eventProjectSchema).
		WithFile("events/kickoff.md", "---\ntype: \"event\"\ntitle: Kickoff\n---\n# Kickoff\n").
		WithFile("projects/roadmap.md", "---\ntype: project\nkickoff: events/kickoff\n---\n# Roadmap\n\nKickoff: [[events/kickoff]]\n").
		Build()

	preview, err := RenameType(RenameTypeRequest{VaultPath: v.Path, OldName: "event", NewName: "meeting"})
	if err != nil {
		t.Fatalf("preview RenameType: %v", err)
	}
	if got := countTypeChanges(preview.Changes, "frontmatter"); got != 1 {
		t.Fatalf("expected 1 frontmatter change in preview for quoted type, got %d\n%+v", got, preview.Changes)
	}
	if preview.FilesToMove != 1 {
		t.Fatalf("expected files_to_move=1, got %d", preview.FilesToMove)
	}

	res, err := RenameType(RenameTypeRequest{
		VaultPath:         v.Path,
		OldName:           "event",
		NewName:           "meeting",
		Confirm:           true,
		RenameDefaultPath: true,
	})
	if err != nil {
		t.Fatalf("apply RenameType: %v", err)
	}
	if !res.DefaultPathRenamed {
		t.Fatalf("expected default_path_renamed=true")
	}
	if res.FilesMoved != 1 {
		t.Fatalf("expected files_moved=1, got %d", res.FilesMoved)
	}
	if res.ReferenceFilesUpdated != 1 {
		t.Fatalf("expected reference_files_updated=1, got %d", res.ReferenceFilesUpdated)
	}

	v.AssertFileExists("meetings/kickoff.md")
	v.AssertFileNotExists("events/kickoff.md")

	// The moved file carries the renamed type even though it was quoted.
	moved := v.ReadFile("meetings/kickoff.md")
	if !strings.Contains(moved, "type: meeting") {
		t.Fatalf("expected moved file to have type: meeting, got:\n%s", moved)
	}
	if strings.Contains(moved, "event") {
		t.Fatalf("expected moved file to no longer mention event, got:\n%s", moved)
	}

	// References to the moved object are rewritten to the new path.
	v.AssertFileContains("projects/roadmap.md", "kickoff: meetings/kickoff")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/kickoff]]")
	v.AssertFileNotContains("projects/roadmap.md", "events/kickoff")

	// schema.yaml reflects the rename plus the default-path move.
	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meetings/")
	v.AssertFileContains("schema.yaml", "target: meeting")
}
