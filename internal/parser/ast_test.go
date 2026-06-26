package parser

import (
	"testing"
)

func TestExtractFromAST(t *testing.T) {
	t.Parallel()
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

	t.Run("extracts refs whose targets contain backticks", func(t *testing.T) {
		content := "# Notes\n\nSee [[Use `strict` mode]] but not `[[ignored]]`.\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Refs) != 1 {
			t.Fatalf("got %d refs, want 1: %#v", len(result.Refs), result.Refs)
		}
		if result.Refs[0].TargetRaw != "Use `strict` mode" {
			t.Fatalf("target = %q, want %q", result.Refs[0].TargetRaw, "Use `strict` mode")
		}
	})

	t.Run("extracts local markdown asset links and images", func(t *testing.T) {
		content := "# Notes\n\nSee [paper](assets/papers/paper.pdf) and ![diagram](assets/images/diagram.png).\nExternal [site](https://example.com) and note [note](notes/next.md) are ignored.\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Refs) != 2 {
			t.Fatalf("got %d refs, want 2: %#v", len(result.Refs), result.Refs)
		}
		if result.Refs[0].TargetRaw != "assets/papers/paper.pdf" {
			t.Fatalf("first target = %q, want assets/papers/paper.pdf", result.Refs[0].TargetRaw)
		}
		if result.Refs[1].TargetRaw != "assets/images/diagram.png" {
			t.Fatalf("second target = %q, want assets/images/diagram.png", result.Refs[1].TargetRaw)
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

	t.Run("plain body text does not create traits or refs", func(t *testing.T) {
		content := "# Weekly Standup\nMeeting metadata: 09:00\n\nMeeting notes.\n"

		result, err := ExtractFromAST([]byte(content), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Headings) != 1 {
			t.Fatalf("got %d headings, want 1", len(result.Headings))
		}

		if len(result.Traits) != 0 {
			t.Fatalf("got %d traits, want none", len(result.Traits))
		}
		if len(result.Refs) != 0 {
			t.Fatalf("got %d refs, want none", len(result.Refs))
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
