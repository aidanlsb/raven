package cli

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestFinishCanonicalLeafSuccessUsesHumanRenderer(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() {
		jsonOutput = prevJSON
	})
	jsonOutput = false

	var rendered bool
	err := finishCanonicalLeaf(&cobra.Command{}, commandexec.Result{OK: true}, func(_ *cobra.Command, _ commandexec.Result) error {
		rendered = true
		return nil
	}, func(_ *cobra.Command, _ commandexec.Result) error {
		return errors.New("failure handler called on successful result")
	})
	if err != nil {
		t.Fatalf("finishCanonicalLeaf() error = %v", err)
	}
	if !rendered {
		t.Fatal("expected human renderer to run")
	}
}

func TestFinishCanonicalLeafFailureUsesErrorHandler(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() {
		jsonOutput = prevJSON
	})
	jsonOutput = false

	want := errors.New("handled failure")
	err := finishCanonicalLeaf(&cobra.Command{}, commandexec.Result{OK: false}, func(_ *cobra.Command, _ commandexec.Result) error {
		t.Fatal("human renderer must not run on failure")
		return nil
	}, func(_ *cobra.Command, _ commandexec.Result) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("finishCanonicalLeaf() error = %v, want %v", err, want)
	}
}

func TestCanonicalLeafAnnotations(t *testing.T) {
	defaultCmd := newCanonicalLeafCommand("docs_fetch", canonicalLeafOptions{})
	if defaultCmd.Annotations[canonicalLeafAnnotationKey] != "true" {
		t.Fatal("default leaf missing canonical annotation")
	}
	if defaultCmd.Annotations[exceptionLeafAnnotationKey] == "true" {
		t.Fatal("default leaf must not be marked as an exception")
	}

	exceptionCmd := newExceptionLeafCommand("docs_fetch", exceptionLeafOptions{})
	if exceptionCmd.Annotations[canonicalLeafAnnotationKey] != "true" {
		t.Fatal("exception leaf missing canonical annotation")
	}
	if exceptionCmd.Annotations[exceptionLeafAnnotationKey] != "true" {
		t.Fatal("exception leaf missing exception annotation")
	}
}

func TestCanonicalVaultPathFollowsRegistryScope(t *testing.T) {
	prev := resolvedVaultPath
	resolvedVaultPath = "/tmp/test-vault"
	t.Cleanup(func() {
		resolvedVaultPath = prev
	})

	if got := canonicalVaultPath("version", nil); got != "" {
		t.Fatalf("version vault path = %q, want empty", got)
	}
	if got := canonicalVaultPath("read", nil); got != "/tmp/test-vault" {
		t.Fatalf("read vault path = %q, want /tmp/test-vault", got)
	}
}
