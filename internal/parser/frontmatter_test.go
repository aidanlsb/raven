package parser

import (
	"strings"
	"testing"
)

func TestFrontmatterBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		lines     []string
		wantStart int
		wantEnd   int
		wantOK    bool
	}{
		{
			name:      "normal frontmatter",
			lines:     []string{"---", "type: person", "---", "", "content"},
			wantStart: 0, wantEnd: 2, wantOK: true,
		},
		{
			name:      "empty frontmatter",
			lines:     []string{"---", "---", "", "content"},
			wantStart: 0, wantEnd: 1, wantOK: true,
		},
		{
			name:      "unclosed frontmatter",
			lines:     []string{"---", "type: person", "name: Freya"},
			wantStart: 0, wantEnd: -1, wantOK: true,
		},
		{
			name:      "no frontmatter",
			lines:     []string{"# Title", "content"},
			wantStart: 0, wantEnd: -1, wantOK: false,
		},
		{
			name:      "empty input",
			lines:     []string{},
			wantStart: 0, wantEnd: -1, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := FrontmatterBounds(tt.lines)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if start != tt.wantStart {
				t.Errorf("startLine = %d, want %d", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("endLine = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		content     string
		wantType    string
		wantNil     bool
		wantEndLine int
		wantErr     bool
	}{
		{
			name: "basic frontmatter",
			content: `---
type: person
name: Freya
email: freya@asgard.realm
---

# Freya

Some content`,
			wantType: "person",
			// Closing --- is line 5.
			wantEndLine: 5,
		},
		{
			name:    "no frontmatter",
			content: "# Just a heading\n\nSome content",
			wantNil: true,
		},
		{
			name: "empty frontmatter still counts as frontmatter",
			content: `---
---

# Title
Content`,
			wantType:    "",
			wantEndLine: 2,
		},
		{
			name: "frontmatter without type",
			content: `---
name: Freya
email: freya@asgard.realm
client: "[[clients/midgard|Midgard]]"
---

Content here`,
			wantType:    "",
			wantEndLine: 5,
		},
		{
			name: "daily type",
			content: `---
type: daily
date: 2025-02-01
---

Content`,
			wantType:    "daily",
			wantEndLine: 4,
		},
		{
			name: "datetime in frontmatter",
			content: `---
type: meeting
starts_at: 2025-02-01T10:30:00Z
---

Content`,
			wantType:    "meeting",
			wantEndLine: 4,
		},
		{
			name: "unclosed frontmatter returns nil",
			content: `---
type: person
name: Freya
`,
			wantNil: true,
		},
		{
			name: "nested YAML object is rejected",
			content: `---
type: person
address:
  city: Oslo
  country: Norway
---
`,
			wantErr: true,
		},
		{
			name: "nested YAML object inside array is rejected",
			content: `---
type: person
history:
  - year: 2025
    city: Oslo
---
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, err := ParseFrontmatter(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if fm != nil {
					t.Error("expected nil frontmatter")
				}
				return
			}

			if fm == nil {
				t.Fatal("expected non-nil frontmatter")
			}

			if fm.ObjectType != tt.wantType {
				t.Errorf("type = %q, want %q", fm.ObjectType, tt.wantType)
			}

			// Ensure wikilinks with display text in YAML frontmatter still parse as refs.
			if strings.Contains(tt.content, "client:") {
				v, ok := fm.Fields["client"]
				if !ok {
					t.Fatalf("expected client field to be present")
				}
				if ref, ok := v.AsRef(); !ok || ref != "clients/midgard" {
					t.Fatalf("expected client to be ref clients/midgard, got %v", v)
				}
			}
			if strings.Contains(tt.content, "starts_at:") {
				v, ok := fm.Fields["starts_at"]
				if !ok {
					t.Fatalf("expected starts_at field to be present")
				}
				if !v.IsDatetime() {
					t.Fatalf("expected starts_at to be datetime, got %v", v)
				}
				if s, ok := v.AsString(); !ok || s != "2025-02-01T10:30" {
					t.Fatalf("expected starts_at to be 2025-02-01T10:30, got %v", v)
				}
			}

			if tt.wantEndLine != 0 && fm.EndLine != tt.wantEndLine {
				t.Errorf("EndLine = %d, want %d", fm.EndLine, tt.wantEndLine)
			}
		})
	}
}

func TestFieldValueFromYAML_UnsupportedMapReturnsNull(t *testing.T) {
	t.Parallel()

	got := FieldValueFromYAML(map[string]interface{}{
		"city":    "Oslo",
		"country": "Norway",
	})

	if !got.IsNull() {
		t.Fatalf("expected null, got %v", got)
	}
}

func TestFieldValueFromYAML_ScalarAndArrayKinds(t *testing.T) {
	t.Parallel()

	stringVal := FieldValueFromYAML("hello")
	if s, ok := stringVal.AsString(); !ok || s != "hello" || stringVal.IsRef() || stringVal.IsDate() || stringVal.IsDatetime() {
		t.Fatalf("expected plain string value, got %v", stringVal)
	}

	boolVal := FieldValueFromYAML(true)
	if b, ok := boolVal.AsBool(); !ok || !b {
		t.Fatalf("expected bool true, got %v", boolVal)
	}

	arrayVal := FieldValueFromYAML([]interface{}{"a", "b"})
	arr, ok := arrayVal.AsArray()
	if !ok {
		t.Fatalf("expected array value, got %v", arrayVal)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 array items, got %d", len(arr))
	}
	if s, ok := arr[0].AsString(); !ok || s != "a" {
		t.Fatalf("first array item = %v, want string %q", arr[0], "a")
	}
	if s, ok := arr[1].AsString(); !ok || s != "b" {
		t.Fatalf("second array item = %v, want string %q", arr[1], "b")
	}
}
