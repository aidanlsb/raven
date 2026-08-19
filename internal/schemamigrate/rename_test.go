package schemamigrate

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func migrationTestRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{})
}

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

func countTypeChanges(changes []schemasvc.TypeRenameChange, changeType string) int {
	count := 0
	for _, change := range changes {
		if change.ChangeType == changeType {
			count++
		}
	}
	return count
}

func hasTypeChangeForFile(changes []schemasvc.TypeRenameChange, changeType, filePath string) bool {
	for _, change := range changes {
		if change.ChangeType == changeType && change.FilePath == filePath {
			return true
		}
	}
	return false
}

// TestRenameType_PreviewCountMatchesApplyForQuotedTypes is the core anti-drift
// regression test. Preview and apply must consume the same migration plan,
// including for quoted YAML type values.
func TestRenameType_PreviewCountMatchesApplyForQuotedTypes(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(eventTypeSchema).
		WithFile("notes/quoted.md", "---\ntype: \"event\"\ntitle: Quoted\n---\n# Quoted\n").
		WithFile("notes/single.md", "---\ntype: 'event'\ntitle: Single\n---\n# Single\n").
		WithFile("notes/plain.md", "---\ntype: event\ntitle: Plain\n---\n# Plain\n\ntype: event\n").
		WithFile("notes/other.md", "---\ntype: page\ntitle: Other\n---\n# Other\n").
		Build()

	preview, err := RenameType(migrationTestRuntime(t, vault.Path), RenameTypeRequest{VaultPath: vault.Path, OldName: "event", NewName: "meeting"})
	if err != nil {
		t.Fatalf("preview RenameType: %v", err)
	}
	if !preview.Preview {
		t.Fatalf("expected preview result")
	}
	if got := countTypeChanges(preview.Changes, "frontmatter"); got != 3 {
		t.Fatalf("expected preview to count 3 frontmatter changes (quoted, single, plain), got %d\n%+v", got, preview.Changes)
	}

	apply, err := RenameType(migrationTestRuntime(t, vault.Path), RenameTypeRequest{VaultPath: vault.Path, OldName: "event", NewName: "meeting", Confirm: true})
	if err != nil {
		t.Fatalf("apply RenameType: %v", err)
	}
	if preview.TotalChanges != apply.ChangesApplied {
		t.Fatalf("preview total (%d) != apply applied (%d): counts drifted", preview.TotalChanges, apply.ChangesApplied)
	}

	for _, filePath := range []string{"notes/quoted.md", "notes/single.md"} {
		content := vault.ReadFile(filePath)
		if !strings.Contains(content, "type: meeting") {
			t.Fatalf("expected %s frontmatter to become meeting, got:\n%s", filePath, content)
		}
		if strings.Contains(content, "event") {
			t.Fatalf("expected %s to no longer mention event, got:\n%s", filePath, content)
		}
	}

	plain := vault.ReadFile("notes/plain.md")
	if !strings.Contains(plain, "type: meeting") {
		t.Fatalf("expected plain frontmatter to become meeting, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\ntype: event\n") {
		t.Fatalf("expected plain body occurrence of `type: event` to be preserved, got:\n%s", plain)
	}
	if got := vault.ReadFile("notes/other.md"); !strings.Contains(got, "type: page") {
		t.Fatalf("expected other.md to stay type page, got:\n%s", got)
	}
}

func TestRenameType_PreviewListsReferenceUpdatesFromDirectoryMove(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(eventProjectSchema).
		WithFile("events/kickoff.md", "---\ntype: event\ntitle: Kickoff\n---\n# Kickoff\n").
		WithFile("projects/roadmap.md", "---\ntype: project\nkickoff: events/kickoff\n---\n# Roadmap\n\nKickoff: [[events/kickoff]]\n").
		Build()

	beforeRoadmap := vault.ReadFile("projects/roadmap.md")
	preview, err := RenameType(migrationTestRuntime(t, vault.Path), RenameTypeRequest{VaultPath: vault.Path, OldName: "event", NewName: "meeting"})
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

	if got := vault.ReadFile("projects/roadmap.md"); got != beforeRoadmap {
		t.Fatalf("expected roadmap unchanged during preview")
	}
	vault.AssertFileExists("events/kickoff.md")
	vault.AssertFileNotExists("meetings/kickoff.md")
}

func TestRenameType_DefaultPathRenameHandlesQuotedTypeAndRefs(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(eventProjectSchema).
		WithFile("events/kickoff.md", "---\ntype: \"event\"\ntitle: Kickoff\n---\n# Kickoff\n").
		WithFile("projects/roadmap.md", "---\ntype: project\nkickoff: events/kickoff\n---\n# Roadmap\n\nKickoff: [[events/kickoff]]\n").
		Build()

	preview, err := RenameType(migrationTestRuntime(t, vault.Path), RenameTypeRequest{VaultPath: vault.Path, OldName: "event", NewName: "meeting"})
	if err != nil {
		t.Fatalf("preview RenameType: %v", err)
	}
	if got := countTypeChanges(preview.Changes, "frontmatter"); got != 1 {
		t.Fatalf("expected 1 frontmatter change in preview for quoted type, got %d\n%+v", got, preview.Changes)
	}
	if preview.FilesToMove != 1 {
		t.Fatalf("expected files_to_move=1, got %d", preview.FilesToMove)
	}

	result, err := RenameType(migrationTestRuntime(t, vault.Path), RenameTypeRequest{
		VaultPath:         vault.Path,
		OldName:           "event",
		NewName:           "meeting",
		Confirm:           true,
		RenameDefaultPath: true,
	})
	if err != nil {
		t.Fatalf("apply RenameType: %v", err)
	}
	if !result.DefaultPathRenamed {
		t.Fatalf("expected default_path_renamed=true")
	}
	if result.FilesMoved != 1 {
		t.Fatalf("expected files_moved=1, got %d", result.FilesMoved)
	}
	if result.ReferenceFilesUpdated != 1 {
		t.Fatalf("expected reference_files_updated=1, got %d", result.ReferenceFilesUpdated)
	}

	vault.AssertFileExists("meetings/kickoff.md")
	vault.AssertFileNotExists("events/kickoff.md")
	moved := vault.ReadFile("meetings/kickoff.md")
	if !strings.Contains(moved, "type: meeting") {
		t.Fatalf("expected moved file to have type: meeting, got:\n%s", moved)
	}
	if strings.Contains(moved, "event") {
		t.Fatalf("expected moved file to no longer mention event, got:\n%s", moved)
	}

	vault.AssertFileContains("projects/roadmap.md", "kickoff: meetings/kickoff")
	vault.AssertFileContains("projects/roadmap.md", "[[meetings/kickoff]]")
	vault.AssertFileNotContains("projects/roadmap.md", "events/kickoff")
	vault.AssertFileContains("schema.yaml", "meeting:")
	vault.AssertFileContains("schema.yaml", "default_path: meetings/")
	vault.AssertFileContains("schema.yaml", "target: meeting")
}

func TestRenameField_PreviewAndApplyUpdateAllPlannedSurfaces(t *testing.T) {
	t.Parallel()

	const personSchema = `version: 2
types:
  person:
    name_field: name
    template: templates/person.md
    fields:
      name: { type: string }
      email: { type: string }
traits: {}
`
	const ravenYAML = `queries:
  contacts:
    query: 'type:person .email=="alex@example.com"'
  unrelated:
    query: 'type:page .title=="Email"'
`
	vault := testutil.NewTestVault(t).
		WithSchema(personSchema).
		WithRavenYAML(ravenYAML).
		WithFile("templates/person.md", "Email: {{field.email}}\n").
		WithFile("people/alex.md", "---\ntype: person\nname: Alex\nemail: alex@example.com\n---\n# Alex\n").
		WithFile("notes/email.md", "---\ntype: page\ntitle: Email\nemail: body-only\n---\n").
		Build()

	beforeSchema := vault.ReadFile("schema.yaml")
	beforeTemplate := vault.ReadFile("templates/person.md")
	beforeConfig := vault.ReadFile("raven.yaml")
	beforePerson := vault.ReadFile("people/alex.md")
	beforePage := vault.ReadFile("notes/email.md")

	preview, err := RenameField(migrationTestRuntime(t, vault.Path), RenameFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "person",
		OldField:  "email",
		NewField:  "contact",
	})
	if err != nil {
		t.Fatalf("preview RenameField() error = %v", err)
	}
	if !preview.Preview || preview.TotalChanges != 4 {
		t.Fatalf("preview = %#v, want four planned changes", preview)
	}
	for _, changeType := range []string{"schema_field", "template_file", "saved_query", "frontmatter"} {
		if countFieldChanges(preview.Changes, changeType) != 1 {
			t.Fatalf("preview changes missing one %q: %#v", changeType, preview.Changes)
		}
	}
	if vault.ReadFile("schema.yaml") != beforeSchema ||
		vault.ReadFile("templates/person.md") != beforeTemplate ||
		vault.ReadFile("raven.yaml") != beforeConfig ||
		vault.ReadFile("people/alex.md") != beforePerson {
		t.Fatalf("preview mutated one or more vault files")
	}

	applied, err := RenameField(migrationTestRuntime(t, vault.Path), RenameFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "person",
		OldField:  "email",
		NewField:  "contact",
		Confirm:   true,
	})
	if err != nil {
		t.Fatalf("apply RenameField() error = %v", err)
	}
	if applied.Preview || applied.ChangesApplied != preview.TotalChanges {
		t.Fatalf("apply = %#v, want preview count %d", applied, preview.TotalChanges)
	}
	vault.AssertFileContains("schema.yaml", "contact:")
	vault.AssertFileNotContains("schema.yaml", "email:")
	vault.AssertFileContains("templates/person.md", "{{field.contact}}")
	vault.AssertFileNotContains("templates/person.md", "{{field.email}}")
	vault.AssertFileContains("raven.yaml", ".contact==")
	vault.AssertFileNotContains("raven.yaml", ".email==")
	vault.AssertFileContains("people/alex.md", "contact: alex@example.com")
	vault.AssertFileNotContains("people/alex.md", "email: alex@example.com")
	if got := vault.ReadFile("notes/email.md"); got != beforePage {
		t.Fatalf("rename changed a different object type:\n%s", got)
	}
}

