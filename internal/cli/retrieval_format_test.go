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
