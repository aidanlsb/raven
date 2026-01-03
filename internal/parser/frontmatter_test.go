package parser

import (
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantType string
		wantNil  bool
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
		},
		{
			name:    "no frontmatter",
			content: "# Just a heading\n\nSome content",
			wantNil: true,
		},
		{
			name: "frontmatter without type",
			content: `---
name: Freya
email: freya@asgard.realm
---

Content here`,
			wantType: "",
		},
		{
			name: "daily type",
			content: `---
type: daily
date: 2025-02-01
---

Content`,
			wantType: "daily",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, err := ParseFrontmatter(tt.content)
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
		})
	}
}
