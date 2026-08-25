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

func TestResolve_ObjectFound(t *testing.T) {
	t.Parallel()

	content := `---
type: book
---

A book.
`

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("books/dune.md", content).
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	result, err := Resolve("books/dune", rt, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if result.ObjectID != "books/dune" {
		t.Errorf("ObjectID = %q, want %q", result.ObjectID, "books/dune")
	}
}

func TestResolve_RefNotFound(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	result, err := Resolve("nonexistent", rt, false)
	if result != nil {
		t.Errorf("result = %#v, want nil", result)
	}
	if !IsRefNotFound(err) {
		t.Errorf("error type = %T, want RefNotFoundError", err)
	}
}
