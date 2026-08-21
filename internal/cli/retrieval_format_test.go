package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
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

func TestPrintReferenceGroupsIncludeGroupHeadersAndErrors(t *testing.T) {
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

	line := 12
	label := "planning note"
	out := captureStdout(t, func() {
		printBacklinksGroups([]model.BacklinksGroup{
			{
				Input:  "project/raven",
				Target: "project/raven",
				Items: []model.Reference{
					{
						SourceID:    "note/planning",
						TargetRaw:   "project/raven",
						FilePath:    "note/planning.md",
						Line:        &line,
						DisplayText: &label,
					},
				},
				Count: 1,
			},
		}, []model.ReferenceInputError{
			{Input: "missing", Code: "REF_NOT_FOUND", Message: "reference not found"},
		})
	})

	for _, want := range []string{"Backlinks to project/raven", "planning note", "note/planning.md:12", "Errors", "missing: reference not found"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected grouped backlinks output to include %q, got: %q", want, out)
		}
	}
}

func TestPrintObjectTableIncludesHeadersNameFieldDynamicFieldsAndLocation(t *testing.T) {
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

	sch := schema.New()
	sch.Types["project"] = &schema.TypeDefinition{
		NameField: "name",
		Fields: map[string]*schema.FieldDefinition{
			"name":   {Type: schema.FieldTypeString},
			"owner":  {Type: schema.FieldTypeString},
			"status": {Type: schema.FieldTypeString},
		},
	}

	out := captureStdout(t, func() {
		printObjectTable([]model.Object{
			{
				ID:        "projects/raven",
				Type:      "project",
				FilePath:  "projects/raven.md",
				LineStart: 3,
				Fields: map[string]fieldvalue.FieldValue{
					"name":   fieldvalue.String("Raven Project"),
					"owner":  fieldvalue.String("people/aidan"),
					"status": fieldvalue.String("active"),
				},
			},
		}, sch)
	})

	for _, want := range []string{"name", "owner", "status", "location", "Raven Project", "aidan", "active", "projects/raven.md:3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected object table output to include %q, got: %q", want, out)
		}
	}
}

func TestPrintQuerySectionResultsIncludesColumnHeaders(t *testing.T) {
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
		printQuerySectionResults("section level:2", []model.Section{
			{
				ID:        "page/raven#section-query-results",
				FilePath:  "page/raven.md",
				Slug:      "section-query-results",
				Title:     "Section query results",
				Level:     2,
				LineStart: 17,
			},
		})
	})

	for _, want := range []string{"title", "heading", "location", "Section query results", "h2 #section-query-results", "page/raven.md:17"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected section query output to include %q, got: %q", want, out)
		}
	}
}

func TestPrintQueryTraitResultsIncludesColumnHeaders(t *testing.T) {
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

	value := "open"
	out := captureStdout(t, func() {
		trait := model.Trait{
			TraitType: "status",
			Content:   "Review section query output",
			FilePath:  "page/raven.md",
			Line:      23,
		}
		trait.SetIndexValueString(&value)
		printQueryTraitResults("trait:status", "status", []model.Trait{trait})
	})

	for _, want := range []string{"content", "trait", "location", "Review section query output", "@status(open)", "page/raven.md:23"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected trait query output to include %q, got: %q", want, out)
		}
	}
}

