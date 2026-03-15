package toolexec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExecute_ExecutableRequired(t *testing.T) {
	_, err := Execute("", "", "raven_stats", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var execErr *Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if execErr.Code != CodeExecutableRequired {
		t.Fatalf("code=%s, want %s", execErr.Code, CodeExecutableRequired)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	_, err := Execute("", "/bin/echo", "raven_not_a_tool", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var execErr *Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if execErr.Code != CodeUnknownTool {
		t.Fatalf("code=%s, want %s", execErr.Code, CodeUnknownTool)
	}
}

func TestExecute_InvalidJSONOutput(t *testing.T) {
	_, err := Execute("", "/bin/echo", "raven_stats", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var execErr *Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if execErr.Code != CodeInvalidJSON {
		t.Fatalf("code=%s, want %s", execErr.Code, CodeInvalidJSON)
	}
}

func TestExecute_EnvelopeError(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-rvn.sh")
	script := "#!/bin/sh\n" +
		"echo '{\"ok\":false,\"error\":{\"code\":\"X\",\"message\":\"bad\"}}'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	_, err := Execute("", scriptPath, "raven_stats", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var execErr *Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if execErr.Code != CodeToolReturnedError {
		t.Fatalf("code=%s, want %s", execErr.Code, CodeToolReturnedError)
	}
}

func TestIsCode(t *testing.T) {
	err := &Error{Code: CodeUnknownTool}
	if !IsCode(err, CodeUnknownTool) {
		t.Fatal("expected IsCode to match")
	}
	if IsCode(err, CodeInvalidJSON) {
		t.Fatal("expected IsCode mismatch")
	}
}
