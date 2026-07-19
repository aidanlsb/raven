//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_NewPageRespectsPagesRoot verifies that creating a page type
// uses the configured pages root directory.
func TestIntegration_NewPageRespectsPagesRoot(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  type: objects/
  page: pages/
`).
		Build()

	result := v.RunCLI("new", "page", "Quick Note")
	result.MustSucceed(t)

	v.AssertFileExists("pages/quick-note.md")
	v.AssertFileNotExists("quick-note.md")
	v.AssertFileNotExists("objects/quick-note.md")
	v.AssertFileContains("pages/quick-note.md", "type: page")
}

func TestIntegration_InvalidRavenYAMLFailsCommands(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  type: [
`).
		Build()

	result := v.RunCLI("new", "page", "Broken Config Note")
	result.MustFail(t, "CONFIG_INVALID")
	result.MustFailWithMessage(t, "failed to load raven.yaml")

	v.AssertFileNotExists("broken-config-note.md")
}

func TestIntegration_ReadCommandsClassifyInvalidRavenYAMLAsConfigError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "anything"}},
		{name: "backlinks", args: []string{"backlinks", "anything"}},
		{name: "outlinks", args: []string{"outlinks", "anything"}},
		{name: "resolve", args: []string{"resolve", "anything"}},
		{name: "read", args: []string{"read", "anything"}},
		{name: "open", args: []string{"open", "anything"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.MinimalSchema()).
				WithRavenYAML(`directories:
  type: [
`).
				Build()

			result := v.RunCLI(tc.args...)
			result.MustFail(t, "CONFIG_INVALID")
			result.MustFailWithMessage(t, "failed to load raven.yaml")
		})
	}
}

func TestIntegration_ReadCommandsClassifyDatabaseBootstrapFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "anything"}},
		{name: "backlinks", args: []string{"backlinks", "anything"}},
		{name: "outlinks", args: []string{"outlinks", "anything"}},
		{name: "resolve", args: []string{"resolve", "anything"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.MinimalSchema()).
				Build()

			ravenDir := filepath.Join(v.Path, ".raven")
			if err := os.RemoveAll(ravenDir); err != nil {
				t.Fatalf("remove .raven directory: %v", err)
			}
			if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
				t.Fatalf("write .raven file: %v", err)
			}

			result := v.RunCLI(tc.args...)
			result.MustFail(t, "DATABASE_ERROR")
			result.MustFailWithMessage(t, "rvn reindex")
		})
	}
}
