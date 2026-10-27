package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/testutil"
)

const testToken = "test-token-abcdefghijklmnopqrstuvwxyz012345"

func newTestServer(t *testing.T, opts Options) *Server {
	t.Helper()
	if opts.Token == "" {
		opts.Token = testToken
	}
	if opts.Sleep == nil {
		opts.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	server, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return server
}

func doJSON(t *testing.T, handler http.Handler, method, target, token, host, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	if host != "" {
		req.Host = host
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder) commandexec.Result {
	t.Helper()
	var result commandexec.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\nbody: %s", err, rec.Body.String())
	}
	return result
}

func decodeDataMap(t *testing.T, result commandexec.Result) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	return data
}

func isolatedConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.toml")
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := isolatedConfigPath(t)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func getSession(t *testing.T, server *Server) Session {
	t.Helper()
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/session", testToken, "127.0.0.1", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("session failed: %#v", result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		t.Fatalf("decode session: %v\nbody: %s", err, rec.Body.String())
	}
	return session
}

func TestHealthRequiresAuth(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/health", "", "127.0.0.1", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrUnauthorized {
		t.Fatalf("result = %#v, want UNAUTHORIZED", result)
	}
}

func TestHealthRejectsWrongToken(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/health", "wrong-token", "127.0.0.1", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHealthOK(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/health", testToken, "127.0.0.1", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("health failed: %#v", result)
	}
	data := decodeDataMap(t, result)
	if data["status"] != "ok" {
		t.Fatalf("status = %#v, want ok", data["status"])
	}
	if data["api_version"] != Version {
		t.Fatalf("api_version = %#v, want %q", data["api_version"], Version)
	}
	if _, ok := data["editor"]; ok {
		t.Fatal("health must not include editor; use GET /v1/session")
	}
	if _, ok := data["vault_path"]; ok {
		t.Fatal("health must not include vault_path; use GET /v1/session")
	}
}

func TestSessionRequiresAuth(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{ConfigPath: isolatedConfigPath(t)})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/session", "", "127.0.0.1", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrUnauthorized {
		t.Fatalf("result = %#v, want UNAUTHORIZED", result)
	}
}

func TestSessionRejectsWrongToken(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{ConfigPath: isolatedConfigPath(t)})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/session", "wrong-token", "127.0.0.1", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSessionExposesAbsoluteVaultPath(t *testing.T) {
	t.Parallel()
	vaultDir := t.TempDir()
	server := newTestServer(t, Options{
		VaultPath:  vaultDir,
		ConfigPath: isolatedConfigPath(t),
	})
	session := getSession(t, server)
	if session.VaultPath != vaultDir {
		t.Fatalf("vault_path = %q, want %q", session.VaultPath, vaultDir)
	}
	if !filepath.IsAbs(session.VaultPath) {
		t.Fatalf("vault_path %q is not absolute", session.VaultPath)
	}
}

func TestSessionAbsolutizesRelativeVaultPath(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	server := newTestServer(t, Options{
		VaultPath:  "notes",
		ConfigPath: isolatedConfigPath(t),
	})
	session := getSession(t, server)
	want, err := filepath.Abs("notes")
	if err != nil {
		t.Fatal(err)
	}
	if session.VaultPath != want {
		t.Fatalf("vault_path = %q, want %q", session.VaultPath, want)
	}
}

func TestSessionResolvesEditorFromConfig(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	configPath := writeTestConfig(t, "editor = \"code --wait\"\neditor_mode = \"gui\"\n")
	server := newTestServer(t, Options{
		VaultPath:  t.TempDir(),
		ConfigPath: configPath,
	})
	session := getSession(t, server)
	if session.Editor != "code --wait" {
		t.Fatalf("editor = %q, want code --wait", session.Editor)
	}
	if session.EditorMode != "gui" {
		t.Fatalf("editor_mode = %q, want gui", session.EditorMode)
	}
	if session.APIVersion != Version {
		t.Fatalf("api_version = %q, want %q", session.APIVersion, Version)
	}
}

