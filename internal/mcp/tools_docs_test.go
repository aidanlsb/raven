package mcp

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

func TestMCPDocsOnlyUseCompactToolCalls(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	targets := []string{
		filepath.Join(repoRoot, "docs", "agents", "mcp.md"),
		filepath.Join(repoRoot, "internal", "mcp", "agent-guide"),
	}

	callPattern := regexp.MustCompile(`\braven_[a-z_]+\s*\(`)

	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat %s: %v", target, err)
		}
		if !info.IsDir() {
			assertNoLegacyCallsInDoc(t, target, callPattern)
			continue
		}

		err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			assertNoLegacyCallsInDoc(t, path, callPattern)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", target, err)
		}
	}
}

func assertNoLegacyCallsInDoc(t *testing.T, path string, pattern *regexp.Regexp) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	for _, loc := range pattern.FindAllStringIndex(content, -1) {
		call := strings.TrimSpace(content[loc[0]:loc[1]])
		if strings.HasPrefix(call, "raven_discover(") ||
			strings.HasPrefix(call, "raven_describe(") ||
			strings.HasPrefix(call, "raven_invoke(") {
			continue
		}

		line := 1 + strings.Count(content[:loc[0]], "\n")
		t.Fatalf("legacy MCP direct-call example found in %s at line %d", path, line)
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
