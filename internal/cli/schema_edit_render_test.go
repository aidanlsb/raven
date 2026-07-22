package cli

import (
	"strings"
	"testing"
)

func TestPrintSchemaFileChangesGroupsAndSortsCanonicalChanges(t *testing.T) {
	type change struct {
		file        string
		line        int
		description string
	}
	changes := []change{
		{file: "z.md", description: "schema update"},
		{file: "a.md", line: 7, description: "frontmatter update"},
		{file: "a.md", description: "template update"},
	}

	out := captureStdout(t, func() {
		printSchemaFileChanges(
			changes,
			func(item change) string { return item.file },
			func(item change) int { return item.line },
			func(item change) string { return item.description },
		)
	})

	if strings.Index(out, "a.md") > strings.Index(out, "z.md") {
		t.Fatalf("files are not sorted:\n%s", out)
	}
	for _, snippet := range []string{"Line 7: frontmatter update", "template update", "schema update"} {
		if !strings.Contains(out, snippet) {
			t.Fatalf("output missing %q:\n%s", snippet, out)
		}
	}
}