func TestSessionFallsBackToEditorEnv(t *testing.T) {
	t.Setenv("EDITOR", "hx")
	server := newTestServer(t, Options{
		VaultPath:  t.TempDir(),
		ConfigPath: isolatedConfigPath(t),
	})
	session := getSession(t, server)
	if session.Editor != "hx" {
		t.Fatalf("editor = %q, want hx", session.Editor)
	}
	if session.EditorMode != "auto" {
		t.Fatalf("editor_mode = %q, want auto", session.EditorMode)
	}
}

func TestSessionUnknownEditorModeFallsBackToAuto(t *testing.T) {
	t.Setenv("EDITOR", "")
	configPath := writeTestConfig(t, "editor_mode = \"tui\"\n")
	server := newTestServer(t, Options{ConfigPath: configPath})
	session := getSession(t, server)
	if session.EditorMode != "auto" {
		t.Fatalf("editor_mode = %q, want auto", session.EditorMode)
	}
}

func TestHostHeaderRejected(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://evil.example/v1/health", testToken, "evil.example", "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if result.Error == nil || result.Error.Code != codes.ErrInvalidHost {
		t.Fatalf("result = %#v, want INVALID_HOST", result)
	}
}

func TestLoopbackHostsAllowed(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	for _, host := range []string{"127.0.0.1", "127.0.0.1:7438", "localhost", "localhost:7438"} {
		rec := doJSON(t, server.Handler(), http.MethodGet, "http://"+host+"/v1/health", testToken, host, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("host %q status = %d, want 200; body=%s", host, rec.Code, rec.Body.String())
		}
	}
}

func TestInvokeRejectsNonInvokableCommands(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"mcp_serve","args":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrCommandNotInvokable {
		t.Fatalf("result = %#v, want COMMAND_NOT_INVOKABLE", result)
	}
}

func TestInvokeUnknownCommandIsNotFound(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{Invoker: app.CommandInvoker()})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"vault_current","args":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != "COMMAND_NOT_FOUND" {
		t.Fatalf("result = %#v, want COMMAND_NOT_FOUND", result)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{CORSOrigins: []string{"http://localhost:5173"}})
	rec := doJSON(t, server.Handler(), http.MethodOptions, "http://127.0.0.1/v1/health", "", "127.0.0.1", "http://localhost:5173", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}

func TestCORSOmitsDisallowedOrigin(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{CORSOrigins: []string{"http://localhost:5173"}})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/health", testToken, "127.0.0.1", "http://evil.example", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty", got)
	}
}

