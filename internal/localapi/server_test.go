package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

type stubInvoker struct {
	fn func(context.Context, commandexec.Request) commandexec.Result
}

func (s stubInvoker) Execute(ctx context.Context, req commandexec.Request) commandexec.Result {
	return s.fn(ctx, req)
}

func testServer(t *testing.T, opts Options) *Server {
	t.Helper()
	if opts.Token == "" {
		opts.Token = "test-token"
	}
	if opts.VaultPath == "" {
		opts.VaultPath = t.TempDir()
	}
	server, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return server
}

func apiRequest(method, path, token string, body []byte) *http.Request {
	url := "http://127.0.0.1" + path
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func doRequest(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder) commandexec.Result {
	t.Helper()
	var result commandexec.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\nbody=%s", err, rec.Body.String())
	}
	return result
}

func TestHealthRequiresAuth(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{})

	rec := doRequest(server, apiRequest(http.MethodGet, "/v1/health", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrUnauthorized {
		t.Fatalf("result=%#v, want UNAUTHORIZED", result)
	}

	rec = doRequest(server, apiRequest(http.MethodGet, "/v1/health", "wrong", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d, want 401", rec.Code)
	}

	rec = doRequest(server, apiRequest(http.MethodGet, "/v1/health", "test-token", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("authed status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	result = decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("health failed: %#v", result)
	}
}

func TestRejectsNonLoopbackHost(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{})
	req := apiRequest(http.MethodGet, "/v1/health", "test-token", nil)
	req.Host = "evil.example"
	rec := doRequest(server, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.Error == nil || result.Error.Code != codes.ErrForbidden {
		t.Fatalf("result=%#v, want FORBIDDEN", result)
	}
}

func TestCORSPreflightAndDeniedOrigin(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{CORSOrigins: []string{"http://localhost:5173"}})

	preflight := apiRequest(http.MethodOptions, "/v1/health", "", nil)
	preflight.Header.Set("Origin", "http://localhost:5173")
	rec := doRequest(server, preflight)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Allow-Origin=%q", got)
	}

	denied := apiRequest(http.MethodGet, "/v1/health", "test-token", nil)
	denied.Header.Set("Origin", "https://evil.example")
	rec = doRequest(server, denied)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied origin status=%d, want 403", rec.Code)
	}
}

func TestAllowlistDeniesUnlistedCommands(t *testing.T) {
	t.Parallel()
	called := false
	server := testServer(t, Options{
		Invoker: stubInvoker{fn: func(context.Context, commandexec.Request) commandexec.Result {
			called = true
			return commandexec.Success(map[string]interface{}{"ran": true}, nil)
		}},
	})

	body, _ := json.Marshal(invokeRequest{Command: "delete", Args: map[string]interface{}{"reference": "people/freya"}})
	rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrAPICommandDenied {
		t.Fatalf("result=%#v, want API_COMMAND_DENIED", result)
	}
	if called {
		t.Fatal("invoker should not run for denied commands")
	}
}

func TestQueryApplyDenyUnlistedNestedCommand(t *testing.T) {
	t.Parallel()
	called := false
	server := testServer(t, Options{
		Invoker: stubInvoker{fn: func(context.Context, commandexec.Request) commandexec.Result {
			called = true
			return commandexec.Success(nil, nil)
		}},
	})

	body, _ := json.Marshal(invokeRequest{
		Command: "query",
		Args: map[string]interface{}{
			"query_string": "type:person",
			"apply":        []interface{}{"delete"},
		},
	})
	rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", body))
	result := decodeResult(t, rec)
	if result.Error == nil || result.Error.Code != codes.ErrAPICommandDenied {
		t.Fatalf("result=%#v, want API_COMMAND_DENIED", result)
	}
	if called {
		t.Fatal("invoker should not run for denied query apply")
	}
}

func TestIndexLockRetryThenSuccess(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := testServer(t, Options{
		IndexLockRetry: 200 * time.Millisecond,
		IndexLockDelay: 5 * time.Millisecond,
		Invoker: stubInvoker{fn: func(context.Context, commandexec.Request) commandexec.Result {
			n := calls.Add(1)
			if n == 1 {
				return commandexec.Failure(codes.ErrDatabase, "failed to open database: index is locked for rebuild", nil, "")
			}
			return commandexec.Success(map[string]interface{}{"attempt": n}, nil)
		}},
	})

	body, _ := json.Marshal(invokeRequest{Command: "vault_stats", Args: map[string]interface{}{}})
	rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", body))
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("expected success after retry, got %#v", result)
	}
	if calls.Load() < 2 {
		t.Fatalf("calls=%d, want at least 2", calls.Load())
	}
}

