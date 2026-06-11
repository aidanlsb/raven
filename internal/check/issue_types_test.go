package check

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAllIssueTypesDocumentedInAgentGuide(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	docPath := filepath.Join(filepath.Dir(filename), "..", "mcp", "agent-guide", "issue-types.md")
	contents, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read issue types guide: %v", err)
	}

	guide := string(contents)
	for _, issueType := range AllIssueTypes() {
		token := "`" + string(issueType) + "`"
		if !strings.Contains(guide, token) {
			t.Errorf("issue type %s is not documented in %s", token, docPath)
		}
	}
}
