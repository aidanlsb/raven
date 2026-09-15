package objectsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func testRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{RequireSchema: true})
}
