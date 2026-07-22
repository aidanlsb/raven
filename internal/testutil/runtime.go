package testutil

import (
	"testing"

	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// NewVaultRuntime constructs a runtime for service tests and registers cleanup.
func NewVaultRuntime(t testing.TB, vaultPath string, opts vaultruntime.Options) *vaultruntime.Runtime {
	t.Helper()
	rt, err := vaultruntime.New(vaultPath, opts)
	if err != nil {
		t.Fatalf("create vault runtime: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt
}
