package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestCompactDescribeReturnsContract(t *testing.T) {
	server := NewServer("")
	out, isErr := server.callCompactDescribe(map[string]interface{}{"command": "query"})
	if isErr {
		t.Fatalf("describe returned error: %s", out)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Command       string `json:"command"`
			SchemaHash    string `json:"schema_hash"`
			SchemaVersion string `json:"schema_version"`
			Invoke        struct {
				Tool string `json:"tool"`
			} `json:"invoke"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal describe response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got: %s", out)
	}
	if envelope.Data.Command != "query" {
		t.Fatalf("command=%q, want query", envelope.Data.Command)
	}
	if envelope.Data.SchemaHash == "" {
		t.Fatalf("expected schema_hash, got empty response: %s", out)
	}
	if envelope.Data.SchemaVersion != commandContractSchemaVersion {
		t.Fatalf("schema_version=%q, want %q", envelope.Data.SchemaVersion, commandContractSchemaVersion)
	}
	if envelope.Data.Invoke.Tool != compactToolInvoke {
		t.Fatalf("invoke.tool=%q, want %q", envelope.Data.Invoke.Tool, compactToolInvoke)
	}
}

func TestCompactInvokeRejectsInvalidArgumentTypes(t *testing.T) {
	server := NewServer("")
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "query",
		"args": map[string]interface{}{
			"query_string": "object:project",
			"apply":        "set status=done",
		},
	})
	if !isErr {
		t.Fatalf("expected invoke error, got: %s", out)
	}

	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal invoke response: %v", err)
	}
	if envelope.Error.Code != "INVALID_ARGS" {
		t.Fatalf("error.code=%q, want INVALID_ARGS; response=%s", envelope.Error.Code, out)
	}
}

func TestCompactInvokeRejectsNonInvokableCommand(t *testing.T) {
	server := NewServer("")
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "serve",
	})
	if !isErr {
		t.Fatalf("expected invoke error, got: %s", out)
	}

	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal invoke response: %v", err)
	}
	if envelope.Error.Code != "COMMAND_NOT_INVOKABLE" {
		t.Fatalf("error.code=%q, want COMMAND_NOT_INVOKABLE; response=%s", envelope.Error.Code, out)
	}
}

func TestCompactInvokeSuccess(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	server := NewServer(v.Path)

	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "new",
		"args": map[string]interface{}{
			"type":  "person",
			"title": "Alice",
		},
	})
	if isErr {
		t.Fatalf("invoke returned error: %s", out)
	}

	if !v.FileExists("people/alice.md") {
		t.Fatal("expected people/alice.md to exist")
	}
}

func TestCompactInvokeRejectsTopLevelCommandArgumentsWithHint(t *testing.T) {
	server := NewServer("")

	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command":      "query",
		"query_string": "object:project .status==active",
	})
	if !isErr {
		t.Fatalf("expected invoke error, got: %s", out)
	}
	if !strings.Contains(out, `"code":"INVALID_ARGS"`) {
		t.Fatalf("expected INVALID_ARGS, got: %s", out)
	}
	if !strings.Contains(out, "put command parameters inside args") {
		t.Fatalf("expected nested-args hint, got: %s", out)
	}
	if !strings.Contains(out, "UNKNOWN_ARGUMENT") {
		t.Fatalf("expected UNKNOWN_ARGUMENT issue, got: %s", out)
	}
}
