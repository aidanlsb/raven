package parser

import (
	"strings"
	"testing"
)

// FuzzParseFrontmatter tests that the frontmatter parser never panics on arbitrary input.
func FuzzParseFrontmatter(f *testing.F) {
	// Seed corpus with valid and edge-case inputs
	f.Add("---\ntype: page\n---\n")
	f.Add("---\n---\n")
	f.Add("---\ntype: page\ntitle: Test\n---\n")
	f.Add("---\ninvalid yaml: [unclosed\n---\n")
	f.Add("---\n")
	f.Add("")
	f.Add("not frontmatter")
	f.Add("---\ntype: 'quoted'\n---\n")
	f.Add("---\nlist:\n  - item1\n  - item2\n---\n")
	f.Add("---\nnested:\n  key: value\n---\n")
	f.Add(strings.Repeat("---\ntype: page\n", 100))
	f.Add("---\n" + strings.Repeat("x", 10000) + "\n---\n")

	f.Fuzz(func(t *testing.T, content string) {
		// The parser must not panic on any input
		_, err := ParseFrontmatter(content)
		// We don't care about the error, just that it doesn't panic
		_ = err
	})
}

// FuzzFrontmatterBounds tests that FrontmatterBounds never panics.
func FuzzFrontmatterBounds(f *testing.F) {
	f.Add("---\ntype: page\n---\ncontent")
	f.Add("---\n---\n")
	f.Add("---\nunclosed")
	f.Add("")
	f.Add("no frontmatter")
	f.Add("---")
	f.Add(strings.Repeat("---\n", 1000))

	f.Fuzz(func(t *testing.T, content string) {
		lines := strings.Split(content, "\n")
		startLine, endLine, ok := FrontmatterBounds(lines)

		// Sanity checks
		if ok && startLine < 0 {
			t.Errorf("FrontmatterBounds returned ok=true but startLine=%d < 0", startLine)
		}
		if ok && endLine >= 0 && endLine < startLine {
			t.Errorf("FrontmatterBounds returned endLine=%d < startLine=%d", endLine, startLine)
		}
		if ok && endLine >= 0 && endLine >= len(lines) {
			t.Errorf("FrontmatterBounds returned endLine=%d >= len(lines)=%d", endLine, len(lines))
		}
	})
}

// FuzzExtractFrontmatterYAML tests that raw frontmatter extraction never panics.
func FuzzExtractFrontmatterYAML(f *testing.F) {
	f.Add("---\ntype: page\n---\n")
	f.Add("---\n---\n")
	f.Add("")
	f.Add("---")
	f.Add("not yaml at all")
	f.Add(strings.Repeat("a", 100000))

	f.Fuzz(func(t *testing.T, content string) {
		lines := strings.Split(content, "\n")
		startLine, endLine, ok := FrontmatterBounds(lines)
		if !ok || endLine < 0 {
			return
		}

		// Extract the YAML content between delimiters
		var yamlLines []string
		for i := startLine + 1; i < endLine && i < len(lines); i++ {
			yamlLines = append(yamlLines, lines[i])
		}
		yaml := strings.Join(yamlLines, "\n")

		// Just ensure we can construct it without panic
		_ = yaml
	})
}
