//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_VaultConfigShow(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithRavenYAML("auto_reindex: false\nassets:\n  root: resources/assets/\n  kinds:\n    image:\n      extensions: [svg]\n      default_path: images/\nprotected_prefixes:\n  - private/\nexclude:\n  - AGENTS.md\n").
		Build()

	result := v.RunCLI("vault", "config", "show")
	result.MustSucceed(t)

	if got := result.Data["auto_reindex"]; got != false {
		t.Fatalf("expected auto_reindex=false, got %#v", got)
	}
	if got := result.Data["auto_reindex_explicit"]; got != true {
		t.Fatalf("expected auto_reindex_explicit=true, got %#v", got)
	}
	assets, ok := result.Data["assets"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected assets map, got %#v", result.Data["assets"])
	}
	if got := assets["root"]; got != "resources/assets/" {
		t.Fatalf("expected assets.root resources/assets/, got %#v", got)
	}

	prefixes := result.DataList("protected_prefixes")
	if len(prefixes) != 1 {
		t.Fatalf("expected one protected prefix, got %#v", prefixes)
	}
	exclude := result.DataList("exclude")
	if len(exclude) != 1 {
		t.Fatalf("expected one exclude pattern, got %#v", exclude)
	}
}

func TestIntegration_VaultConfigAssetsLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "assets", "show")
	result.MustSucceed(t)
	assets, ok := result.Data["assets"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected assets map, got %#v", result.Data["assets"])
	}
	if got := assets["root"]; got != "assets/" {
		t.Fatalf("expected default assets root, got %#v", got)
	}

	result = v.RunCLI("vault", "config", "assets", "set", "--root=resources/assets")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "assets:")
	v.AssertFileContains("raven.yaml", "root: resources/assets/")
	if got := result.Data["reindex_required"]; got != true {
		t.Fatalf("expected reindex_required=true, got %#v", got)
	}

	result = v.RunCLI("vault", "config", "assets", "kind", "set", "image", "--extensions=svg,png", "--media-types=image/svg+xml", "--default-path=images")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "image:")
	v.AssertFileContains("raven.yaml", "- png")
	v.AssertFileContains("raven.yaml", "- svg")
	v.AssertFileContains("raven.yaml", "default_path: images/")
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

func TestIntegration_VaultConfigExcludeLifecycle(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("vault", "config", "exclude", "add", ".cursor/")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "exclude:")
	v.AssertFileContains("raven.yaml", "- .cursor/")

	result = v.RunCLI("vault", "config", "exclude", "remove", ".cursor/")
	result.MustSucceed(t)
	v.AssertFileNotContains("raven.yaml", "- .cursor/")
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
