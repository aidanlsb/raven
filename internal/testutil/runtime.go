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

// NewRuntimeForTest constructs a runtime with default options for unit tests.
// Does not register cleanup - caller must call Close().
func NewRuntimeForTest(t testing.TB, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	rt, err := vaultruntime.New(vaultPath, vaultruntime.Options{
		RequireSchema: false,
	})
	if err != nil {
		t.Fatalf("create vault runtime: %v", err)
	}
	return rt
}
