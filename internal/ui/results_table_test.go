package ui

import (
	"strings"
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

func TestTableUsesVisibleWidthForANSIContent(t *testing.T) {
	t.Parallel()

	table := NewTable(2)
	table.AddRow("\x1b[31mred\x1b[0m", "ok")

	if table.colWidths[0] != 3 {
		t.Fatalf("expected ANSI escapes to be ignored when tracking width, got %d", table.colWidths[0])
	}

	rendered := table.String()
	if !strings.Contains(rendered, "red") {
		t.Fatalf("expected rendered table to include cell content, got %q", rendered)
	}
}

func TestVisibleLenHandlesUnicodeAndANSI(t *testing.T) {
	t.Parallel()

	if got := VisibleLen("\x1b[31m猫\x1b[0m"); got != 2 {
		t.Fatalf("expected visible width 2 for colored CJK rune, got %d", got)
	}
}