func TestIndexLockRetryExhausted(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{
		IndexLockRetry: 20 * time.Millisecond,
		IndexLockDelay: 5 * time.Millisecond,
		Invoker: stubInvoker{fn: func(context.Context, commandexec.Request) commandexec.Result {
			return commandexec.Failure(codes.ErrDatabase, "failed to open database: index is locked for rebuild", nil, "")
		}},
	})

	body, _ := json.Marshal(invokeRequest{Command: "vault_stats", Args: map[string]interface{}{}})
	rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", body))
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrDatabase {
		t.Fatalf("result=%#v, want DATABASE_ERROR", result)
	}
}

func TestCommandsCatalogIsAllowlistedAndVersioned(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{})
	rec := doRequest(server, apiRequest(http.MethodGet, "/v1/commands", "test-token", nil))
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("commands failed: %#v", result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var catalog commandCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if catalog.APIVersion != APIVersion {
		t.Fatalf("api_version=%q, want %q", catalog.APIVersion, APIVersion)
	}
	if len(catalog.Commands) == 0 {
		t.Fatal("expected allowlisted commands")
	}
	seen := map[string]bool{}
	for _, cmd := range catalog.Commands {
		if !CommandAllowed(cmd.ID) {
			t.Fatalf("catalog includes denied command %q", cmd.ID)
		}
		if cmd.ArgsSchema == nil {
			t.Fatalf("command %q missing args_schema", cmd.ID)
		}
		seen[cmd.ID] = true
	}
	if seen["delete"] {
		t.Fatal("delete must not appear in the API catalog")
	}
	for _, need := range []string{"query", "read", "set", "new", "move"} {
		if !seen[need] {
			t.Fatalf("catalog missing %q", need)
		}
	}
}

func TestEnvelopeParityWithCLI(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\nQueen of Asgard\n").
		Build()

	server := testServer(t, Options{
		VaultPath: v.Path,
		Invoker:   app.CommandInvoker(),
	})

	cases := []struct {
		command string
		args    map[string]interface{}
	}{
		{command: "schema", args: map[string]interface{}{}},
		{command: "read", args: map[string]interface{}{"reference": "people/freya", "raw": true}},
		{command: "query", args: map[string]interface{}{"query_string": "type:person"}},
	}

	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			cliResult := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
				CommandID: tc.command,
				VaultPath: v.Path,
				Caller:    commandexec.CallerCLI,
				Args:      cloneArgs(tc.args),
			})

			body, _ := json.Marshal(invokeRequest{Command: tc.command, Args: cloneArgs(tc.args)})
			rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", body))
			apiResult := decodeResult(t, rec)
			assertEnvelopeParity(t, cliResult, apiResult)
		})
	}

	schemaRec := doRequest(server, apiRequest(http.MethodGet, "/v1/schema", "test-token", nil))
	schemaResult := decodeResult(t, schemaRec)
	if !schemaResult.OK {
		t.Fatalf("schema endpoint failed: %#v", schemaResult)
	}
	data, _ := schemaResult.Data.(map[string]interface{})
	types, _ := data["types"].(map[string]interface{})
	project, _ := types["project"].(map[string]interface{})
	fields, _ := project["fields"].(map[string]interface{})
	status, _ := fields["status"].(map[string]interface{})
	values, _ := status["values"].([]interface{})
	if len(values) != 3 {
		t.Fatalf("expected enum values on project.status, got %#v", status)
	}
}

