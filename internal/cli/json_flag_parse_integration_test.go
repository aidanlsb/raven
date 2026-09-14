//go:build integration

package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_JSONFlagParseErrorsUseEnvelope(t *testing.T) {
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

	run := func(t *testing.T, args ...string) (stdout, stderr string, exitCode int, err error) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		err = cmd.Run()
		exitCode = 0
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		return stdoutBuf.String(), stderrBuf.String(), exitCode, err
	}

	assertJSONFlagParseFailure := func(t *testing.T, stdout, stderr string, exitCode int, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected nonzero exit, got success\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1\nstdout=%s\nstderr=%s", exitCode, stdout, stderr)
		}
		if strings.TrimSpace(stderr) != "" {
			t.Fatalf("expected empty stderr in --json mode, got %q\nstdout=%s", stderr, stdout)
		}
		if strings.TrimSpace(stdout) == "" {
			t.Fatalf("expected JSON error envelope on stdout, got 0 bytes (silent exit)")
		}

		var resp response
		if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
			t.Fatalf("expected JSON envelope, got parse error: %v\nstdout=%s", unmarshalErr, stdout)
		}
		if resp.OK || resp.Error == nil {
			t.Fatalf("expected ok=false with structured error, got: %s", stdout)
		}
		if resp.Error.Code != "INVALID_INPUT" {
			t.Fatalf("expected INVALID_INPUT, got %q\nstdout=%s", resp.Error.Code, stdout)
		}
		if !strings.Contains(resp.Error.Message, "unknown shorthand flag") {
			t.Fatalf("message = %q, want cobra flag-parse text", resp.Error.Message)
		}
		if !strings.Contains(resp.Error.Suggestion, "--") {
			t.Fatalf("expected dash-terminator suggestion, got %q", resp.Error.Suggestion)
		}
	}

	t.Run("add leading dash with --json last", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, exitCode, err := run(t, "add", "- probe a", "--to", "daily/2026-09-03#interviews", "--json")
		assertJSONFlagParseFailure(t, stdout, stderr, exitCode, err)
	})

	t.Run("add leading dash with --json first", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, exitCode, err := run(t, "--json", "add", "- probe a", "--to", "daily/2026-09-03#interviews")
		assertJSONFlagParseFailure(t, stdout, stderr, exitCode, err)
	})

	t.Run("edit leading dash old_str", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, exitCode, err := run(t, "edit", "daily/2026-09-03.md", "- old task", "- new task", "--json")
		assertJSONFlagParseFailure(t, stdout, stderr, exitCode, err)
	})

	t.Run("human mode still prints error", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, exitCode, err := run(t, "add", "- probe a", "--to", "daily/2026-09-03#interviews")
		if err == nil {
			t.Fatalf("expected nonzero exit, got success\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1\nstdout=%s\nstderr=%s", exitCode, stdout, stderr)
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "unknown shorthand flag") {
			t.Fatalf("expected human flag-parse error, got stdout=%q stderr=%q", stdout, stderr)
		}
		if strings.Contains(strings.TrimSpace(stdout), `"ok": false`) {
			t.Fatalf("human mode should not emit a JSON envelope, got stdout=%q", stdout)
		}
	})
}
