package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestMCPDocsToolListMatchesGeneratedTools(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	docsPath := filepath.Join(repoRoot, "docs", "agents", "mcp.md")
	raw, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", docsPath, err)
	}

	const beginMarker = "<!-- BEGIN MCP TOOL LIST -->"
	const endMarker = "<!-- END MCP TOOL LIST -->"

	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	begin := strings.Index(content, beginMarker)
	end := strings.Index(content, endMarker)
	if begin < 0 || end < 0 || end <= begin {
		t.Fatalf("could not find MCP tool list markers (%q ... %q) in %s", beginMarker, endMarker, docsPath)
	}

	actual := strings.TrimSpace(content[begin+len(beginMarker) : end])
	expected := strings.TrimSpace(generateMCPToolListMarkdown())
	if actual != expected {
		t.Fatalf("MCP tool list in %s is out of sync with generated tools", docsPath)
	}
}

func generateMCPToolListMarkdown() string {
	tools := GenerateToolSchemas()
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	var b strings.Builder
	b.WriteString("| Tool | Description |\n")
	b.WriteString("|------|-------------|\n")
	for _, tool := range tools {
		desc := strings.TrimSpace(tool.Description)
		if idx := strings.Index(desc, "\n"); idx >= 0 {
			desc = strings.TrimSpace(desc[:idx])
		}
		desc = strings.ReplaceAll(desc, "|", "\\|")
		b.WriteString("| `")
		b.WriteString(tool.Name)
		b.WriteString("` | ")
		b.WriteString(desc)
		b.WriteString(" |\n")
	}

	return b.String()
}