func TestIndexLockRetryThenSuccess(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	registry := commandexec.NewHandlerRegistry()
	registry.Register("query", func(context.Context, commandexec.Request) commandexec.Result {
		n := calls.Add(1)
		if n < 3 {
			return commandexec.Failure(codes.ErrIndexLocked, "index is locked for rebuild", nil, "retry")
		}
		return commandexec.Success(map[string]interface{}{"items": []interface{}{}}, nil)
	})
	server := newTestServer(t, Options{
		Invoker:      commandexec.NewInvoker(registry, nil),
		RetryBackoff: []time.Duration{time.Millisecond, time.Millisecond},
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"query","args":{"query":"type:page"}}`)
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("expected success after retry, got %#v", result)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestIndexLockRetryForWriteCommand(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	registry := commandexec.NewHandlerRegistry()
	registry.Register("delete", func(context.Context, commandexec.Request) commandexec.Result {
		n := calls.Add(1)
		if n < 2 {
			return commandexec.Failure(codes.ErrIndexLocked, "index is locked for rebuild", nil, "retry")
		}
		return commandexec.Success(map[string]interface{}{"deleted": true}, nil)
	})
	server := newTestServer(t, Options{
		Invoker:      commandexec.NewInvoker(registry, nil),
		RetryBackoff: []time.Duration{time.Millisecond},
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"delete","args":{"reference":"people/freya"}}`)
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("expected success after retry, got %#v", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestIndexLockRetryExhausted(t *testing.T) {
	t.Parallel()
	registry := commandexec.NewHandlerRegistry()
	registry.Register("query", func(context.Context, commandexec.Request) commandexec.Result {
		return commandexec.Failure(codes.ErrIndexLocked, "index is locked for rebuild", nil, "retry")
	})
	server := newTestServer(t, Options{
		Invoker:      commandexec.NewInvoker(registry, nil),
		RetryBackoff: []time.Duration{time.Millisecond},
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"query","args":{"query":"type:page"}}`)
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrIndexLocked {
		t.Fatalf("result = %#v, want INDEX_LOCKED", result)
	}
}

func TestEnvelopeParityWithCLI(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\nQueen of Asgard.\n").
		Build()

	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})

	cases := []struct {
		name    string
		command string
		args    map[string]interface{}
	}{
		{name: "schema", command: "schema", args: map[string]interface{}{}},
		{name: "read", command: "read", args: map[string]interface{}{"reference": "people/freya", "raw": true}},
		{name: "query", command: "query", args: map[string]interface{}{"query_string": "type:person"}},
		{name: "delete-preview", command: "delete", args: map[string]interface{}{"reference": "people/freya", "dry-run": true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cliResult := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
				CommandID: tc.command,
				VaultPath: vault.Path,
				Caller:    commandexec.CallerCLI,
				Args:      tc.args,
			})

			payload, err := json.Marshal(invokeRequest{Command: tc.command, Args: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "", string(payload))
			apiResult := decodeResult(t, rec)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
			}
			assertEnvelopeParity(t, cliResult, apiResult)
		})
	}
}

func TestSchemaIncludesEnumValues(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/schema", testToken, "127.0.0.1", "", "")
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("schema: %#v", result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Types map[string]struct {
			Fields map[string]struct {
				Values []string `json:"values"`
			} `json:"fields"`
		} `json:"types"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	values := data.Types["project"].Fields["status"].Values
	if len(values) != 3 {
		t.Fatalf("status values = %#v, want [active paused done]", values)
	}
}

func TestCommandsListsInvokableSurfaceAndVersion(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/commands", testToken, "127.0.0.1", "", "")
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("commands: %#v", result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		APIVersion string `json:"api_version"`
		Commands   []struct {
			ID         string                 `json:"id"`
			ReadOnly   bool                   `json:"read_only"`
			ArgsSchema map[string]interface{} `json:"args_schema"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.APIVersion != Version {
		t.Fatalf("api_version = %q, want %q", data.APIVersion, Version)
	}

	contracts := commands.DiscoverableContracts()
	if len(data.Commands) != len(contracts) {
		t.Fatalf("listed %d commands, want %d discoverable contracts", len(data.Commands), len(contracts))
	}

	seen := map[string]bool{}
	for i, cmd := range data.Commands {
		seen[cmd.ID] = true
		if cmd.ArgsSchema == nil {
			t.Errorf("command %q missing args_schema", cmd.ID)
		}
		if !CommandAllowed(cmd.ID) {
			t.Errorf("commands listed non-invokable %q", cmd.ID)
		}
		if cmd.ID != contracts[i].CommandID {
			t.Errorf("commands[%d] = %q, want %q", i, cmd.ID, contracts[i].CommandID)
		}
		if cmd.ReadOnly != contracts[i].ReadOnly {
			t.Errorf("command %q read_only = %v, want %v", cmd.ID, cmd.ReadOnly, contracts[i].ReadOnly)
		}
	}
	for _, id := range []string{"query", "objects", "set", "new", "move", "delete", "write", "add", "edit", "reindex", "open", "init", "vault_list"} {
		if !seen[id] {
			t.Errorf("missing command %q", id)
		}
	}
	for _, id := range []string{"mcp_serve", "api", "vault", "vault_current", "vault_path"} {
		if seen[id] {
			t.Errorf("command %q should not be listed", id)
		}
	}
}

func TestSetPreviewPhaseInMeta(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		Build()
	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"set","args":{"reference":"people/freya","field":{"email":"freya@asgard"},"dry-run":true}}`)
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("set dry-run failed: %#v", result)
	}
	if result.Meta == nil || result.Meta.Mutation == nil || result.Meta.Mutation.Phase != commandexec.MutationPhasePreview {
		t.Fatalf("meta.mutation = %#v, want preview", result.Meta)
	}
}

func TestInvokeVersionDoesNotRequireVault(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{
		Invoker:    app.CommandInvoker(),
		ConfigPath: isolatedConfigPath(t),
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"version","args":{}}`)
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("version failed: %#v", result)
	}
	data := decodeDataMap(t, result)
	if data["version"] == nil || data["version"] == "" {
		t.Fatalf("version payload = %#v", data)
	}
}

func TestInvokeOpenIsAllowedWithoutSpawningEditor(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).Build()
	server := newTestServer(t, Options{
		VaultPath:  vault.Path,
		Invoker:    app.CommandInvoker(),
		ConfigPath: isolatedConfigPath(t),
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"open","args":{}}`)
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrMissingArgument {
		t.Fatalf("result = %#v, want MISSING_ARGUMENT", result)
	}
}

func TestInvokeWriteAndQuerySavedSet(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	reindexVault(t, vault.Path)
	server := newTestServer(t, Options{
		VaultPath:  vault.Path,
		Invoker:    app.CommandInvoker(),
		ConfigPath: isolatedConfigPath(t),
	})

	writeRec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"write","args":{"type":"person","title":"Thor","field":{"name":"Thor"}}}`)
	writeResult := decodeResult(t, writeRec)
	if !writeResult.OK {
		t.Fatalf("write failed: %#v", writeResult)
	}

	savedRec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"query_saved_set","args":{"name":"people","query_string":"type:person"}}`)
	savedResult := decodeResult(t, savedRec)
	if !savedResult.OK {
		t.Fatalf("query_saved_set failed: %#v", savedResult)
	}
}

