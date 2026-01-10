package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeExecutableScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestExecuteRvnTreatsOkFalseAsErrorEvenWithExit0(t *testing.T) {
	// Skip on Windows just in case; Raven targets mac/linux.
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on windows in this test")
	}

	tmp := t.TempDir()
	script := writeExecutableScript(t, tmp, "fake-rvn.sh", `#!/bin/sh
echo '{"ok":false,"error":{"code":"REQUIRED_FIELD_MISSING","message":"Missing required fields: name","suggestion":"Provide field: {name: ...}"}}'
exit 0
`)

	s := &Server{vaultPath: tmp, executable: script}
	out, isErr := s.executeRvn([]string{"new", "--json", "--", "person", "Freya"})
	if !isErr {
		t.Fatalf("expected isError=true, got false; out=%s", out)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if okVal, _ := parsed["ok"].(bool); okVal {
		t.Fatalf("expected ok=false, got ok=true; out=%s", out)
	}
}

func TestExecuteRvnWrapsNonJSONOutputOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on windows in this test")
	}

	tmp := t.TempDir()
	script := writeExecutableScript(t, tmp, "fake-rvn.sh", `#!/bin/sh
echo "something went wrong" 1>&2
exit 1
`)

	s := &Server{vaultPath: tmp, executable: script}
	out, isErr := s.executeRvn([]string{"new", "--json", "--", "person", "Freya"})
	if !isErr {
		t.Fatalf("expected isError=true, got false; out=%s", out)
	}

	var parsed struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details struct {
				Output string `json:"output"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if parsed.OK {
		t.Fatalf("expected ok=false, got ok=true; out=%s", out)
	}
	if parsed.Error.Code != "EXECUTION_ERROR" {
		t.Fatalf("expected error.code=EXECUTION_ERROR, got %q; out=%s", parsed.Error.Code, out)
	}
	if parsed.Error.Details.Output == "" {
		t.Fatalf("expected error.details.output to be present; out=%s", out)
	}
}

