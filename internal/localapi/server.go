// Package localapi implements Raven's loopback-only HTTP API for companion UIs.
package localapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/vaultwatch"
)

const (
	// Version is the HTTP API contract version advertised on /v1/commands,
	// /v1/health, /v1/session, /v1/revision, and in the startup envelope. It
	// matches the /v1/ URL prefix.
	Version = "1"

	defaultPort     = 7438
	maxRequestBytes = 1 << 20
	shutdownTimeout = 5 * time.Second
	bindHost        = "127.0.0.1"
)

// DefaultCORSOrigins are localhost dev ports used by SvelteKit/Vite and Tauri.
var DefaultCORSOrigins = []string{
	"http://localhost:5173",
	"http://127.0.0.1:5173",
	"http://localhost:4173",
	"http://127.0.0.1:4173",
	"http://localhost:1420",
	"http://127.0.0.1:1420",
}

// defaultLockRetryBackoff totals two seconds of wait after the first attempt.
var defaultLockRetryBackoff = []time.Duration{
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	450 * time.Millisecond,
}

// Options configures the local HTTP API server.
type Options struct {
	VaultPath    string
	ConfigPath   string
	Token        string
	Port         int
	CORSOrigins  []string
	Invoker      *commandexec.Invoker
	Sleep        func(context.Context, time.Duration) error
	RetryBackoff []time.Duration
}

// Startup is printed on listen so a launcher can parse URL and token.
type Startup struct {
	URL         string   `json:"url"`
	Token       string   `json:"token"`
	Port        int      `json:"port"`
	Bind        string   `json:"bind"`
	APIVersion  string   `json:"api_version"`
	VaultPath   string   `json:"vault_path"`
	CORSOrigins []string `json:"cors_origins"`
}

// Server is the local HTTP API.
type Server struct {
	vaultPath    string
	configPath   string
	token        string
	port         int
	corsOrigins  map[string]struct{}
	corsList     []string
	invoker      *commandexec.Invoker
	sleep        func(context.Context, time.Duration) error
	retryBackoff []time.Duration
	handler      http.Handler
}

// New constructs a server. An empty token is replaced with a generated value.
func New(opts Options) (*Server, error) {
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		generated, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("generate API token: %w", err)
		}
		token = generated
	}

	origins := opts.CORSOrigins
	if len(origins) == 0 {
		origins = append([]string{}, DefaultCORSOrigins...)
	}
	originSet := make(map[string]struct{}, len(origins))
	originList := make([]string, 0, len(origins))
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if _, ok := originSet[trimmed]; ok {
			continue
		}
		originSet[trimmed] = struct{}{}
		originList = append(originList, trimmed)
	}

	invoker := opts.Invoker
	if invoker == nil {
		invoker = app.CommandInvoker()
	}

	sleep := opts.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	backoff := opts.RetryBackoff
	if len(backoff) == 0 {
		backoff = append([]time.Duration{}, defaultLockRetryBackoff...)
	}

	s := &Server{
		vaultPath:    absoluteVaultPath(opts.VaultPath),
		configPath:   strings.TrimSpace(opts.ConfigPath),
		token:        token,
		port:         opts.Port,
		corsOrigins:  originSet,
		corsList:     originList,
		invoker:      invoker,
		sleep:        sleep,
		retryBackoff: backoff,
	}
	s.handler = s.wrap(s.mux())
	return s, nil
}

// Handler returns the HTTP handler (host check, CORS, auth, routes).
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Token returns the bearer token required by the server.
func (s *Server) Token() string {
	return s.token
}

// Listen binds 127.0.0.1 on the configured port (0 selects an ephemeral port).
func (s *Server) Listen() (net.Listener, Startup, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, s.port))
	if err != nil {
		return nil, Startup{}, err
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, Startup{}, fmt.Errorf("listen address is not TCP")
	}
	info := Startup{
		URL:         fmt.Sprintf("http://%s:%d", bindHost, addr.Port),
		Token:       s.token,
		Port:        addr.Port,
		Bind:        bindHost,
		APIVersion:  Version,
		VaultPath:   s.vaultPath,
		CORSOrigins: append([]string{}, s.corsList...),
	}
	return ln, info, nil
}

// Serve serves HTTP until ctx is cancelled or the listener closes.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if watcher, err := s.startWatcher(ctx); err != nil {
		log.Printf("raven api: vault watcher not started: %v", err)
	} else if watcher != nil {
		defer func() { _ = watcher.Close() }()
	}

	httpServer := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func (s *Server) startWatcher(ctx context.Context) (*vaultwatch.Watcher, error) {
	if strings.TrimSpace(s.vaultPath) == "" {
		return nil, nil
	}
	var exclude *ravenignore.Matcher
	if vaultCfg, err := config.LoadVaultConfig(s.vaultPath); err == nil && vaultCfg != nil {
		exclude, _ = ravenignore.NewMatcher(vaultCfg.GetExcludePatterns())
	}
	return vaultwatch.Start(ctx, vaultwatch.Options{
		VaultPath: s.vaultPath,
		Exclude:   exclude,
	})
}

func absoluteVaultPath(vaultPath string) string {
	vaultPath = strings.TrimSpace(vaultPath)
	if vaultPath == "" {
		return ""
	}
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return vaultPath
	}
	return abs
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
