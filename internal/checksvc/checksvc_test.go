package checksvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestRun_FiltersParseErrorsBeforeCounting(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		opts Options
	}{
		{
			name: "issues filter excludes parse error",
			opts: Options{Issues: "missing_reference", ErrorsOnly: true},
		},
		{
			name: "exclude filter drops parse error",
			opts: Options{Exclude: "parse_error", ErrorsOnly: true},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vault := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("broken.md", "---\ntype: person\nname: [\n---\nbody\n").
				Build()

			sch, err := schema.Load(vault.Path)
			if err != nil {
				t.Fatalf("load schema: %v", err)
			}

			result, err := Run(vault.Path, &config.VaultConfig{}, sch, tt.opts)
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			if got := result.ErrorCount; got != 0 {
				t.Fatalf("error count = %d, want 0", got)
			}
			if len(result.Issues) != 0 {
				t.Fatalf("issues = %v, want none", result.Issues)
			}

			jsonResult := BuildJSON(vault.Path, result)
			if got := jsonResult.ErrorCount; got != 0 {
				t.Fatalf("json error_count = %d, want 0", got)
			}
			if len(jsonResult.Issues) != 0 {
				t.Fatalf("json issues = %v, want none", jsonResult.Issues)
			}
		})
	}
}

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
