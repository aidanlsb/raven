package checkfixsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestCreateMissingRefsNonInteractive_ReportsFailures(t *testing.T) {
	t.Parallel()

	result := CreateMissingRefsNonInteractive(
		t.TempDir(),
		&schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"meeting": {DefaultPath: "meeting/"},
			},
		},
		[]*check.MissingRef{
			{TargetPath: "all-hands", InferredType: "meeting"},
		},
		"",
		"",
		"daily",
		"",
		[]string{"meeting/"},
	)

	if result.Created != 0 {
		t.Fatalf("created = %d, want 0", result.Created)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("failures = %v, want 1 failure", result.Failures)
	}
	if !strings.Contains(result.Failures[0].Error, "protected") {
		t.Fatalf("unexpected failure error: %v", result.Failures[0])
	}
}
