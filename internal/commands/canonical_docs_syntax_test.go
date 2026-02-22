package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCanonicalDocsDoNotUseLegacyQueryOrMCPSyntax(t *testing.T) {
	t.Parallel()

	files := []string{
		"README.md",
		"docs/guide/cli.md",
		"docs/reference/cli.md",
		"docs/types-and-traits/file-format.md",
		"docs/agents/mcp.md",
		"internal/mcp/agent-guide/querying.md",
		"internal/mcp/agent-guide/examples.md",
		"internal/commands/registry.go",
	}

	legacyTokens := []string{
		"has:{",
		"contains:{",
		"within:{",
		"on:{",
		"refs:{",
		"refs:[[",
		`content:"`,
		`["mcp", "--vault"`,
	}

	root := repoRoot(t)
	for _, rel := range files {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		content := string(data)
		for _, token := range legacyTokens {
			if strings.Contains(content, token) {
				t.Errorf("%s contains legacy token %q", rel, token)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
