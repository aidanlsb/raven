//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_VaultConfigShow(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithRavenYAML("auto_reindex: false\nprotected_prefixes:\n  - private/\n").
		Build()

	result := v.RunCLI("vault", "config", "show")
	result.MustSucceed(t)

	if got := result.Data["auto_reindex"]; got != false {
		t.Fatalf("expected auto_reindex=false, got %#v", got)
	}
	if got := result.Data["auto_reindex_explicit"]; got != true {
		t.Fatalf("expected auto_reindex_explicit=true, got %#v", got)
	}

	prefixes := result.DataList("protected_prefixes")
	if len(prefixes) != 1 {
		t.Fatalf("expected one protected prefix, got %#v", prefixes)
	}
}

func TestIntegration_VaultConfigProtectedPrefixesLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "protected-prefixes", "add", "private")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "protected_prefixes:")
	v.AssertFileContains("raven.yaml", "- private/")

	result = v.RunCLI("vault", "config", "protected-prefixes", "remove", "private/")
	result.MustSucceed(t)
	v.AssertFileNotContains("raven.yaml", "- private/")
}

func TestIntegration_VaultConfigAutoReindexLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "auto-reindex", "set", "--value=false")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "auto_reindex: false")

	result = v.RunCLI("vault", "config", "auto-reindex", "unset")
	result.MustSucceed(t)
	v.AssertFileNotContains("raven.yaml", "auto_reindex:")
}
