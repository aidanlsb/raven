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

func TestIntegration_VaultConfigDirectoriesLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "directories", "set", "--daily=journal", "--type=types", "--template=templates/custom")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "directories:")
	v.AssertFileContains("raven.yaml", "daily: journal/")
	v.AssertFileContains("raven.yaml", "type: types/")
	v.AssertFileContains("raven.yaml", "template: templates/custom/")

	result = v.RunCLI("vault", "config", "directories", "unset", "--daily", "--type", "--template")
	result.MustSucceed(t)
	v.AssertFileNotContains("raven.yaml", "directories:")
}

func TestIntegration_VaultConfigCaptureLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "capture", "set", "--destination=inbox.md", "--heading=## Captured")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "capture:")
	v.AssertFileContains("raven.yaml", "destination: inbox.md")
	v.AssertFileContains("raven.yaml", `heading: '## Captured'`)

	result = v.RunCLI("vault", "config", "capture", "unset", "--destination", "--heading")
	result.MustSucceed(t)
	v.AssertFileNotContains("raven.yaml", "capture:")
}

func TestIntegration_VaultConfigDeletionLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "deletion", "set", "--behavior=permanent", "--trash-dir=archive/trash")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "deletion:")
	v.AssertFileContains("raven.yaml", "behavior: permanent")
	v.AssertFileContains("raven.yaml", "trash_dir: archive/trash")

	result = v.RunCLI("vault", "config", "deletion", "unset", "--behavior", "--trash-dir")
	result.MustSucceed(t)
	v.AssertFileNotContains("raven.yaml", "deletion:")
}
