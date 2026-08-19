//go:build integration

package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_JSONStartupErrorsUseEnvelope(t *testing.T) {
	t.Parallel()

	binary := testutil.BuildCLI(t)

	type response struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion,omitempty"`
		} `json:"error,omitempty"`
	}

	run := func(t *testing.T, args ...string) response {
		t.Helper()

		cmd := exec.Command(binary, args...)
		output, err := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected startup failure to exit nonzero, got %v\noutput=%s", err, output)
		}
		if got := exitErr.ExitCode(); got != 1 {
			t.Fatalf("exit code = %d, want 1\noutput=%s", got, output)
		}

		var resp response
		if err := json.Unmarshal(output, &resp); err != nil {
			t.Fatalf("expected JSON envelope, got parse error: %v\noutput=%s", err, output)
		}
		if resp.OK {
			t.Fatalf("expected startup failure, got success: %s", output)
		}
		if resp.Error == nil {
			t.Fatalf("expected structured error, got: %s", output)
		}
		return resp
	}

	t.Run("invalid config file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		configFile := filepath.Join(root, "config.toml")
		if err := os.WriteFile(configFile, []byte("default_vault = [\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		resp := run(t, "--config", configFile, "--json", "version")
		if resp.Error.Code != "CONFIG_INVALID" {
			t.Fatalf("expected CONFIG_INVALID, got %q", resp.Error.Code)
		}
	})

	t.Run("unknown named vault", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		configFile := filepath.Join(root, "config.toml")
		vaultPath := filepath.Join(root, "notes")
		if err := os.MkdirAll(vaultPath, 0o755); err != nil {
			t.Fatalf("mkdir vault: %v", err)
		}
		config := `default_vault = "main"

[vaults]
main = "` + vaultPath + `"
`
		if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		resp := run(t, "--config", configFile, "--vault", "missing", "--json", "vault", "path")
		if resp.Error.Code != "VAULT_NOT_FOUND" {
			t.Fatalf("expected VAULT_NOT_FOUND, got %q", resp.Error.Code)
		}
		if resp.Error.Suggestion == "" {
			t.Fatal("expected suggestion for unknown named vault")
		}
	})

	t.Run("missing active vault does not fall back to default", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		configFile := filepath.Join(root, "config.toml")
		stateFile := filepath.Join(root, "state.toml")
		defaultPath := filepath.Join(root, "default")
		explicitPath := filepath.Join(root, "explicit")
		for _, path := range []string{defaultPath, explicitPath} {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("mkdir vault: %v", err)
			}
		}
		configContents := `default_vault = "main"

[vaults]
main = "` + defaultPath + `"
`
		if err := os.WriteFile(configFile, []byte(configContents), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(stateFile, []byte("version = 1\nactive_vault = \"missing\"\n"), 0o644); err != nil {
			t.Fatalf("write state: %v", err)
		}

		resp := run(t, "--config", configFile, "--json", "vault", "list", "--path-only")
		if resp.Error.Code != "VAULT_NOT_FOUND" {
			t.Fatalf("expected VAULT_NOT_FOUND, got %q", resp.Error.Code)
		}
		if !strings.Contains(resp.Error.Message, "active vault 'missing' not found in config") {
			t.Fatalf("unexpected error message: %q", resp.Error.Message)
		}
		if !strings.Contains(resp.Error.Suggestion, "rvn vault use <name>") ||
			!strings.Contains(resp.Error.Suggestion, "rvn vault clear") {
			t.Fatalf("expected active-vault repair guidance, got %q", resp.Error.Suggestion)
		}

		runPath := func(t *testing.T, want string, flags ...string) {
			t.Helper()
			for _, commandArgs := range [][]string{
				{"vault", "list", "--path-only"},
				{"vault", "path"},
			} {
				args := append([]string{"--config", configFile}, flags...)
				args = append(args, "--json")
				args = append(args, commandArgs...)
				output, err := exec.Command(binary, args...).CombinedOutput()
				if err != nil {
					t.Fatalf("%v failed: %v\n%s", commandArgs, err, output)
				}
				var success struct {
					OK   bool                   `json:"ok"`
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(output, &success); err != nil {
					t.Fatalf("unmarshal success response: %v\n%s", err, output)
				}
				if !success.OK || success.Data["path"] != want {
					t.Fatalf("%v response = %s, want path %q", commandArgs, output, want)
				}
			}
		}

		runPath(t, defaultPath, "--vault", "main")
		runPath(t, explicitPath, "--vault-path", explicitPath)

		if err := os.WriteFile(stateFile, []byte("version = 1\nactive_vault = \"main\"\n"), 0o644); err != nil {
			t.Fatalf("write valid active state: %v", err)
		}
		runPath(t, defaultPath)

		if err := os.WriteFile(stateFile, []byte("version = 1\nactive_vault = \"\"\n"), 0o644); err != nil {
			t.Fatalf("clear active state: %v", err)
		}
		runPath(t, defaultPath)
	})

	t.Run("no vault configured", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		configFile := filepath.Join(root, "config.toml")
		if err := os.WriteFile(configFile, []byte(""), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		resp := run(t, "--config", configFile, "--json", "vault", "list", "--path-only")
		if resp.Error.Code != "VAULT_NOT_SPECIFIED" {
			t.Fatalf("expected VAULT_NOT_SPECIFIED, got %q", resp.Error.Code)
		}
		if resp.Error.Suggestion == "" {
			t.Fatal("expected suggestion when no vault is configured")
		}
	})
}
