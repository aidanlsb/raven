package localapi

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func (s *Server) mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/schema", s.handleSchema)
	mux.HandleFunc("/v1/commands", s.handleCommands)
	mux.HandleFunc("/v1/invoke", s.handleInvoke)
	return mux
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Raven-API-Version", Version)

		if !loopbackHost(r.Host) {
			writeResult(w, http.StatusForbidden, commandexec.Failure(
				codes.ErrInvalidHost,
				"host header is not a loopback address",
				map[string]interface{}{"host": r.Host},
				"Call 127.0.0.1 or localhost; remote Host headers are rejected to prevent DNS rebinding",
			))
			return
		}

		if s.handleCORS(w, r) {
			return
		}

		if r.Method != http.MethodOptions && !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeResult(w, http.StatusUnauthorized, commandexec.Failure(
				codes.ErrUnauthorized,
				"missing or invalid bearer token",
				nil,
				"Send Authorization: Bearer <token> using the token printed at API startup",
			))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		if _, ok := s.corsOrigins[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
	}
	if r.Method != http.MethodOptions {
		return false
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

func (s *Server) authorized(r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if got == "" || len(got) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func loopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func writeResult(w http.ResponseWriter, status int, result commandexec.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	if err := enc.Encode(result); err != nil {
		return
	}
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method+", OPTIONS")
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	return false
}
