package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestCompactDescribeReturnsContract(t *testing.T) {
	t.Parallel()
	server := NewServer("")
	out, isErr := server.callCompactDescribe(map[string]interface{}{"command": "query"})
	if isErr {
		t.Fatalf("describe returned error: %s", out)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Command     string `json:"command"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			CLIUsage    string `json:"cli_usage"`
			ReadOnly    bool   `json:"read_only"`
			Invokable   bool   `json:"invokable"`
			SchemaHash  string `json:"schema_hash"`
			ArgsSchema  struct {
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
	if !strings.Contains(envelope.Data.Description, "Query syntax:") {
		t.Fatalf("expected query DSL syntax in describe response: %s", out)
	}
	if !strings.Contains(envelope.Data.Description, "type:<type>") {
		t.Fatalf("expected type query hint in describe response: %s", out)
	}
	if !strings.Contains(envelope.Data.Description, "content(\"text\")") {
		t.Fatalf("expected content predicate hint in describe response: %s", out)
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
	if envelope.Data.InvokeShape.Wrapper == "" {
		t.Fatalf("expected invoke wrapper note metadata: %s", out)
	}
	if len(envelope.Data.ArgsSchema.Required) != 1 || envelope.Data.ArgsSchema.Required[0] != "query_string" {
		t.Fatalf("expected query_string to be required in compact schema: %s", out)
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
		t.Fatalf("expected invoke example args to include required query_string: %s", out)
	}
}

func TestCompactDescribeUpdateUsesTraitIDsForBulkArgs(t *testing.T) {
	t.Parallel()

	server := NewServer("")
	out, isErr := server.callCompactDescribe(map[string]interface{}{"command": "update"})
	if isErr {
		t.Fatalf("describe returned error: %s", out)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ArgsSchema struct {
				Properties map[string]interface{} `json:"properties"`
			} `json:"args_schema"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal describe response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got: %s", out)
	}
	if _, ok := envelope.Data.ArgsSchema.Properties["trait_ids"]; !ok {
		t.Fatalf("expected trait_ids in update args schema: %s", out)
	}
	if _, ok := envelope.Data.ArgsSchema.Properties["object_ids"]; ok {
		t.Fatalf("did not expect object_ids in update args schema: %s", out)
	}
}

