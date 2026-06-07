package ui

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateWithEllipsisPreservesUTF8(t *testing.T) {
	t.Parallel()

	got := TruncateWithEllipsis("ééééé", 4)
	if got != "é..." {
		t.Fatalf("expected UTF-8-safe truncation, got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8 output, got %q", got)
	}
}

func TestWrapTextTwoLinesPreservesUTF8(t *testing.T) {
	t.Parallel()

	line1, line2 := WrapTextTwoLines("éééé éééé", 4)
	if line1 != "éééé" {
		t.Fatalf("expected first line to preserve multibyte characters, got %q", line1)
	}
	if line2 != "éééé" {
		t.Fatalf("expected second line to preserve multibyte characters, got %q", line2)
	}
}

func TestVisibleLenHandlesUnicodeAndANSI(t *testing.T) {
	t.Parallel()

	if got := VisibleLen("\x1b[31m猫\x1b[0m"); got != 2 {
		t.Fatalf("expected visible width 2 for colored CJK rune, got %d", got)
	}
}

func TestBacklinksLayoutStylesPrimaryContentLikeQueryResults(t *testing.T) {
	t.Parallel()

	columns := BacklinksLayout()
	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}
	if columns[1].Name != "content" {
		t.Fatalf("expected primary backlinks column to be content, got %q", columns[1].Name)
	}
	if columns[1].HasStyle {
		t.Fatalf("expected primary backlinks content to use default styling")
	}
	if columns[2].Name != "file" || !columns[2].HasStyle {
		t.Fatalf("expected backlinks location column to stay styled as file metadata")
	}
}
