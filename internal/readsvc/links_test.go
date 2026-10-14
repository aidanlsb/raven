package readsvc

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestUtf16Units(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want int
	}{
		{in: "", want: 0},
		{in: "abc", want: 3},
		{in: "café", want: 4},
		{in: "😀", want: 2},
		{in: "a😀b", want: 4},
	}
	for _, tt := range tests {
		if got := utf16Units(tt.in); got != tt.want {
			t.Errorf("utf16Units(%q) = %d, want %d (std %d)", tt.in, got, tt.want, len(utf16.Encode([]rune(tt.in))))
		}
	}
}

func TestReadLinksStructuredOutput(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
alias: ally
---
# Alice
`).
		WithFile("staff/bob.md", `---
type: person
name: Bob
---
# Bob
`).
		WithFile("projects/raven.md", `---
type: project
title: Raven
status: active
owner: "[[people/alice]]"
---
café 😀 See [[people/alice|Alice]] and [[ally]] and [[missing-page]] and [[bob]] and `+"`[[people/alice]]`"+`.

`+"```\n[[people/alice]]\n```"+`

    [[people/alice]]
`).
		Build()

	rt := reindexReadRuntime(t, v.Path)

	result, err := Read(rt, ReadRequest{Reference: "projects/raven"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	byText := make(map[string]ReadLink, len(result.Links))
	for _, link := range result.Links {
		if result.Content[link.ByteStart:link.ByteEnd] != link.Text {
			t.Errorf("content[%d:%d] = %q, want text %q", link.ByteStart, link.ByteEnd, result.Content[link.ByteStart:link.ByteEnd], link.Text)
		}
		if utf16Units(result.Content[:link.ByteStart]) != link.UTF16Start {
			t.Errorf("%q utf16_start = %d, want %d", link.Text, link.UTF16Start, utf16Units(result.Content[:link.ByteStart]))
		}
		if utf16Units(result.Content[:link.ByteEnd]) != link.UTF16End {
			t.Errorf("%q utf16_end = %d, want %d", link.Text, link.UTF16End, utf16Units(result.Content[:link.ByteEnd]))
		}
		byText[link.Text] = link
	}

	if len(result.Links) != 5 {
		t.Fatalf("links = %#v, want 1 frontmatter + 4 body wikilinks (code skipped)", result.Links)
	}

	frontmatterLink := result.Links[0]
	if frontmatterLink.Text != "[[people/alice]]" || frontmatterLink.Status != LinkStatusResolved {
		t.Errorf("frontmatter link = %#v, want resolved [[people/alice]]", frontmatterLink)
	}
	if !strings.HasPrefix(result.Content, "---") {
		t.Fatalf("expected full file including frontmatter, got %q", result.Content[:20])
	}
	if frontmatterLink.ByteStart < 3 || result.Content[frontmatterLink.ByteStart:frontmatterLink.ByteEnd] != "[[people/alice]]" {
		t.Errorf("frontmatter offsets do not slice content: %#v", frontmatterLink)
	}

	aliased, ok := byText["[[people/alice|Alice]]"]
	if !ok {
		t.Fatalf("missing aliased link in %#v", result.Links)
	}
	if aliased.Target != "people/alice" || aliased.Status != LinkStatusResolved || aliased.ObjectID != "people/alice" {
		t.Errorf("aliased link = %#v, want resolved people/alice", aliased)
	}
	if !strings.Contains(result.Content[:aliased.ByteStart], "---") {
		t.Errorf("full-file offsets should include frontmatter prefix")
	}
	if aliased.ByteStart == aliased.UTF16Start {
		t.Errorf("utf16_start should differ from byte_start after café 😀, got both %d", aliased.ByteStart)
	}

	aliasTarget, ok := byText["[[ally]]"]
	if !ok {
		t.Fatalf("missing object-alias link in %#v", result.Links)
	}
	if aliasTarget.Status != LinkStatusResolved || aliasTarget.ObjectID != "people/alice" {
		t.Errorf("[[ally]] = %#v, want resolved people/alice", aliasTarget)
	}

	missing, ok := byText["[[missing-page]]"]
	if !ok {
		t.Fatalf("missing unresolved link in %#v", result.Links)
	}
	if missing.Status != LinkStatusUnresolved || missing.ObjectID != "" {
		t.Errorf("[[missing-page]] = %#v, want unresolved", missing)
	}

	shortName, ok := byText["[[bob]]"]
	if !ok {
		t.Fatalf("missing short-name link in %#v", result.Links)
	}
	if shortName.Status != LinkStatusResolved || shortName.ObjectID != "staff/bob" {
		t.Errorf("[[bob]] = %#v, want resolved staff/bob", shortName)
	}
}

func TestReadLinksAmbiguousAndSectionOffsets(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/ada.md", `---
type: person
name: Ada People
---
# Ada
`).
		WithFile("staff/ada.md", `---
type: person
name: Ada Staff
---
# Ada Staff
`).
		WithFile("projects/site.md", `---
type: project
title: Site
status: active
owner: people/ada
---
# Site

## Tasks

Link [[ada]] here.

## Notes

plain
`).
		Build()

	rt := reindexReadRuntime(t, v.Path)

	full, err := Read(rt, ReadRequest{Reference: "projects/site"})
	if err != nil {
		t.Fatalf("Read full: %v", err)
	}
	if len(full.Links) != 1 {
		t.Fatalf("full links = %#v, want 1", full.Links)
	}
	link := full.Links[0]
	if link.Text != "[[ada]]" || link.Target != "ada" || link.Status != LinkStatusAmbiguous {
		t.Errorf("full link = %#v, want ambiguous [[ada]]", link)
	}
	if len(link.Matches) < 2 {
		t.Errorf("matches = %#v, want both ada objects", link.Matches)
	}
	if full.Content[link.ByteStart:link.ByteEnd] != "[[ada]]" {
		t.Errorf("full slice = %q", full.Content[link.ByteStart:link.ByteEnd])
	}

	section, err := Read(rt, ReadRequest{Reference: "projects/site#tasks"})
	if err != nil {
		t.Fatalf("Read section: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(section.Content), "---") {
		t.Fatalf("section content still has frontmatter: %q", section.Content)
	}
	if len(section.Links) != 1 {
		t.Fatalf("section links = %#v, want 1", section.Links)
	}
	secLink := section.Links[0]
	if section.Content[secLink.ByteStart:secLink.ByteEnd] != "[[ada]]" {
		t.Fatalf("section slice = %q from %q", section.Content[secLink.ByteStart:secLink.ByteEnd], section.Content)
	}
	if secLink.ByteStart == link.ByteStart {
		t.Errorf("section offsets should be relative to the sliced body, not the full file")
	}
	if secLink.Status != LinkStatusAmbiguous {
		t.Errorf("section link status = %q, want ambiguous", secLink.Status)
	}
}

func TestReadRawOmitsLinks(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
See [[missing]].
`).
		Build()
	rt := reindexReadRuntime(t, v.Path)

	result, err := Read(rt, ReadRequest{Reference: "people/alice", Raw: true})
	if err != nil {
		t.Fatalf("Read raw: %v", err)
	}
	if len(result.Links) != 0 {
		t.Fatalf("raw links = %#v, want none", result.Links)
	}
}

func reindexReadRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	indexRT := testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{})
	if _, err := reindexsvc.Run(indexRT, reindexsvc.RunRequest{Full: true}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{OpenDB: true})
}