func TestPrintQueryLinkResultsHyperlinksVaultFileTarget(t *testing.T) {
	prevJSON := jsonOutput
	prevHyperlinksDisabled := hyperlinksDisabled
	prevHyperlinkEnabled := hyperlinkEnabled
	prevVaultPath := resolvedVaultPath
	prevCfg := cfg
	jsonOutput = false
	hyperlinksDisabled = false
	enabled := true
	hyperlinkEnabled = &enabled
	resolvedVaultPath = t.TempDir()
	cfg = &config.Config{Editor: "cursor"}
	t.Cleanup(func() {
		jsonOutput = prevJSON
		hyperlinksDisabled = prevHyperlinksDisabled
		hyperlinkEnabled = prevHyperlinkEnabled
		resolvedVaultPath = prevVaultPath
		cfg = prevCfg
	})

	link := model.Link{
		SourceID:      "projects/raven",
		FilePath:      "projects/raven.md",
		Line:          12,
		RawTarget:     "../files/spec.pdf",
		Display:       "Spec",
		Scheme:        "file",
		Ext:           "pdf",
		NormalizedKey: "files/spec.pdf",
	}
	out := captureStdout(t, func() {
		printQueryLinkResults("link .ext==pdf", []model.Link{link})
	})

	targetURL := buildEditorURL(cfg, filepath.Join(resolvedVaultPath, filepath.FromSlash(link.NormalizedKey)), 1)
	if !strings.Contains(out, "\x1b]8;;"+targetURL+"\x07Spec (../files/spec.pdf)\x1b]8;;\x07") {
		t.Fatalf("expected target cell to hyperlink the in-vault file, got: %q", out)
	}
	sourceURL := buildEditorURL(cfg, filepath.Join(resolvedVaultPath, link.FilePath), link.Line)
	if !strings.Contains(out, "\x1b]8;;"+sourceURL+"\x07") {
		t.Fatalf("expected source location to remain hyperlinked, got: %q", out)
	}
}

func TestPrintQueryLinkResultsNoLinksDisablesHyperlinks(t *testing.T) {
	prevJSON := jsonOutput
	prevHyperlinksDisabled := hyperlinksDisabled
	prevHyperlinkEnabled := hyperlinkEnabled
	prevVaultPath := resolvedVaultPath
	jsonOutput = false
	resolvedVaultPath = t.TempDir()
	setHyperlinksDisabled(true)
	t.Cleanup(func() {
		jsonOutput = prevJSON
		hyperlinksDisabled = prevHyperlinksDisabled
		hyperlinkEnabled = prevHyperlinkEnabled
		resolvedVaultPath = prevVaultPath
	})

	out := captureStdout(t, func() {
		printQueryLinkResults("link .ext==pdf", []model.Link{{
			FilePath:      "projects/raven.md",
			Line:          12,
			RawTarget:     "../files/spec.pdf",
			Display:       "Spec",
			Scheme:        "file",
			NormalizedKey: "files/spec.pdf",
		}})
	})

	if strings.Contains(out, "\x1b]8;;") {
		t.Fatalf("--no-links output unexpectedly contains OSC 8: %q", out)
	}
	if !strings.Contains(out, "Spec (../files/spec.pdf)") {
		t.Fatalf("expected plain target text, got: %q", out)
	}
}

func TestRenderCanonicalQueryHumanLinkPipeHasNoHyperlinks(t *testing.T) {
	prevJSON := jsonOutput
	prevHyperlinksDisabled := hyperlinksDisabled
	prevHyperlinkEnabled := hyperlinkEnabled
	prevPipeOverride := pipeFormatOverride
	jsonOutput = false
	hyperlinksDisabled = false
	enabled := true
	hyperlinkEnabled = &enabled
	usePipe := true
	SetPipeFormat(&usePipe)
	t.Cleanup(func() {
		jsonOutput = prevJSON
		hyperlinksDisabled = prevHyperlinksDisabled
		hyperlinkEnabled = prevHyperlinkEnabled
		pipeFormatOverride = prevPipeOverride
	})

	out := captureStdout(t, func() {
		err := renderCanonicalQueryHuman("link .ext==pdf", commandpayload.QueryLinkResult{
			Items: []commandpayload.LinkItem{{
				SourceID:      "projects/raven",
				FilePath:      "projects/raven.md",
				Line:          12,
				RawTarget:     "../files/spec.pdf",
				Display:       "Spec",
				Scheme:        "file",
				NormalizedKey: "files/spec.pdf",
			}},
		}, false)
		if err != nil {
			t.Fatalf("renderCanonicalQueryHuman() error = %v", err)
		}
	})

	if strings.Contains(out, "\x1b]8;;") {
		t.Fatalf("pipe output unexpectedly contains OSC 8: %q", out)
	}
	if !strings.Contains(out, "projects/raven\tSpec (../files/spec.pdf)\tprojects/raven.md:12") {
		t.Fatalf("expected plain pipe row, got: %q", out)
	}
}
