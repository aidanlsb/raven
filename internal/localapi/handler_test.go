package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
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

func TestAllowlistRejectsDelete(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"delete","args":{"reference":"people/freya"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrCommandNotAllowed {
		t.Fatalf("result = %#v, want COMMAND_NOT_ALLOWED", result)
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

func TestCommandsListsAllowlistAndVersion(t *testing.T) {
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
	seen := map[string]bool{}
	for _, cmd := range data.Commands {
		seen[cmd.ID] = true
		if cmd.ArgsSchema == nil {
			t.Errorf("command %q missing args_schema", cmd.ID)
		}
		if !CommandAllowed(cmd.ID) {
			t.Errorf("commands listed disallowed %q", cmd.ID)
		}
	}
	for _, id := range []string{"query", "set", "new", "move"} {
		if !seen[id] {
			t.Errorf("missing command %q", id)
		}
	}
	if seen["delete"] {
		t.Fatal("delete should not be listed")
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
