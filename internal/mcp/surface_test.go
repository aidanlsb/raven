package mcp

import (
	"encoding/json"
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
			Command    string `json:"command"`
			Summary    string `json:"summary"`
			CLIUsage   string `json:"cli_usage"`
			ReadOnly   bool   `json:"read_only"`
			Invokable  bool   `json:"invokable"`
			SchemaHash string `json:"schema_hash"`
			ArgsSchema struct {
				Required   []string               `json:"required"`
				Properties map[string]interface{} `json:"properties"`
			} `json:"args_schema"`
			InvokeShape struct {
				Wrapper string `json:"wrapper"`
			} `json:"invoke_shape"`
			InvokeExample struct {
				Command    string                 `json:"command"`
				SchemaHash string                 `json:"schema_hash"`
				Args       map[string]interface{} `json:"args"`
			} `json:"invoke_example"`
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
	if envelope.Data.Summary == "" {
		t.Fatalf("expected summary in describe response: %s", out)
	}
	if envelope.Data.CLIUsage != "rvn query <query_string|saved-query> [inputs...]" {
		t.Fatalf("cli_usage=%q, want query usage; response=%s", envelope.Data.CLIUsage, out)
	}
	if !envelope.Data.ReadOnly {
		t.Fatalf("expected query to be read_only: %s", out)
	}
	if !envelope.Data.Invokable {
		t.Fatalf("expected query to be invokable: %s", out)
	}
	if envelope.Data.SchemaHash == "" {
		t.Fatalf("expected schema_hash, got empty response: %s", out)
	}
	if envelope.Data.InvokeShape.Wrapper != "args" {
		t.Fatalf("invoke_shape.wrapper=%q, want args; response=%s", envelope.Data.InvokeShape.Wrapper, out)
	}
	if len(envelope.Data.ArgsSchema.Required) == 0 {
		t.Fatalf("expected required args in compact schema: %s", out)
	}
	if _, ok := envelope.Data.ArgsSchema.Properties["query_string"]; !ok {
		t.Fatalf("expected query_string property in compact schema: %s", out)
	}
	if envelope.Data.InvokeExample.Command != "query" {
		t.Fatalf("invoke_example.command=%q, want query; response=%s", envelope.Data.InvokeExample.Command, out)
	}
	if envelope.Data.InvokeExample.SchemaHash != envelope.Data.SchemaHash {
		t.Fatalf("invoke_example.schema_hash=%q, want %q", envelope.Data.InvokeExample.SchemaHash, envelope.Data.SchemaHash)
	}
	if _, ok := envelope.Data.InvokeExample.Args["query_string"]; !ok {
		t.Fatalf("expected invoke example args to include query_string: %s", out)
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

func TestCompactInvokeHintsForTopLevelCommandArgs(t *testing.T) {
	server := NewServer("")
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "read",
		"path":    "daily/2026-03-17.md",
	})
	if !isErr {
		t.Fatalf("expected invoke error, got: %s", out)
	}

	var envelope struct {
		Error struct {
			Code   string `json:"code"`
			Detail struct {
				Issues []struct {
					Field   string `json:"field"`
					Code    string `json:"code"`
					Message string `json:"message"`
					Hint    string `json:"hint"`
				} `json:"issues"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal invoke error response: %v", err)
	}
	if envelope.Error.Code != "INVALID_ARGS" {
		t.Fatalf("error.code=%q, want INVALID_ARGS; response=%s", envelope.Error.Code, out)
	}
	if len(envelope.Error.Detail.Issues) != 1 {
		t.Fatalf("expected one validation issue, got %d; response=%s", len(envelope.Error.Detail.Issues), out)
	}
	issue := envelope.Error.Detail.Issues[0]
	if issue.Field != "path" {
		t.Fatalf("issue.field=%q, want path; response=%s", issue.Field, out)
	}
	if issue.Hint != "Did you mean args.path? Command-specific parameters must be nested under args." {
		t.Fatalf("issue.hint=%q; response=%s", issue.Hint, out)
	}
}
