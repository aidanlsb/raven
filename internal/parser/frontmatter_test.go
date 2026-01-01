package parser

import (
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantType string
		wantTags []string
		wantNil  bool
	}{
		{
			name: "basic frontmatter",
			content: `---
type: person
name: Alice
tags: [friend, colleague]
---

# Alice

Some content`,
			wantType: "person",
			wantTags: []string{"friend", "colleague"},
		},
		{
			name:    "no frontmatter",
			content: "# Just a heading\n\nSome content",
			wantNil: true,
		},
		{
			name: "frontmatter without type",
			content: `---
name: Alice
email: alice@example.com
---

Content here`,
			wantType: "",
		},
		{
			name: "tags as string",
			content: `---
type: daily
tags: work, personal
---

Content`,
			wantType: "daily",
			wantTags: []string{"work", "personal"},
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

			if len(tt.wantTags) > 0 {
				if len(fm.Tags) != len(tt.wantTags) {
					t.Errorf("tags = %v, want %v", fm.Tags, tt.wantTags)
				}
				for i, tag := range tt.wantTags {
					if i < len(fm.Tags) && fm.Tags[i] != tag {
						t.Errorf("tag[%d] = %q, want %q", i, fm.Tags[i], tag)
					}
				}
			}
		})
	}
}
