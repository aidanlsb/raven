package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePipeableList(t *testing.T) {
	items := []PipeableItem{
		{Num: 1, ID: "id1", Content: "First item", Location: "file1.md:10"},
		{Num: 2, ID: "id2", Content: "Second item", Location: "file2.md:20"},
		{Num: 3, ID: "id3", Content: "Third item", Location: "file3.md:30"},
	}

	var buf bytes.Buffer
	WritePipeableList(&buf, items)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	// Check format: Num<tab>ID<tab>Content<tab>Location
	expected := []struct {
		num      string
		id       string
		content  string
		location string
	}{
		{"1", "id1", "First item", "file1.md:10"},
		{"2", "id2", "Second item", "file2.md:20"},
		{"3", "id3", "Third item", "file3.md:30"},
	}

	for i, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			t.Errorf("Line %d: expected 4 tab-separated parts, got %d", i, len(parts))
			continue
		}
		if parts[0] != expected[i].num {
			t.Errorf("Line %d: Num = %q, want %q", i, parts[0], expected[i].num)
		}
		if parts[1] != expected[i].id {
			t.Errorf("Line %d: ID = %q, want %q", i, parts[1], expected[i].id)
		}
		if parts[2] != expected[i].content {
			t.Errorf("Line %d: Content = %q, want %q", i, parts[2], expected[i].content)
		}
		if parts[3] != expected[i].location {
			t.Errorf("Line %d: Location = %q, want %q", i, parts[3], expected[i].location)
		}
	}
}

func TestWritePipeableListSanitizesContent(t *testing.T) {
	items := []PipeableItem{
		{Num: 1, ID: "id1", Content: "Has\ttab", Location: "file.md:1"},
		{Num: 2, ID: "id2", Content: "Has\nnewline", Location: "file.md:2"},
	}

	var buf bytes.Buffer
	WritePipeableList(&buf, items)

	output := buf.String()

	// Should not contain tabs within content (only as separators)
	// Each line should have exactly 3 tabs (4 fields)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		tabCount := strings.Count(line, "\t")
		if tabCount != 3 {
			t.Errorf("Line %d has %d tabs, expected 3 (content should be sanitized)", i, tabCount)
		}
	}
}

func TestWritePipeableIDs(t *testing.T) {
	items := []PipeableItem{
		{Num: 1, ID: "id1", Content: "First", Location: "loc1"},
		{Num: 2, ID: "id2", Content: "Second", Location: "loc2"},
	}

	var buf bytes.Buffer
	WritePipeableIDs(&buf, items)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "id1" {
		t.Errorf("Line 0 = %q, want %q", lines[0], "id1")
	}
	if lines[1] != "id2" {
		t.Errorf("Line 1 = %q, want %q", lines[1], "id2")
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		want    string
	}{
		{
			name:    "short content unchanged",
			content: "hello",
			maxLen:  10,
			want:    "hello",
		},
		{
			name:    "exact length unchanged",
			content: "hello",
			maxLen:  5,
			want:    "hello",
		},
		{
			name:    "truncated with ellipsis",
			content: "hello world this is a long string",
			maxLen:  15,
			want:    "hello world...",
		},
		{
			name:    "truncates at word boundary",
			content: "hello world foo bar",
			maxLen:  16,
			want:    "hello world...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateContent(tt.content, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateContent() = %q, want %q", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("TruncateContent() length = %d, exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestSetPipeFormat(t *testing.T) {
	// Save original state
	original := pipeFormatOverride
	defer func() { pipeFormatOverride = original }()

	// Test setting to true
	trueVal := true
	SetPipeFormat(&trueVal)
	if pipeFormatOverride == nil || *pipeFormatOverride != true {
		t.Error("SetPipeFormat(true) did not set override correctly")
	}

	// Test setting to false
	falseVal := false
	SetPipeFormat(&falseVal)
	if pipeFormatOverride == nil || *pipeFormatOverride != false {
		t.Error("SetPipeFormat(false) did not set override correctly")
	}

	// Test clearing
	SetPipeFormat(nil)
	if pipeFormatOverride != nil {
		t.Error("SetPipeFormat(nil) did not clear override")
	}
}
