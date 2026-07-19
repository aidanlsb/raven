//go:build integration

package mcp_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_DirectDispatchParityWithCLI(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)

	runMCPParityMutationTests(t, binary)
	runMCPParityRetrievalTests(t, binary)
	runMCPParityQueryTests(t, binary)
	runMCPParityMetaTests(t, binary)
	runMCPParitySkillTests(t, binary)
	runMCPParitySchemaTests(t, binary)
	runMCPParityTemplateTests(t, binary)
}
