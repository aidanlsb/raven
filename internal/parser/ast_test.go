package parser

import (
	"testing"
)

func TestExtractFromAST(t *testing.T) {
	t.Run("extracts headings", func(t *testing.T) {
		content := `# Title

## Section One

Some content.

## Section Two

More content.
`
		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Headings) != 3 {
			t.Errorf("got %d headings, want 3", len(result.Headings))
		}
	})

	t.Run("skips fenced code blocks", func(t *testing.T) {
		content := "# Notes\n\n@todo Real trait\n\n```python\n@decorator\ndef foo(): pass\n```\n\n@done Another trait\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should find 2 traits, not 3 (decorator is in code block)
		if len(result.Traits) != 2 {
			t.Errorf("got %d traits, want 2", len(result.Traits))
			for _, tr := range result.Traits {
				t.Logf("  trait: %s (line %d)", tr.TraitName, tr.Line)
			}
		}
	})

	t.Run("skips inline code", func(t *testing.T) {
		content := "# Notes\n\nUse `@decorator` for Python. @todo Real trait\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should find 1 trait (decorator is in inline code)
		if len(result.Traits) != 1 {
			t.Errorf("got %d traits, want 1", len(result.Traits))
			for _, tr := range result.Traits {
				t.Logf("  trait: %s", tr.TraitName)
			}
		}
	})

	t.Run("extracts refs from wikilinks", func(t *testing.T) {
		content := "# Notes\n\nSee [[people/freya]] and [[projects/website]] for details.\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Refs) != 2 {
			t.Errorf("got %d refs, want 2", len(result.Refs))
			for _, ref := range result.Refs {
				t.Logf("  ref: %s", ref.TargetRaw)
			}
		}
	})

	t.Run("skips refs in code blocks", func(t *testing.T) {
		content := "# Notes\n\n[[real-ref]]\n\n```\n[[fake-ref]]\n```\n\n[[another-real]]\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Refs) != 2 {
			t.Errorf("got %d refs, want 2", len(result.Refs))
			for _, ref := range result.Refs {
				t.Logf("  ref: %s", ref.TargetRaw)
			}
		}
	})

	t.Run("detects type declarations after headings", func(t *testing.T) {
		content := "# Weekly Standup\n::meeting(time=09:00)\n\nMeeting notes.\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Headings) != 1 {
			t.Fatalf("got %d headings, want 1", len(result.Headings))
		}

		headingLine := result.Headings[0].Line
		if decl, ok := result.TypeDecls[headingLine]; !ok {
			t.Error("type declaration not found for heading")
		} else if decl.TypeName != "meeting" {
			t.Errorf("type = %q, want meeting", decl.TypeName)
		}
	})

	t.Run("handles list items with traits", func(t *testing.T) {
		content := "# Tasks\n\n- @todo First task\n- @done Second task\n- Regular item\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Traits) != 2 {
			t.Errorf("got %d traits, want 2", len(result.Traits))
		}
	})

	t.Run("handles code blocks in list items", func(t *testing.T) {
		content := "- @todo Real task\n- ```\n  @fake inside code\n  ```\n- @done Another task\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should find 2 traits (fake is in code block)
		if len(result.Traits) != 2 {
			t.Errorf("got %d traits, want 2", len(result.Traits))
			for _, tr := range result.Traits {
				t.Logf("  trait: %s (line %d)", tr.TraitName, tr.Line)
			}
		}
	})

	t.Run("handles indented code blocks", func(t *testing.T) {
		content := "# Notes\n\n@todo Real trait\n\n    @indented code block\n    more code\n\n@done After code\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should find 2 traits (indented block is code)
		if len(result.Traits) != 2 {
			t.Errorf("got %d traits, want 2", len(result.Traits))
			for _, tr := range result.Traits {
				t.Logf("  trait: %s (line %d)", tr.TraitName, tr.Line)
			}
		}
	})
}