func TestInvokeInitUsesArgsPathNotPinnedVault(t *testing.T) {
	t.Parallel()
	pinned := t.TempDir()
	newVault := filepath.Join(t.TempDir(), "fresh")
	payload, err := json.Marshal(invokeRequest{
		Command: "init",
		Args:    map[string]interface{}{"path": newVault},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, Options{
		VaultPath:  pinned,
		Invoker:    app.CommandInvoker(),
		ConfigPath: isolatedConfigPath(t),
	})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "", string(payload))
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("init failed: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(newVault, "schema.yaml")); err != nil {
		t.Fatalf("init did not create schema.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pinned, "schema.yaml")); !os.IsNotExist(err) {
		t.Fatalf("init mutated pinned vault: %v", err)
	}
}

func assertEnvelopeParity(t *testing.T, cliResult, apiResult commandexec.Result) {
	t.Helper()
	if cliResult.OK != apiResult.OK {
		t.Fatalf("ok cli=%v api=%v\ncli=%s\napi=%s", cliResult.OK, apiResult.OK, mustJSON(cliResult), mustJSON(apiResult))
	}
	if cliResult.Error != nil || apiResult.Error != nil {
		cliCode, apiCode := "", ""
		if cliResult.Error != nil {
			cliCode = string(cliResult.Error.Code)
		}
		if apiResult.Error != nil {
			apiCode = string(apiResult.Error.Code)
		}
		if cliCode != apiCode {
			t.Fatalf("error.code cli=%q api=%q", cliCode, apiCode)
		}
	}

	cliNorm := normalizeEnvelope(t, cliResult)
	apiNorm := normalizeEnvelope(t, apiResult)
	if !bytes.Equal(cliNorm, apiNorm) {
		t.Fatalf("envelope mismatch\ncli: %s\napi: %s", cliNorm, apiNorm)
	}
}

func normalizeEnvelope(t *testing.T, result commandexec.Result) []byte {
	t.Helper()
	if result.Meta != nil {
		metaCopy := *result.Meta
		metaCopy.QueryTimeMs = 0
		metaCopy.VaultContext = nil
		result.Meta = &metaCopy
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var generic interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
