//go:build integration

package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_CanonicalGroupDefaultsMatchExplicitLeafCommands(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithRavenYAML(`auto_reindex: false
protected_prefixes:
  - private/
directories:
  daily: journal
  type: types
  template: templates/custom
capture:
  destination: inbox.md
  heading: '## Captured'
deletion:
  behavior: trash
  trash_dir: archive/trash
`).
		Build()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	configContent := fmt.Sprintf(`default_vault = "work"

[vaults]
work = %q
archive = %q
`, v.Path, filepath.Join(t.TempDir(), "archive"))
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(statePath, []byte("active_vault = \"work\"\n"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	rootFlags := []string{"--config", configPath, "--state", statePath}
	cases := []struct {
		name       string
		parentArgs []string
		leafArgs   []string
	}{
		{
			name:       "config defaults to show",
			parentArgs: append(append([]string{}, rootFlags...), "config"),
			leafArgs:   append(append([]string{}, rootFlags...), "config", "show"),
		},
		{
			name:       "vault defaults to list",
			parentArgs: append(append([]string{}, rootFlags...), "vault"),
			leafArgs:   append(append([]string{}, rootFlags...), "vault", "list"),
		},
		{
			name:       "vault config defaults to show",
			parentArgs: append(append([]string{}, rootFlags...), "vault", "config"),
			leafArgs:   append(append([]string{}, rootFlags...), "vault", "config", "show"),
		},
		{
			name:       "vault config protected-prefixes defaults to list",
			parentArgs: append(append([]string{}, rootFlags...), "vault", "config", "protected-prefixes"),
			leafArgs:   append(append([]string{}, rootFlags...), "vault", "config", "protected-prefixes", "list"),
		},
		{
			name:       "vault config directories defaults to get",
			parentArgs: append(append([]string{}, rootFlags...), "vault", "config", "directories"),
			leafArgs:   append(append([]string{}, rootFlags...), "vault", "config", "directories", "get"),
		},
		{
			name:       "vault config capture defaults to get",
			parentArgs: append(append([]string{}, rootFlags...), "vault", "config", "capture"),
			leafArgs:   append(append([]string{}, rootFlags...), "vault", "config", "capture", "get"),
		},
		{
			name:       "vault config deletion defaults to get",
			parentArgs: append(append([]string{}, rootFlags...), "vault", "config", "deletion"),
			leafArgs:   append(append([]string{}, rootFlags...), "vault", "config", "deletion", "get"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parent := v.RunCLI(tc.parentArgs...).MustSucceed(t)
			leaf := v.RunCLI(tc.leafArgs...).MustSucceed(t)
			assertCLIResultsEquivalent(t, parent, leaf)
		})
	}
}

func assertCLIResultsEquivalent(t *testing.T, got, want *testutil.CLIResult) {
	t.Helper()

	if got.OK != want.OK {
		t.Fatalf("ok = %v, want %v\nparent=%s\nleaf=%s", got.OK, want.OK, got.RawJSON, want.RawJSON)
	}
	if got.ExitCode != want.ExitCode {
		t.Fatalf("exit code = %d, want %d\nparent=%s\nleaf=%s", got.ExitCode, want.ExitCode, got.RawJSON, want.RawJSON)
	}
	if !reflect.DeepEqual(got.Data, want.Data) {
		t.Fatalf("data mismatch\nparent=%s\nleaf=%s", got.RawJSON, want.RawJSON)
	}
	if !reflect.DeepEqual(got.Error, want.Error) {
		t.Fatalf("error mismatch\nparent=%s\nleaf=%s", got.RawJSON, want.RawJSON)
	}
	if !reflect.DeepEqual(got.Warnings, want.Warnings) {
		t.Fatalf("warnings mismatch\nparent=%s\nleaf=%s", got.RawJSON, want.RawJSON)
	}
	if !reflect.DeepEqual(got.Meta, want.Meta) {
		t.Fatalf("meta mismatch\nparent=%s\nleaf=%s", got.RawJSON, want.RawJSON)
	}
}