func TestRenameField_ConflictBlocksAllWrites(t *testing.T) {
	t.Parallel()

	const personSchema = `version: 2
types:
  person:
    fields:
      name: { type: string }
      email: { type: string }
traits: {}
`
	vault := testutil.NewTestVault(t).
		WithSchema(personSchema).
		WithFile("people/alex.md", "---\ntype: person\nname: Alex\nemail: old@example.com\ncontact: new@example.com\n---\n").
		Build()
	beforeSchema := vault.ReadFile("schema.yaml")
	beforePerson := vault.ReadFile("people/alex.md")

	_, err := RenameField(migrationTestRuntime(t, vault.Path), RenameFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "person",
		OldField:  "email",
		NewField:  "contact",
		Confirm:   true,
	})
	requireMigrationCode(t, err, codes.ErrDataIntegrityBlock)
	if got := vault.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatalf("conflicted rename changed schema:\n%s", got)
	}
	if got := vault.ReadFile("people/alex.md"); got != beforePerson {
		t.Fatalf("conflicted rename changed object:\n%s", got)
	}
}

func TestRenameType_DestinationConflictBlocksAllWrites(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(eventProjectSchema).
		WithFile("events/kickoff.md", "---\ntype: event\ntitle: Kickoff\n---\n").
		WithFile("meetings/kickoff.md", "---\ntype: page\ntitle: Existing\n---\n").
		WithFile("projects/roadmap.md", "---\ntype: project\nkickoff: events/kickoff\n---\n[[events/kickoff]]\n").
		Build()
	beforeSchema := vault.ReadFile("schema.yaml")
	beforeSource := vault.ReadFile("events/kickoff.md")
	beforeDestination := vault.ReadFile("meetings/kickoff.md")
	beforeReference := vault.ReadFile("projects/roadmap.md")

	_, err := RenameType(migrationTestRuntime(t, vault.Path), RenameTypeRequest{
		VaultPath:         vault.Path,
		OldName:           "event",
		NewName:           "meeting",
		Confirm:           true,
		RenameDefaultPath: true,
	})
	requireMigrationCode(t, err, codes.ErrValidationFailed)
	if vault.ReadFile("schema.yaml") != beforeSchema ||
		vault.ReadFile("events/kickoff.md") != beforeSource ||
		vault.ReadFile("meetings/kickoff.md") != beforeDestination ||
		vault.ReadFile("projects/roadmap.md") != beforeReference {
		t.Fatalf("destination conflict mutated one or more vault files")
	}
}

