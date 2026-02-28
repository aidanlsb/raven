package testutil

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	// binaryPath caches the path to the built rvn binary.
	binaryPath string
	buildMu    sync.Mutex
	buildErr   error
)

// CLIResult represents the result of running a CLI command.
type CLIResult struct {
	OK       bool
	Data     map[string]interface{}
	Error    *CLIError
	Warnings []CLIWarning
	Meta     *CLIMeta
	RawJSON  string
	ExitCode int
}

// CLIError represents a structured error from the CLI.
type CLIError struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Suggestion string                 `json:"suggestion,omitempty"`
}

// CLIWarning represents a warning from the CLI.
type CLIWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CLIMeta contains metadata from the response.
type CLIMeta struct {
	Count       int   `json:"count,omitempty"`
	QueryTimeMs int64 `json:"query_time_ms,omitempty"`
}

// BuildCLI builds the rvn binary and returns its path.
// This is called automatically by RunCLI but can be called
// explicitly if you need the binary path for other purposes.
func BuildCLI(t *testing.T) string {
	t.Helper()

	buildMu.Lock()
	defer buildMu.Unlock()

	// Reuse previously built binary if it still exists.
	if binaryPath != "" {
		if _, err := os.Stat(binaryPath); err == nil {
			return binaryPath
		}
		// Binary disappeared (can happen on some Windows runners with temp cleanup).
		binaryPath = ""
		buildErr = nil
	}

	// Find the project root (directory containing go.mod)
	projectRoot, err := findProjectRoot()
	if err != nil {
		buildErr = err
	} else {
		// Build to a temp location.
		tmpDir, err := os.MkdirTemp("", "rvn-cli-bin-*")
		if err != nil {
			buildErr = err
		} else {
			binName := "rvn"
			if runtime.GOOS == "windows" {
				// Avoid relying on extension resolution in os/exec on Windows.
				binName = "rvn.exe"
			}

			binaryPath = filepath.Join(tmpDir, binName)
			cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/rvn")
			cmd.Dir = projectRoot
			output, err := cmd.CombinedOutput()
			if err != nil {
				buildErr = &BuildError{Output: string(output), Err: err}
				binaryPath = ""
			}
		}
	}

	if buildErr != nil {
		t.Fatalf("failed to build CLI: %v", buildErr)
	}

	return binaryPath
}

// BuildError represents an error building the CLI binary.
type BuildError struct {
	Output string
	Err    error
}

func (e *BuildError) Error() string {
	return e.Err.Error() + "\n" + e.Output
}

// findProjectRoot walks up the directory tree to find go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// RunCLI executes a CLI command against the vault and returns the parsed result.
// Commands are run with --json flag automatically.
func (v *TestVault) RunCLI(args ...string) *CLIResult {
	v.t.Helper()

	// Ensure binary is built
	binary := BuildCLI(v.t)

	// Build command args with vault path and json flag
	cmdArgs := []string{"--vault-path", v.Path, "--json"}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(binary, cmdArgs...)
	output, err := cmd.CombinedOutput()

	result := &CLIResult{
		RawJSON: string(output),
	}

	// Get exit code
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	// Parse JSON response
	var resp struct {
		OK       bool                   `json:"ok"`
		Data     map[string]interface{} `json:"data,omitempty"`
		Error    *CLIError              `json:"error,omitempty"`
		Warnings []CLIWarning           `json:"warnings,omitempty"`
		Meta     *CLIMeta               `json:"meta,omitempty"`
	}

	if err := json.Unmarshal(output, &resp); err != nil {
		// If parsing fails, create a synthetic error
		result.OK = false
		result.Error = &CLIError{
			Code:    "PARSE_ERROR",
			Message: "Failed to parse JSON output: " + err.Error(),
			Details: map[string]interface{}{"raw": string(output)},
		}
		return result
	}

	result.OK = resp.OK
	result.Data = resp.Data
	result.Error = resp.Error
	result.Warnings = resp.Warnings
	result.Meta = resp.Meta

	return result
}

