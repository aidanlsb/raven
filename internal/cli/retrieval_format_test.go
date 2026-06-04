package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/model"
)

func TestPrintSearchResultsIncludesLocation(t *testing.T) {
	prevJSON := jsonOutput
	prevHyperlinksDisabled := hyperlinksDisabled
	prevHyperlinkEnabled := hyperlinkEnabled
	jsonOutput = false
	setHyperlinksDisabled(true)
	t.Cleanup(func() {
		jsonOutput = prevJSON
		hyperlinksDisabled = prevHyperlinksDisabled
		hyperlinkEnabled = prevHyperlinkEnabled
	})

	out := captureStdout(t, func() {
		printSearchResults("quarterly", []model.SearchMatch{
			{
				ObjectID: "notes/meeting",
				Title:    "Team Meeting",
				FilePath: "notes/meeting.md",
				Snippet:  "Discussed the »quarterly« roadmap.",
			},
		})
	})

	if !strings.Contains(out, "notes/meeting.md:1") {
		t.Fatalf("expected search output to include location, got: %q", out)
	}
}

func TestPrintBacklinksAndOutlinksUseQueryStyleLocations(t *testing.T) {
	prevJSON := jsonOutput
	prevHyperlinksDisabled := hyperlinksDisabled
	prevHyperlinkEnabled := hyperlinkEnabled
	jsonOutput = false
	setHyperlinksDisabled(true)
	t.Cleanup(func() {
		jsonOutput = prevJSON
		hyperlinksDisabled = prevHyperlinksDisabled
		hyperlinkEnabled = prevHyperlinkEnabled
	})

	backlinkLine := 12
	backlinkLabel := "planning note"
	backlinksOut := captureStdout(t, func() {
		printBacklinksResults("project/raven", []model.Reference{
			{
				SourceID:    "note/planning",
				TargetRaw:   "project/raven",
				FilePath:    "note/planning.md",
				Line:        &backlinkLine,
				DisplayText: &backlinkLabel,
			},
		})
	})

	if !strings.Contains(backlinksOut, "planning note") {
		t.Fatalf("expected backlinks output to include display text, got: %q", backlinksOut)
	}
	if !strings.Contains(backlinksOut, "note/planning.md:12") {
		t.Fatalf("expected backlinks output to include query-style location, got: %q", backlinksOut)
	}

	outlinkLine := 7
	outlinkLabel := "Raven"
	outlinksOut := captureStdout(t, func() {
		printOutlinksResults("note/planning", []model.Reference{
			{
				SourceID:    "note/planning",
				TargetRaw:   "project/raven",
				FilePath:    "note/planning.md",
				Line:        &outlinkLine,
				DisplayText: &outlinkLabel,
			},
		})
	})

	if !strings.Contains(outlinksOut, "Raven (project/raven)") {
		t.Fatalf("expected outlinks output to include alias and target, got: %q", outlinksOut)
	}
	if !strings.Contains(outlinksOut, "note/planning.md:7") {
		t.Fatalf("expected outlinks output to include query-style location, got: %q", outlinksOut)
	}
}
