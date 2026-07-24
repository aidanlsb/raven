package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

func TestAdaptCanonicalResultForMCPRewritesValidationSuggestion(t *testing.T) {
	result := adaptCanonicalResultForMCP("add", nil, commandexec.Failure("INVALID_ARGS", "argument validation failed", nil, "Check command arguments and retry"))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Call raven_describe with command 'add' for the strict contract and retry"
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestAdaptCanonicalResultForMCPKeepsStructuredValidationSuggestion(t *testing.T) {
	result := adaptCanonicalResultForMCP("set", nil, commandexec.Failure("MISSING_ARGUMENT", "no references provided for bulk set", nil, "Provide references for the bulk update and retry"))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Provide references for the bulk update and retry"
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestAdaptCanonicalResultForMCPAddsQuerySuggestionWhenMissing(t *testing.T) {
	result := adaptCanonicalResultForMCP("query", nil, commandexec.Failure("QUERY_INVALID", "parse error: expected 2, got 1 at pos 5", nil, ""))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Check the query syntax, quote string literals, and retry."
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestAdaptCanonicalResultForMCPAddsQueryArgumentHints(t *testing.T) {
	result := adaptCanonicalResultForMCP("query", map[string]interface{}{
		"saved": "issues",
	}, commandexec.Failure("INVALID_ARGS", "argument validation failed", map[string]interface{}{
		"command": "query",
		"issues": []validationIssue{
			{Field: "saved", Code: "UNKNOWN_ARGUMENT", Message: "unknown argument"},
			{Field: "query_string", Code: "MISSING_REQUIRED_ARGUMENT", Message: "required argument is missing"},
		},
	}, "Check command arguments and retry"))

	if result.Error == nil {
		t.Fatal("expected error")
	}

	details, ok := result.Error.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("details type = %T, want map[string]interface{}", result.Error.Details)
	}
	issues, ok := details["issues"].([]validationIssue)
	if !ok {
		t.Fatalf("issues type = %T, want []validationIssue", details["issues"])
	}
	if _, ok := details["args_schema"].(map[string]interface{}); !ok {
		t.Fatalf("args_schema type = %T, want map[string]interface{}", details["args_schema"])
	}
	if _, ok := details["invoke_shape"].(map[string]interface{}); !ok {
		t.Fatalf("invoke_shape type = %T, want map[string]interface{}", details["invoke_shape"])
	}
	if details["schema_hash"] == "" {
		t.Fatalf("expected schema_hash in details: %#v", details)
	}

	if len(issues) != 2 {
		t.Fatalf("issues len = %d, want 2", len(issues))
	}
	if issues[0].Hint != "Pass the saved query name as args.query_string and any saved-query parameters in args.inputs." {
		t.Fatalf("saved hint = %q", issues[0].Hint)
	}
	if issues[1].Hint != "Use args.query_string for either raw RQL or a saved query name." {
		t.Fatalf("query_string hint = %q", issues[1].Hint)
	}
}

func TestCommandVaultRequirementsFollowRegistryMetadata(t *testing.T) {
	t.Parallel()

	if commands.RequiresVault("version") {
		t.Fatal("version should not require vault resolution")
	}
	if commands.RequiresVault("config_show") {
		t.Fatal("config_show should not require vault resolution")
	}
	if commands.RequiresVault("docs") || commands.RequiresVault("docs_fetch") {
		t.Fatal("docs commands should not require vault resolution")
	}
	if !commands.RequiresVault("query") {
		t.Fatal("query should require vault resolution")
	}
}

func TestVaultContextInjectedIntoMeta(t *testing.T) {
	t.Parallel()

	result := commandexec.Success(map[string]interface{}{"items": []string{}}, &commandexec.Meta{Count: 3, QueryTimeMs: 42})

	vc := &commandexec.VaultContext{
		Name:   "work",
		Path:   "/tmp/work-vault",
		Source: "vault",
	}
	result.Meta.VaultContext = vc

	if result.Meta.VaultContext == nil {
		t.Fatal("expected vault_context to be set on meta")
	}
	if result.Meta.VaultContext.Name != "work" {
		t.Fatalf("vault_context.name = %q, want %q", result.Meta.VaultContext.Name, "work")
	}
	if result.Meta.VaultContext.Path != "/tmp/work-vault" {
		t.Fatalf("vault_context.path = %q, want %q", result.Meta.VaultContext.Path, "/tmp/work-vault")
	}
	if result.Meta.VaultContext.Source != "vault" {
		t.Fatalf("vault_context.source = %q, want %q", result.Meta.VaultContext.Source, "vault")
	}
	// Existing meta fields preserved
	if result.Meta.Count != 3 {
		t.Fatalf("meta.count = %d, want 3", result.Meta.Count)
	}
	if result.Meta.QueryTimeMs != 42 {
		t.Fatalf("meta.query_time_ms = %d, want 42", result.Meta.QueryTimeMs)
	}
}

func TestVaultContextNilMetaCreated(t *testing.T) {
	t.Parallel()

	result := commandexec.Success(map[string]interface{}{}, nil)

	if result.Meta != nil {
		t.Fatal("expected meta to be nil initially")
	}

	// Simulate what callCanonicalCommand does when meta is nil
	vc := &commandexec.VaultContext{
		Path:   "/tmp/notes",
		Source: "vault",
	}
	if result.Meta == nil {
		result.Meta = &commandexec.Meta{}
	}
	result.Meta.VaultContext = vc

	if result.Meta.VaultContext == nil {
		t.Fatal("expected vault_context to be set")
	}
	if result.Meta.VaultContext.Source != "vault" {
		t.Fatalf("vault_context.source = %q, want %q", result.Meta.VaultContext.Source, "vault")
	}
}

func TestCallCanonicalCommandReturnsVaultAmbiguousWithoutExplicitVault(t *testing.T) {
	t.Parallel()

	registry := commandexec.NewHandlerRegistry()
	registry.Register("reindex", func(_ context.Context, _ commandexec.Request) commandexec.Result {
		t.Fatal("handler should not run when vault resolution fails")
		return commandexec.Success(nil, nil)
	})

	server := &Server{invoker: commandexec.NewInvoker(registry, nil)}

	out, isErr, handled := server.callCanonicalCommandWithContext(context.Background(), "reindex", map[string]interface{}{"dry-run": true}, "", "")
	if !handled {
		t.Fatal("expected command to be handled")
	}
	if !isErr {
		t.Fatalf("expected error result, got: %s", out)
	}

	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.OK {
		t.Fatalf("expected ok=false, got true: %s", out)
	}
	if envelope.Error.Code != "VAULT_AMBIGUOUS" {
		t.Fatalf("error code = %q, want VAULT_AMBIGUOUS", envelope.Error.Code)
	}
}

func TestCallCanonicalCommandAllowsExplicitVaultPath(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	registry := commandexec.NewHandlerRegistry()
	registry.Register("reindex", func(_ context.Context, req commandexec.Request) commandexec.Result {
		if req.VaultPath != vaultPath {
			t.Fatalf("handler vault path = %q, want %q", req.VaultPath, vaultPath)
		}
		return commandexec.Success(map[string]interface{}{"ok": true}, nil)
	})

	server := &Server{invoker: commandexec.NewInvoker(registry, nil)}

	out, isErr, handled := server.callCanonicalCommandWithContext(context.Background(), "reindex", map[string]interface{}{"dry-run": true}, "", vaultPath)
	if !handled {
		t.Fatal("expected command to be handled")
	}
	if isErr {
		t.Fatalf("expected success, got error: %s", out)
	}
}

func TestCallCanonicalCommandWithContextPropagatesCancellation(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	registry := commandexec.NewHandlerRegistry()
	registry.Register("reindex", func(ctx context.Context, _ commandexec.Request) commandexec.Result {
		if err := ctx.Err(); err == nil {
			t.Fatal("expected canceled context in canonical handler")
		}
		return commandexec.Failure("CANCELLED", ctx.Err().Error(), nil, "")
	})

	server := &Server{
		vaultPath: vaultPath,
		invoker:   commandexec.NewInvoker(registry, nil),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out, isErr, handled := server.callCanonicalCommandWithContext(ctx, "reindex", map[string]interface{}{"dry-run": true}, "", "")
	if !handled {
		t.Fatal("expected canonical command to be handled")
	}
	if !isErr {
		t.Fatalf("expected canceled command to be marked as error, got output: %s", out)
	}

	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.OK {
		t.Fatalf("expected ok=false, got true: %s", out)
	}
	if envelope.Error.Code != "CANCELLED" {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, "CANCELLED")
	}
	if envelope.Error.Message != "context canceled" {
		t.Fatalf("error message = %q, want %q", envelope.Error.Message, "context canceled")
	}
}