func TestAllowedCommandIDsExistInRegistry(t *testing.T) {
	t.Parallel()
	for commandID := range AllowedCommandIDs {
		if _, ok := commands.EffectiveMeta(commandID); !ok {
			t.Fatalf("allowlisted command %q is missing from the registry", commandID)
		}
	}
}

func TestBindLoopbackOnly(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{})
	listener, info, err := server.Bind(0)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	defer listener.Close()
	if info.Host != "127.0.0.1" {
		t.Fatalf("host=%q", info.Host)
	}
	if !strings.HasPrefix(info.URL, "http://127.0.0.1:") {
		t.Fatalf("url=%q", info.URL)
	}
	if info.Port == 0 {
		t.Fatal("expected assigned port")
	}
}

func cloneArgs(args map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}

func assertEnvelopeParity(t *testing.T, cli, api commandexec.Result) {
	t.Helper()
	if cli.OK != api.OK {
		t.Fatalf("ok cli=%v api=%v\ncli=%s\napi=%s", cli.OK, api.OK, mustJSON(cli), mustJSON(api))
	}
	if (cli.Error == nil) != (api.Error == nil) {
		t.Fatalf("error presence mismatch\ncli=%s\napi=%s", mustJSON(cli), mustJSON(api))
	}
	if cli.Error != nil {
		if cli.Error.Code != api.Error.Code {
			t.Fatalf("error.code cli=%s api=%s", cli.Error.Code, api.Error.Code)
		}
	}
	if canonicalJSON(cli.Data) != canonicalJSON(api.Data) {
		t.Fatalf("data mismatch\ncli=%s\napi=%s", canonicalJSON(cli.Data), canonicalJSON(api.Data))
	}
	if mutationPhase(cli) != mutationPhase(api) {
		t.Fatalf("mutation phase cli=%q api=%q", mutationPhase(cli), mutationPhase(api))
	}
}

func mutationPhase(result commandexec.Result) string {
	if result.Meta == nil || result.Meta.Mutation == nil {
		return ""
	}
	return string(result.Meta.Mutation.Phase)
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func canonicalJSON(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var generic interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return string(raw)
	}
	out, err := json.Marshal(generic)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func TestInvalidInvokeJSON(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{})
	rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", []byte("{not-json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.Error == nil || result.Error.Code != codes.ErrInvalidJSON {
		t.Fatalf("result=%#v, want INVALID_JSON", result)
	}
}

func TestUnknownPath(t *testing.T) {
	t.Parallel()
	server := testServer(t, Options{})
	rec := doRequest(server, apiRequest(http.MethodGet, "/v1/nope", "test-token", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestWriteMutationPhasePreserved(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	server := testServer(t, Options{
		VaultPath: v.Path,
		Invoker:   app.CommandInvoker(),
	})

	body, _ := json.Marshal(invokeRequest{
		Command: "new",
		Args: map[string]interface{}{
			"type":  "person",
			"title": "Thor",
		},
	})
	rec := doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", body))
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("new failed: %#v", result)
	}
	if mutationPhase(result) != string(commandexec.MutationPhaseApplied) {
		t.Fatalf("phase=%q, want applied", mutationPhase(result))
	}

	previewBody, _ := json.Marshal(invokeRequest{
		Command: "set",
		Args: map[string]interface{}{
			"reference": "people/thor",
			"field":     map[string]interface{}{"email": "thor@asgard.realm"},
			"dry-run":   true,
		},
	})
	rec = doRequest(server, apiRequest(http.MethodPost, "/v1/invoke", "test-token", previewBody))
	result = decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("set dry-run failed: %#v", result)
	}
	if mutationPhase(result) != string(commandexec.MutationPhasePreview) {
		t.Fatalf("set dry-run phase=%q, want preview; body=%s", mutationPhase(result), rec.Body.String())
	}
}
