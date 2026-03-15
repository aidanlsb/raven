package toolexec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFakeTool(t *testing.T, dir string, unixName, unixBody, windowsName, windowsBody string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, windowsName)
		if err := os.WriteFile(path, []byte(windowsBody), 0o644); err != nil {
			t.Fatalf("write windows script: %v", err)
		}
		return path
	}

	path := filepath.Join(dir, unixName)
	if err := os.WriteFile(path, []byte(unixBody), 0o755); err != nil {
		t.Fatalf("write unix script: %v", err)
	}
	return path
}

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
	_, err := Execute("", "non-empty-executable", "raven_not_a_tool", nil)
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
	dir := t.TempDir()
	executable := writeFakeTool(
		t,
		dir,
		"fake-rvn.sh",
		"#!/bin/sh\necho not-json\n",
		"fake-rvn.cmd",
		"@echo off\r\necho not-json\r\n",
	)

	_, err := Execute("", executable, "raven_stats", nil)
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
	scriptPath := writeFakeTool(
		t,
		dir,
		"fake-rvn.sh",
		"#!/bin/sh\necho '{\"ok\":false,\"error\":{\"code\":\"X\",\"message\":\"bad\"}}'\n",
		"fake-rvn.cmd",
		"@echo off\r\necho {\"ok\":false,\"error\":{\"code\":\"X\",\"message\":\"bad\"}}\r\n",
	)

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
