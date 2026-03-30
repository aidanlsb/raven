//go:build integration

package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
		output, _ := cmd.CombinedOutput()

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
		stateFile := filepath.Join(root, "state.toml")
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

		resp := run(t, "--config", configFile, "--state", stateFile, "--vault", "missing", "--json", "vault", "path")
		if resp.Error.Code != "VAULT_NOT_FOUND" {
			t.Fatalf("expected VAULT_NOT_FOUND, got %q", resp.Error.Code)
		}
		if resp.Error.Suggestion == "" {
			t.Fatal("expected suggestion for unknown named vault")
		}
	})

	t.Run("no vault configured", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		configFile := filepath.Join(root, "config.toml")
		stateFile := filepath.Join(root, "state.toml")
		if err := os.WriteFile(configFile, []byte(""), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		resp := run(t, "--config", configFile, "--state", stateFile, "--json", "vault", "path")
		if resp.Error.Code != "VAULT_NOT_SPECIFIED" {
			t.Fatalf("expected VAULT_NOT_SPECIFIED, got %q", resp.Error.Code)
		}
		if resp.Error.Suggestion == "" {
			t.Fatal("expected suggestion when no vault is configured")
		}
	})
}
