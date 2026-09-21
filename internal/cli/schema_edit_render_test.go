package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schemasvc"
)

func TestPrintSchemaFileChangesGroupsAndSortsCanonicalChanges(t *testing.T) {
	changes := []schemasvc.SchemaChange{
		{FilePath: "z.md", Description: "schema update"},
		{FilePath: "a.md", Line: 7, Description: "frontmatter update"},
		{FilePath: "a.md", Description: "template update"},
	}

	out := captureStdout(t, func() {
		printSchemaFileChanges(changes)
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
