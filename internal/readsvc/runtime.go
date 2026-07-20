package readsvc

import "github.com/aidanlsb/raven/internal/vaultruntime"

// RuntimeOptions is retained as a read-service alias for the shared
// invocation-scoped vault runtime options.
type RuntimeOptions = vaultruntime.Options

// Runtime is retained as a read-service alias so read operations can consume
// the shared vault runtime without introducing a parallel context type.
type Runtime = vaultruntime.Runtime

// NewRuntime constructs the shared invocation-scoped vault runtime.
func NewRuntime(vaultPath string, opts RuntimeOptions) (*Runtime, error) {
	return vaultruntime.New(vaultPath, opts)
}
