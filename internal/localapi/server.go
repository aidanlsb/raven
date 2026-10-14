// Package localapi implements Raven's headless local HTTP API for companion UIs.
package localapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

const (
	// DefaultPort is the documented listen port. Pass 0 to let the OS assign one.
	DefaultPort  = 7437
	maxBodyBytes = 1 << 20
)

// DefaultCORSOrigins are localhost origins used by SvelteKit/Vite and Tauri
// during companion-UI development.
func DefaultCORSOrigins() []string {
	return []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:4173",
		"http://127.0.0.1:4173",
		"http://localhost:1420",
		"http://127.0.0.1:1420",
		"tauri://localhost",
		"https://tauri.localhost",
	}
}

// Executor runs a canonical command. *commandexec.Invoker implements this.
type Executor interface {
	Execute(context.Context, commandexec.Request) commandexec.Result
}

// Options configures the local HTTP API process.
type Options struct {
	VaultPath      string
	VaultName      string
	ConfigPath     string
	StatePath      string
	ExecutablePath string
	Token          string
	CORSOrigins    []string
	Invoker        Executor
	IndexLockRetry time.Duration
	IndexLockDelay time.Duration
}

// Info is printed on startup so a launcher can parse the listen URL and token.
type Info struct {
	URL        string `json:"url"`
	Token      string `json:"token"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	APIVersion string `json:"api_version"`
}

// Server is the loopback HTTP API.
type Server struct {
	opts       Options
	token      string
	cors       map[string]struct{}
	invoker    Executor
	httpServer *http.Server
}

type invokeRequest struct {
	Command string                 `json:"command"`
	Args    map[string]interface{} `json:"args"`
}

// GenerateToken returns a 256-bit hex bearer token.
func GenerateToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// New constructs a local API server. Token is generated when Options.Token is empty.
func New(opts Options) (*Server, error) {
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		generated, err := GenerateToken()
		if err != nil {
			return nil, fmt.Errorf("generate API token: %w", err)
		}
		token = generated
	}

	origins := opts.CORSOrigins
	if len(origins) == 0 {
		origins = DefaultCORSOrigins()
	}
	cors := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		cors[origin] = struct{}{}
	}

	invoker := opts.Invoker
	if invoker == nil {
		invoker = app.CommandInvoker()
	}

	return &Server{
		opts:    opts,
		token:   token,
		cors:    cors,
		invoker: invoker,
	}, nil
}

// Token returns the bearer token required by this process.
func (s *Server) Token() string {
	return s.token
}

// Handler returns the HTTP handler, including host, CORS, and auth checks.
func (s *Server) Handler() http.Handler {
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackHost(r.Host) {
		s.writeResult(w, r, http.StatusForbidden, commandexec.Failure(
			codes.ErrForbidden,
			"host header is not loopback",
			map[string]interface{}{"host": r.Host},
			"Call http://127.0.0.1 directly; non-loopback Host headers are rejected to prevent DNS rebinding",
		))
		return
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		if _, ok := s.cors[origin]; !ok {
			s.writeResult(w, r, http.StatusForbidden, commandexec.Failure(
				codes.ErrForbidden,
				"origin is not allowed",
				map[string]interface{}{"origin": origin},
				"Pass --cors-origin or RAVEN_API_CORS_ORIGINS for the companion UI origin",
			))
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "600")
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !s.authorized(r) {
		s.writeResult(w, r, http.StatusUnauthorized, commandexec.Failure(
			codes.ErrUnauthorized,
			"missing or invalid bearer token",
			nil,
			"Send Authorization: Bearer <token> using the token printed at API startup",
		))
		return
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/invoke":
		s.handleInvoke(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/schema":
		s.handleSchema(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/commands":
		s.handleCommands(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
		s.handleHealth(w, r)
	default:
		s.writeResult(w, r, http.StatusNotFound, commandexec.Failure(
			codes.ErrNotFound,
			"unknown API path",
			map[string]interface{}{"path": r.URL.Path, "method": r.Method},
			"Use POST /v1/invoke, GET /v1/schema, GET /v1/commands, or GET /v1/health",
		))
	}
}

// Bind listens on 127.0.0.1 only and returns the bound address. Port 0 lets
// the OS assign a free port. The caller should print Info, then Serve.
func (s *Server) Bind(port int) (net.Listener, Info, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, Info{}, err
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, Info{}, fmt.Errorf("unexpected listen address %T", listener.Addr())
	}

	info := Info{
		URL:        fmt.Sprintf("http://127.0.0.1:%d", addr.Port),
		Token:      s.token,
		Host:       "127.0.0.1",
		Port:       addr.Port,
		APIVersion: APIVersion,
	}
	return listener, info, nil
}

// Serve serves the API on listener until ctx is cancelled or the server fails.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.httpServer = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		serveErr := s.httpServer.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		return <-errCh
	case serveErr := <-errCh:
		return serveErr
	}
}

func (s *Server) authorized(r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if got == "" || s.token == "" {
		return false
	}
	if len(got) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeResult(w, r, http.StatusOK, commandexec.Success(map[string]interface{}{
		"status":      "ok",
		"api_version": APIVersion,
	}, nil))
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	s.writeResult(w, r, http.StatusOK, commandexec.Success(buildCommandCatalog(), nil))
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	result := s.execute(r.Context(), commandexec.Request{
		CommandID: "schema",
		Args:      map[string]interface{}{},
	})
	s.writeResult(w, r, http.StatusOK, result)
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		s.writeResult(w, r, http.StatusBadRequest, commandexec.Failure(
			codes.ErrInvalidJSON,
			"failed to read request body",
			nil,
			"Retry with a valid JSON body",
		))
		return
	}
	if len(body) > maxBodyBytes {
		s.writeResult(w, r, http.StatusBadRequest, commandexec.Failure(
			codes.ErrInvalidJSON,
			"request body is too large",
			nil,
			"Send a smaller invoke payload",
		))
		return
	}

	var req invokeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeResult(w, r, http.StatusBadRequest, commandexec.Failure(
			codes.ErrInvalidJSON,
			"request body is not valid JSON",
			nil,
			`Send {"command":"<id>","args":{...}}`,
		))
		return
	}

	commandRef := strings.TrimSpace(req.Command)
	if commandRef == "" {
		s.writeResult(w, r, http.StatusBadRequest, commandexec.Failure(
			codes.ErrMissingArgument,
			"command is required",
			nil,
			`Send {"command":"<id>","args":{...}}`,
		))
		return
	}

	commandID, ok := commands.ResolveCommandID(commandRef)
	if !ok {
		s.writeResult(w, r, http.StatusOK, commandexec.Failure(
			codes.ErrCommandNotFound,
			"unknown command: "+commandRef,
			map[string]interface{}{"command": commandRef},
			"GET /v1/commands for the allowlisted command catalog",
		))
		return
	}

	if denied := s.denyIfNotAllowed(commandID, req.Args); denied != nil {
		s.writeResult(w, r, http.StatusOK, *denied)
		return
	}

	result := s.execute(r.Context(), commandexec.Request{
		CommandID: commandID,
		Args:      req.Args,
	})
	s.writeResult(w, r, http.StatusOK, result)
}

func (s *Server) denyIfNotAllowed(commandID string, args map[string]interface{}) *commandexec.Result {
	if !CommandAllowed(commandID) {
		result := commandDenied(commandID, "command is not available on the local API")
		return &result
	}
	if commandID != "query" {
		return nil
	}
	applyCommand, hasApply := queryApplyCommand(args)
	if !hasApply {
		return nil
	}
	if CommandAllowed(applyCommand) {
		return nil
	}
	result := commandDenied(applyCommand, "query apply is limited to allowlisted write commands")
	return &result
}

func commandDenied(commandID, message string) commandexec.Result {
	return commandexec.Failure(
		codes.ErrAPICommandDenied,
		message,
		map[string]interface{}{"command": commandID},
		"GET /v1/commands for the allowlisted command catalog",
	)
}

func (s *Server) execute(ctx context.Context, req commandexec.Request) commandexec.Result {
	req.VaultPath = s.opts.VaultPath
	req.ConfigPath = s.opts.ConfigPath
	req.StatePath = s.opts.StatePath
	req.ExecutablePath = s.opts.ExecutablePath
	req.Caller = commandexec.CallerAPI
	if req.Args == nil {
		req.Args = map[string]interface{}{}
	}

	result := executeWithIndexLockRetry(ctx, s.opts.IndexLockRetry, s.opts.IndexLockDelay, func() commandexec.Result {
		return s.invoker.Execute(ctx, req)
	})

	if strings.TrimSpace(s.opts.VaultPath) != "" {
		if result.Meta == nil {
			result.Meta = &commandexec.Meta{}
		}
		result.Meta.VaultContext = &commandexec.VaultContext{
			Name:   s.opts.VaultName,
			Path:   s.opts.VaultPath,
			Source: "pinned",
		}
	}
	return result
}

func (s *Server) writeResult(w http.ResponseWriter, _ *http.Request, status int, result commandexec.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "localapi: failed to encode response envelope: %v\n", err)
	}
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	switch strings.ToLower(hostname) {
	case "localhost", "127.0.0.1", "::1", "0:0:0:0:0:0:0:1":
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}