// RunCLIWithStdin executes a CLI command with stdin input.
func (v *TestVault) RunCLIWithStdin(stdin string, args ...string) *CLIResult {
	v.t.Helper()

	// Ensure binary is built
	binary := BuildCLI(v.t)

	// Build command args with vault path and json flag
	cmdArgs := []string{"--vault-path", v.Path, "--json"}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(binary, cmdArgs...)
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.CombinedOutput()

	result := &CLIResult{
		RawJSON: string(output),
	}

	// Get exit code
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	// Parse JSON response
	var resp struct {
		OK       bool                   `json:"ok"`
		Data     map[string]interface{} `json:"data,omitempty"`
		Error    *CLIError              `json:"error,omitempty"`
		Warnings []CLIWarning           `json:"warnings,omitempty"`
		Meta     *CLIMeta               `json:"meta,omitempty"`
	}

	if err := json.Unmarshal(output, &resp); err != nil {
		result.OK = false
		result.Error = &CLIError{
			Code:    "PARSE_ERROR",
			Message: "Failed to parse JSON output: " + err.Error(),
			Details: map[string]interface{}{"raw": string(output)},
		}
		return result
	}

	result.OK = resp.OK
	result.Data = resp.Data
	result.Error = resp.Error
	result.Warnings = resp.Warnings
	result.Meta = resp.Meta

	return result
}

// MustSucceed fails the test if the CLI command did not succeed.
func (r *CLIResult) MustSucceed(t *testing.T) *CLIResult {
	t.Helper()
	if !r.OK {
		errMsg := "unknown error"
		if r.Error != nil {
			errMsg = r.Error.Code + ": " + r.Error.Message
		}
		t.Fatalf("expected command to succeed, got error: %s\nRaw output: %s", errMsg, r.RawJSON)
	}
	return r
}

// MustFailWithMessage fails the test if the CLI command succeeded, or if it failed
// without an error message containing the expected substring.
func (r *CLIResult) MustFailWithMessage(t *testing.T, msgSubstr string) *CLIResult {
	t.Helper()
	if r.OK {
		t.Fatalf("expected command to fail, but it succeeded\nRaw output: %s", r.RawJSON)
	}
	if msgSubstr != "" && r.Error != nil {
		if !strings.Contains(r.Error.Message, msgSubstr) && !strings.Contains(r.Error.Suggestion, msgSubstr) {
			t.Errorf("expected error to contain %q, got: %s (suggestion: %s)", msgSubstr, r.Error.Message, r.Error.Suggestion)
		}
	}
	return r
}

// MustFail fails the test if the CLI command did not fail with the expected code.
func (r *CLIResult) MustFail(t *testing.T, expectedCode string) *CLIResult {
	t.Helper()
	if r.OK {
		t.Fatalf("expected command to fail with code %s, but it succeeded\nRaw output: %s", expectedCode, r.RawJSON)
	}
	if r.Error == nil {
		t.Fatalf("expected error with code %s, but error is nil\nRaw output: %s", expectedCode, r.RawJSON)
	}
	if r.Error.Code != expectedCode {
		t.Fatalf("expected error code %s, got %s: %s\nRaw output: %s", expectedCode, r.Error.Code, r.Error.Message, r.RawJSON)
	}
	return r
}

// DataList extracts a list from the Data field.
func (r *CLIResult) DataList(key string) []interface{} {
	if r.Data == nil {
		return nil
	}
	if list, ok := r.Data[key].([]interface{}); ok {
		return list
	}
	return nil
}

// DataString extracts a string from the Data field.
func (r *CLIResult) DataString(key string) string {
	if r.Data == nil {
		return ""
	}
	if s, ok := r.Data[key].(string); ok {
		return s
	}
	return ""
}
