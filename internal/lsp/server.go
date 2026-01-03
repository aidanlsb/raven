// Package lsp implements a Language Server Protocol server for Raven.
//
// It provides IDE features like autocomplete, go-to-definition, and diagnostics
// for Raven markdown files.
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/watcher"
)

// Server is the Raven LSP server.
type Server struct {
	// Configuration
	vaultPath string
	debug     bool

	// Raven infrastructure
	db      *index.Database
	schema  *schema.Schema
	cfg     *config.VaultConfig
	watcher *watcher.Watcher

	// Document management
	documents *DocumentManager

	// LSP communication
	input  io.Reader
	output io.Writer
	mu     sync.Mutex // Protects output writes

	// Shutdown
	shutdown bool
}

// NewServer creates a new LSP server.
func NewServer(vaultPath string, debug bool) *Server {
	return &Server{
		vaultPath: vaultPath,
		debug:     debug,
		documents: NewDocumentManager(),
		input:     os.Stdin,
		output:    os.Stdout,
	}
}

// Run starts the LSP server and processes messages until shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Initialize Raven infrastructure
	if err := s.initialize(); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	defer s.db.Close()

	s.logDebug("Raven LSP server started for vault: %s", s.vaultPath)

	// Main message loop
	for !s.shutdown {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := s.handleNextMessage(); err != nil {
				if err == io.EOF {
					return nil
				}
				s.logDebug("Error handling message: %v", err)
			}
		}
	}

	return nil
}

// initialize loads Raven infrastructure (schema, index, config).
func (s *Server) initialize() error {
	// Load schema
	sch, err := schema.Load(s.vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}
	s.schema = sch

	// Load vault config
	cfg, err := config.LoadVaultConfig(s.vaultPath)
	if err != nil {
		// Config is optional, use defaults
		cfg = &config.VaultConfig{}
	}
	s.cfg = cfg

	// Open database
	db, err := index.Open(s.vaultPath)
	if err != nil {
		return fmt.Errorf("failed to open index: %w", err)
	}
	s.db = db

	// Create watcher for reindexing (we don't start the file watcher,
	// but use it for the ReindexFile method)
	w, err := watcher.New(watcher.Config{
		VaultPath: s.vaultPath,
		Database:  s.db,
		Schema:    s.schema,
		Debug:     s.debug,
	})
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	s.watcher = w

	return nil
}

// handleNextMessage reads and processes a single LSP message.
func (s *Server) handleNextMessage() error {
	// Read Content-Length header
	var contentLength int
	for {
		var line string
		for {
			b := make([]byte, 1)
			_, err := s.input.Read(b)
			if err != nil {
				return err
			}
			if b[0] == '\n' {
				break
			}
			if b[0] != '\r' {
				line += string(b)
			}
		}

		if line == "" {
			break // Empty line separates header from content
		}

		if strings.HasPrefix(line, "Content-Length: ") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
		}
	}

	if contentLength == 0 {
		return fmt.Errorf("no Content-Length header")
	}

	// Read content
	content := make([]byte, contentLength)
	_, err := io.ReadFull(s.input, content)
	if err != nil {
		return err
	}

	// Parse JSON-RPC message
	var msg jsonRPCMessage
	if err := json.Unmarshal(content, &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	s.logDebug("Received: %s", msg.Method)

	// Dispatch to handler
	return s.dispatch(msg)
}

// dispatch routes a message to the appropriate handler.
func (s *Server) dispatch(msg jsonRPCMessage) error {
	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg)
	case "initialized":
		// Client acknowledgment, nothing to do
		return nil
	case "shutdown":
		s.shutdown = true
		return s.sendResult(msg.ID, nil)
	case "exit":
		os.Exit(0)
		return nil
	case "textDocument/didOpen":
		return s.handleDidOpen(msg)
	case "textDocument/didChange":
		return s.handleDidChange(msg)
	case "textDocument/didSave":
		return s.handleDidSave(msg)
	case "textDocument/didClose":
		return s.handleDidClose(msg)
	case "textDocument/completion":
		return s.handleCompletion(msg)
	case "textDocument/definition":
		return s.handleDefinition(msg)
	case "textDocument/hover":
		return s.handleHover(msg)
	default:
		s.logDebug("Unhandled method: %s", msg.Method)
		return nil
	}
}

// sendResult sends a successful response.
func (s *Server) sendResult(id interface{}, result interface{}) error {
	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return s.send(response)
}

// sendError sends an error response.
func (s *Server) sendError(id interface{}, code int, message string) error {
	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRPCError{
			Code:    code,
			Message: message,
		},
	}
	return s.send(response)
}

// sendNotification sends a notification (no response expected).
func (s *Server) sendNotification(method string, params interface{}) error {
	notification := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustMarshal(params),
	}
	return s.send(notification)
}

// send writes a JSON-RPC message to the output.
func (s *Server) send(msg interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(content))
	_, err = s.output.Write([]byte(header))
	if err != nil {
		return err
	}
	_, err = s.output.Write(content)
	return err
}

// logDebug logs a debug message to stderr if debug mode is enabled.
func (s *Server) logDebug(format string, args ...interface{}) {
	if s.debug {
		fmt.Fprintf(os.Stderr, "[raven-lsp] "+format+"\n", args...)
	}
}

// Helper functions

func (s *Server) uriToPath(uri string) string {
	// Convert file:// URI to filesystem path
	path := strings.TrimPrefix(uri, "file://")
	return path
}

func (s *Server) pathToURI(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.vaultPath, path)
	}
	return "file://" + path
}

func (s *Server) getResolver() *resolver.Resolver {
	objectIDs, _ := s.db.AllObjectIDs()
	return resolver.New(objectIDs)
}

func (s *Server) parseDocument(uri string, content string) (*parser.ParsedDocument, error) {
	path := s.uriToPath(uri)
	return parser.ParseDocument(content, path, s.vaultPath)
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// JSON-RPC types

type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
