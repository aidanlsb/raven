package schemasvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestValidate_ReportsTraitAndEnumSchemaIssues(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(`version: 1
types:
  project:
    fields:
      status:
        type: enum
traits:
  priority:
    type: enum
  highlight:
    type: enum-ish
`).Build()

	result, err := Validate(vault.Path)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid schema, got valid result: %#v", result)
	}

	for _, want := range []string{
		"Type 'project' field 'status' of type 'enum' must define at least one allowed value",
		"Trait 'priority' of type 'enum' must define at least one allowed value",
		"Trait 'highlight' has unknown trait type 'enum-ish'",
	} {
		if !containsIssue(result.Issues, want) {
			t.Fatalf("expected issue containing %q, got %v", want, result.Issues)
		}
	}
}

func containsIssue(issues []string, want string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, want) {
			return true
		}
	}
	return false
}
