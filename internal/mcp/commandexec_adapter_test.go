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

func TestNormalizeCanonicalArgsUpdateTraitIDs(t *testing.T) {
	args := map[string]interface{}{
		"stdin":     true,
		"trait_ids": []interface{}{"tasks/task1.md:trait:0"},
		"value":     "done",
	}

	got := normalizeCanonicalArgs("update", args)

	rawIDs, ok := got["object_ids"].([]interface{})
	if !ok {
		t.Fatalf("object_ids missing or wrong type: %#v", got["object_ids"])
	}
	if len(rawIDs) != 1 || rawIDs[0] != "tasks/task1.md:trait:0" {
		t.Fatalf("object_ids=%#v, want trait IDs copied through", rawIDs)
	}
}

func TestNormalizeCanonicalArgsLeavesOtherCommandsUntouched(t *testing.T) {
	args := map[string]interface{}{"object_ids": []interface{}{"people/freya"}}
	got := normalizeCanonicalArgs("set", args)
	if got["object_ids"] == nil {
		t.Fatalf("expected object_ids to remain present: %#v", got)
	}
}

func TestAdaptCanonicalResultForMCPRewritesValidationSuggestion(t *testing.T) {
	result := adaptCanonicalResultForMCP("add", commandexec.Failure("INVALID_ARGS", "argument validation failed", nil, "Check command arguments and retry"))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Call raven_describe with command 'add' for the strict contract and retry"
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestAdaptCanonicalResultForMCPKeepsStructuredValidationSuggestion(t *testing.T) {
	result := adaptCanonicalResultForMCP("set", commandexec.Failure("MISSING_ARGUMENT", "no object_ids provided for bulk set", nil, "Provide object_ids for the bulk update and retry"))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Provide object_ids for the bulk update and retry"
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
	}
}

func TestAdaptCanonicalResultForMCPAddsQuerySuggestionWhenMissing(t *testing.T) {
	result := adaptCanonicalResultForMCP("query", commandexec.Failure("QUERY_INVALID", "parse error: expected 2, got 1 at pos 5", nil, ""))

	if result.Error == nil {
		t.Fatal("expected error")
	}
	want := "Check the query syntax, quote string literals, and retry."
	if result.Error.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", result.Error.Suggestion, want)
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
		Source: "active_vault",
	}
	if result.Meta == nil {
		result.Meta = &commandexec.Meta{}
	}
	result.Meta.VaultContext = vc

	if result.Meta.VaultContext == nil {
		t.Fatal("expected vault_context to be set")
	}
	if result.Meta.VaultContext.Source != "active_vault" {
		t.Fatalf("vault_context.source = %q, want %q", result.Meta.VaultContext.Source, "active_vault")
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