func TestValidateTypeDirectoryMoves(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     []string
		moves     []typeDirectoryMove
		wantError string
	}{
		{name: "no moves"},
		{
			name:  "valid move",
			files: []string{"events/a.md"},
			moves: []typeDirectoryMove{{SourceRelPath: "events/a.md", DestinationRelPath: "meetings/a.md"}},
		},
		{
			name:      "missing source",
			moves:     []typeDirectoryMove{{SourceRelPath: "events/missing.md", DestinationRelPath: "meetings/missing.md"}},
			wantError: "source file does not exist",
		},
		{
			name:  "multiple sources share destination",
			files: []string{"events/a.md", "events/b.md"},
			moves: []typeDirectoryMove{
				{SourceRelPath: "events/a.md", DestinationRelPath: "meetings/same.md"},
				{SourceRelPath: "events/b.md", DestinationRelPath: "meetings/same.md"},
			},
			wantError: "multiple files would move",
		},
		{
			name:  "destination exists",
			files: []string{"events/a.md", "meetings/a.md"},
			moves: []typeDirectoryMove{
				{SourceRelPath: "events/a.md", DestinationRelPath: "meetings/a.md"},
			},
			wantError: "destination already exists",
		},
		{
			name:  "destination that is also a source is allowed",
			files: []string{"events/a.md", "meetings/a.md"},
			moves: []typeDirectoryMove{
				{SourceRelPath: "events/a.md", DestinationRelPath: "meetings/a.md"},
				{SourceRelPath: "meetings/a.md", DestinationRelPath: "archive/a.md"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vault := testutil.NewTestVault(t).Build()
			for _, filePath := range tt.files {
				vault.WriteFile(filePath, "content")
			}

			err := validateTypeDirectoryMoves(vault.Path, tt.moves)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateTypeDirectoryMoves() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}

func countFieldChanges(changes []schemasvc.FieldRenameChange, changeType string) int {
	count := 0
	for _, change := range changes {
		if change.ChangeType == changeType {
			count++
		}
	}
	return count
}

func requireMigrationCode(t *testing.T, err error, want codes.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var svcErr *schemasvc.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("error = %T %v, want service error", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %s, want %s", svcErr.Code, want)
	}
}
