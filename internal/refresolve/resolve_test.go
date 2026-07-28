package refresolve

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestResolveDoesNotTreatNonMarkdownFileAsIdentity(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("files/paper.pdf", "%PDF").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	result, err := Resolve("files/paper.pdf", rt, false)
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !IsRefNotFound(err) {
		t.Fatalf("error = %v, want RefNotFoundError", err)
	}
}
