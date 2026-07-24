package readsvc

import "github.com/aidanlsb/raven/internal/refresolve"

// ResolveResult is retained as a compatibility alias for read-service callers.
type ResolveResult = refresolve.ResolveResult

// AmbiguousRefError is retained as a compatibility alias for read-service
// callers. New service code should use refresolve directly.
type AmbiguousRefError = refresolve.AmbiguousRefError

// RefNotFoundError is retained as a compatibility alias for read-service
// callers. New service code should use refresolve directly.
type RefNotFoundError = refresolve.RefNotFoundError

func IsAmbiguousRef(err error) bool {
	return refresolve.IsAmbiguousRef(err)
}

func IsRefNotFound(err error) bool {
	return refresolve.IsRefNotFound(err)
}

type resolveOperation struct {
	op *refresolve.Operation
}

func newResolveOperation(rt *Runtime) (*resolveOperation, error) {
	op, err := refresolve.New(rt)
	if err != nil {
		return nil, err
	}
	return &resolveOperation{op: op}, nil
}

func (op *resolveOperation) Close() error {
	return nil
}

func (op *resolveOperation) resolveReference(reference string, allowMissing bool) (*ResolveResult, error) {
	return op.op.Resolve(reference, allowMissing)
}

func (op *resolveOperation) resolveReferenceWithDynamicDates(reference string, allowDynamicMissing bool) (*ResolveResult, error) {
	return op.op.ResolveDynamic(reference, allowDynamicMissing)
}

// ResolveReference forwards to the vault-aware reference resolver.
func ResolveReference(reference string, rt *Runtime, allowMissing bool) (*ResolveResult, error) {
	return refresolve.Resolve(reference, rt, allowMissing)
}

// ResolveReferenceWithDynamicDates forwards to the vault-aware reference
// resolver with relative-date support.
func ResolveReferenceWithDynamicDates(reference string, rt *Runtime, allowDynamicMissing bool) (*ResolveResult, error) {
	return refresolve.ResolveDynamic(reference, rt, allowDynamicMissing)
}
