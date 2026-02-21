package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestSchemaRenameType_PreviewReportsOptionalDefaultPathRename(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
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
`).
		WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
		WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
`).
		Build()

	beforeSchema := v.ReadFile("schema.yaml")
	beforeKickoff := v.ReadFile("events/kickoff.md")
	beforeRoadmap := v.ReadFile("projects/roadmap.md")

	res := v.RunCLI("schema", "rename", "type", "event", "meeting")
	res.MustSucceed(t)

	if got := res.Data["preview"]; got != true {
		t.Fatalf("expected preview=true, got %v", got)
	}
	if got := res.Data["default_path_rename_available"]; got != true {
		t.Fatalf("expected default_path_rename_available=true, got %v", got)
	}
	if got := res.Data["default_path_old"]; got != "events/" {
		t.Fatalf("expected default_path_old=events/, got %v", got)
	}
	if got := res.Data["default_path_new"]; got != "meetings/" {
		t.Fatalf("expected default_path_new=meetings/, got %v", got)
	}

	if got := v.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatalf("expected schema.yaml unchanged in preview mode")
	}
	if got := v.ReadFile("events/kickoff.md"); got != beforeKickoff {
		t.Fatalf("expected event file unchanged in preview mode")
	}
	if got := v.ReadFile("projects/roadmap.md"); got != beforeRoadmap {
		t.Fatalf("expected project file unchanged in preview mode")
	}
}

func TestSchemaRenameType_ConfirmWithDefaultPathRenameMovesFilesAndUpdatesRefs(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
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
`).
		WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
		WithFile("events/planning.md", `---
type: event
title: Planning
---
# Planning
`).
		WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
Planning: [[events/planning|Planning]]
::project(kickoff=events/kickoff)
`).
		Build()

	res := v.RunCLI("schema", "rename", "type", "event", "meeting", "--confirm", "--rename-default-path")
	res.MustSucceed(t)

	if got := res.Data["default_path_renamed"]; got != true {
		t.Fatalf("expected default_path_renamed=true, got %v", got)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meetings/")
	v.AssertFileContains("schema.yaml", "target: meeting")
	v.AssertFileNotContains("schema.yaml", "\n  event:\n")

	v.AssertFileExists("meetings/kickoff.md")
	v.AssertFileExists("meetings/planning.md")
	v.AssertFileNotExists("events/kickoff.md")
	v.AssertFileNotExists("events/planning.md")
	v.AssertFileContains("meetings/kickoff.md", "type: meeting")
	v.AssertFileContains("meetings/planning.md", "type: meeting")

	v.AssertFileContains("projects/roadmap.md", "kickoff: meetings/kickoff")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/kickoff]]")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/planning|Planning]]")
	v.AssertFileContains("projects/roadmap.md", "::project(kickoff=meetings/kickoff)")
	v.AssertFileNotContains("projects/roadmap.md", "events/kickoff")
	v.AssertFileNotContains("projects/roadmap.md", "events/planning")
}

func TestSchemaRenameType_ConfirmWithoutDefaultPathRenameKeepsDirectory(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  event:
    default_path: events/
    fields:
      title: { type: string }
traits: {}
`).
		WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
		Build()

	res := v.RunCLI("schema", "rename", "type", "event", "meeting", "--confirm")
	res.MustSucceed(t)

	if got := res.Data["default_path_renamed"]; got != false {
		t.Fatalf("expected default_path_renamed=false, got %v", got)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: events/")
	v.AssertFileNotContains("schema.yaml", "\n  event:\n")

	v.AssertFileExists("events/kickoff.md")
	v.AssertFileNotExists("meetings/kickoff.md")
	v.AssertFileContains("events/kickoff.md", "type: meeting")
}
