//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_SchemaTemplateBindingsDriveNew(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  interview:
    default_path: interviews/
`).
		WithFile("templates/interview/technical.md", "## Technical Interview\n").
		Build()

	v.RunCLI("schema", "template", "set", "interview_technical", "--file", "templates/interview/technical.md").MustSucceed(t)
	v.RunCLI("schema", "type", "interview", "template", "set", "interview_technical").MustSucceed(t)
	v.RunCLI("schema", "type", "interview", "template", "default", "interview_technical").MustSucceed(t)

	v.RunCLI("new", "interview", "Jane Doe").MustSucceed(t)
	v.AssertFileContains("interviews/jane-doe.md", "## Technical Interview")

	v.RunCLI("schema", "type", "interview", "template", "default", "--clear").MustSucceed(t)

	v.RunCLI("new", "interview", "No Template").MustSucceed(t)
	v.AssertFileNotContains("interviews/no-template.md", "## Technical Interview")
}