func TestCompactInvokeListsSavedQueriesWithDedicatedCommand(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`queries:
  open-projects:
    query: "type:project .status==active"
    description: "Active projects"
`).
		Build()
	server := NewServer(v.Path)

	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "query_saved_list",
	})
	if isErr {
		t.Fatalf("expected query_saved_list to succeed, got: %s", out)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Queries []map[string]interface{} `json:"queries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal invoke response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got: %s", out)
	}
	if len(envelope.Data.Queries) != 1 {
		t.Fatalf("expected 1 saved query, got %d", len(envelope.Data.Queries))
	}
	if envelope.Data.Queries[0]["name"] != "open-projects" {
		t.Fatalf("unexpected saved query list: %#v", envelope.Data.Queries)
	}
}

func TestCompactInvokeRejectsInvalidArgumentTypes(t *testing.T) {
	t.Parallel()
	server := newServerWithoutDefaultVault(t)
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "query",
		"args": map[string]interface{}{
			"query_string": "type:project",
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
	t.Parallel()
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

func TestCompactDescribeRejectsLegacyCommandAlias(t *testing.T) {
	t.Parallel()
	server := NewServer("")
	out, isErr := server.callCompactDescribe(map[string]interface{}{"command": "raven_query"})
	if !isErr {
		t.Fatalf("expected describe error, got: %s", out)
	}

	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal describe error response: %v", err)
	}
	if envelope.Error.Code != "COMMAND_NOT_FOUND" {
		t.Fatalf("error.code=%q, want COMMAND_NOT_FOUND; response=%s", envelope.Error.Code, out)
	}
}

func TestCompactInvokeRejectsLegacyCommandAlias(t *testing.T) {
	t.Parallel()
	server := NewServer("")
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "raven_query",
		"args": map[string]interface{}{
			"query_string": "type:project",
		},
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
		t.Fatalf("unmarshal invoke error response: %v", err)
	}
	if envelope.Error.Code != "COMMAND_NOT_FOUND" {
		t.Fatalf("error.code=%q, want COMMAND_NOT_FOUND; response=%s", envelope.Error.Code, out)
	}
}

func TestCompactInvokeSuccess(t *testing.T) {
	t.Parallel()
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

func TestCompactInvokeUsesWrapperVaultPathOverride(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	server := NewServer("")
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command":    "new",
		"vault_path": v.Path,
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

func TestCompactInvokeUsesWrapperVaultNameOverride(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	configContent := fmt.Sprintf("[vaults]\nwork = %q\n", v.Path)
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(""), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	server := NewServerWithBaseArgs([]string{"--config", configPath, "--state", statePath})
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "new",
		"vault":   "work",
		"args": map[string]interface{}{
			"type":  "person",
			"title": "Bob",
		},
	})
	if isErr {
		t.Fatalf("invoke returned error: %s", out)
	}

	if !v.FileExists("people/bob.md") {
		t.Fatal("expected people/bob.md to exist")
	}
}

func TestCompactInvokeRejectsConflictingVaultOverrides(t *testing.T) {
	t.Parallel()
	server := NewServer("")
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command":    "query",
		"vault":      "work",
		"vault_path": "/tmp/work",
		"args": map[string]interface{}{
			"query_string": "type:project",
		},
	})
	if !isErr {
		t.Fatalf("expected invoke error, got: %s", out)
	}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Issues []struct {
					Field string `json:"field"`
					Code  string `json:"code"`
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
	if len(envelope.Error.Details.Issues) != 1 {
		t.Fatalf("expected one issue, got %d; response=%s", len(envelope.Error.Details.Issues), out)
	}
	if envelope.Error.Details.Issues[0].Field != "vault_path" {
		t.Fatalf("issue.field=%q, want vault_path; response=%s", envelope.Error.Details.Issues[0].Field, out)
	}
}

func TestCompactInvokeHintsForTopLevelCommandArgs(t *testing.T) {
	t.Parallel()
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

func TestCompactInvokeHintsForQuerySavedArgument(t *testing.T) {
	t.Parallel()
	server := newServerWithoutDefaultVault(t)
	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "query",
		"args": map[string]interface{}{
			"saved": "issues",
		},
	})
	if !isErr {
		t.Fatalf("expected invoke error, got: %s", out)
	}

	var envelope struct {
		Error struct {
			Code   string `json:"code"`
			Detail struct {
				Issues []struct {
					Field string `json:"field"`
					Code  string `json:"code"`
					Hint  string `json:"hint"`
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

	var savedHint, queryStringHint string
	for _, issue := range envelope.Error.Detail.Issues {
		switch issue.Field {
		case "saved":
			savedHint = issue.Hint
		case "query_string":
			queryStringHint = issue.Hint
		}
	}

	if savedHint != "Pass the saved query name as args.query_string and any saved-query parameters in args.inputs." {
		t.Fatalf("saved hint=%q; response=%s", savedHint, out)
	}
	if queryStringHint != "Use args.query_string for either raw RQL or a saved query name." {
		t.Fatalf("query_string hint=%q; response=%s", queryStringHint, out)
	}
}

func TestCompactInvokeCommandValidationMatchesCanonicalInvoker(t *testing.T) {
	t.Parallel()
	server := newServerWithoutDefaultVault(t)
	rawArgs := map[string]interface{}{
		"saved": "issues",
	}

	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "query",
		"args":    rawArgs,
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

	invokerResult := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "query",
		Caller:    commandexec.CallerMCP,
		Args: map[string]interface{}{
			"saved": "issues",
		},
	})
	if invokerResult.OK || invokerResult.Error == nil {
		t.Fatalf("expected canonical invoker validation failure, got %#v", invokerResult)
	}
	if invokerResult.Error.Code != "INVALID_ARGS" {
		t.Fatalf("invoker error.code=%q, want INVALID_ARGS", invokerResult.Error.Code)
	}

	details, ok := invokerResult.Error.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("invoker details type = %T, want map[string]interface{}", invokerResult.Error.Details)
	}
	canonicalIssues, ok := details["issues"].([]commands.ValidationIssue)
	if !ok {
		t.Fatalf("invoker issues type = %T, want []commands.ValidationIssue", details["issues"])
	}

	if !reflect.DeepEqual(compactInvokeIssueSignatures(envelope.Error.Detail.Issues), canonicalIssueSignatures(canonicalIssues)) {
		t.Fatalf("compact invoke issues did not match canonical invoker\ncompact=%#v\ncanonical=%#v", envelope.Error.Detail.Issues, canonicalIssues)
	}

	var savedHint, queryStringHint string
	for _, issue := range envelope.Error.Detail.Issues {
		switch issue.Field {
		case "saved":
			savedHint = issue.Hint
		case "query_string":
			queryStringHint = issue.Hint
		}
	}
	if savedHint == "" || queryStringHint == "" {
		t.Fatalf("expected MCP-specific hints on invoke issues, got %#v", envelope.Error.Detail.Issues)
	}
}

func compactInvokeIssueSignatures(issues []struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Field+"|"+issue.Code+"|"+issue.Message)
	}
	sort.Strings(out)
	return out
}

func canonicalIssueSignatures(issues []commands.ValidationIssue) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Field+"|"+issue.Code+"|"+issue.Message)
	}
	sort.Strings(out)
	return out
}

func newServerWithoutDefaultVault(t *testing.T) *Server {
	t.Helper()

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	statePath := filepath.Join(tmp, "state.toml")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(""), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	return NewServerWithBaseArgs([]string{"--config", configPath, "--state", statePath})
}
