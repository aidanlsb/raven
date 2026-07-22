package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aidanlsb/raven/internal/versioninfo"
)

const defaultDiagnosticsDebounce = 200 * time.Millisecond

// Options configures an LSP server.
type Options struct {
	// ExplicitVaultPath is a vault path pinned via CLI flags. Takes priority
	// over workspace-root detection.
	ExplicitVaultPath string

	// FallbackVaultPath is the active/default vault from Raven config, used
	// when neither flags nor the workspace root identify a vault.
	FallbackVaultPath string

	// In/Out are the transport streams (default: stdin/stdout).
	In  io.Reader
	Out io.Writer

	// DiagnosticsDebounce delays diagnostics after didChange. Zero means the
	// default; negative disables debouncing (used in tests).
	DiagnosticsDebounce time.Duration
}

// document is an open editor buffer.
type document struct {
	uri     string
	content string
	version int
}

// Server is a Raven LSP server.
type Server struct {
	opts Options

	in      *bufio.Reader
	out     io.Writer
	writeMu sync.Mutex

	// mu guards all state below. Request dispatch is sequential, but
	// debounced diagnostics run on timer goroutines.
	mu          sync.Mutex
	ws          *workspace
	docs        map[string]*document
	encoding    string
	initialized bool
	diagTimers  map[string]*time.Timer
}

// NewServer creates an LSP server.
func NewServer(opts Options) *Server {
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.DiagnosticsDebounce == 0 {
		opts.DiagnosticsDebounce = defaultDiagnosticsDebounce
	}

	return &Server{
		opts:       opts,
		in:         bufio.NewReader(opts.In),
		out:        opts.Out,
		encoding:   encodingUTF16,
		docs:       make(map[string]*document),
		diagTimers: make(map[string]*time.Timer),
	}
}

// Run processes messages until the client sends exit or the stream closes.
func (s *Server) Run() error {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, timer := range s.diagTimers {
			timer.Stop()
		}
		s.ws.close()
		s.ws = nil
	}()

	for {
		body, err := readMessage(s.in)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("failed to read message: %w", err)
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			s.reply(nil, nil, &ResponseError{Code: codeParseError, Message: "invalid JSON"})
			continue
		}

		if req.Method == "exit" {
			return nil
		}

		if req.ID == nil {
			s.handleNotification(&req)
			continue
		}

		result, respErr := s.handleRequest(&req)
		s.reply(req.ID, result, respErr)
	}
}

func (s *Server) reply(id *json.RawMessage, result interface{}, respErr *ResponseError) {
	resp := Response{JSONRPC: "2.0", ID: id, Result: result, Error: respErr}
	s.send(resp)
}

func (s *Server) notify(method string, params interface{}) {
	s.send(Notification{JSONRPC: "2.0", Method: method, Params: params})
}

func (s *Server) send(message interface{}) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := writeMessage(s.out, message); err != nil {
		fmt.Fprintf(os.Stderr, "rvn lsp: failed to write message: %v\n", err)
	}
}

func (s *Server) handleRequest(req *Request) (interface{}, *ResponseError) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.Params)
	case "shutdown":
		return nil, nil
	}

	s.mu.Lock()
	ready := s.initialized && s.ws != nil
	s.mu.Unlock()
	if !ready {
		return nil, &ResponseError{Code: codeServerNotInitialized, Message: "server not initialized"}
	}

	switch req.Method {
	case "textDocument/definition":
		return s.handleDefinition(req.Params)
	case "textDocument/references":
		return s.handleReferences(req.Params)
	case "textDocument/hover":
		return s.handleHover(req.Params)
	case "textDocument/completion":
		return s.handleCompletion(req.Params)
	case "textDocument/codeAction":
		return s.handleCodeAction(req.Params)
	default:
		return nil, &ResponseError{Code: codeMethodNotFound, Message: fmt.Sprintf("method not supported: %s", req.Method)}
	}
}

func (s *Server) handleNotification(req *Request) {
	switch req.Method {
	case "initialized":
		// No-op.
	case "textDocument/didOpen":
		s.handleDidOpen(req.Params)
	case "textDocument/didChange":
		s.handleDidChange(req.Params)
	case "textDocument/didSave":
		s.handleDidSave(req.Params)
	case "textDocument/didClose":
		s.handleDidClose(req.Params)
	default:
		// Ignore unknown notifications ($/cancelRequest, $/setTrace, ...).
	}
}

func (s *Server) handleInitialize(raw json.RawMessage) (interface{}, *ResponseError) {
	var params InitializeParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &ResponseError{Code: codeInvalidParams, Message: fmt.Sprintf("invalid initialize params: %v", err)}
		}
	}

	encoding := encodingUTF16
	if params.Capabilities.General != nil {
		for _, e := range params.Capabilities.General.PositionEncodings {
			if e == encodingUTF8 {
				encoding = encodingUTF8
				break
			}
		}
	}

	vaultPath, err := s.resolveVaultPath(&params)
	if err != nil {
		return nil, &ResponseError{Code: codeInternalError, Message: err.Error()}
	}

	ws, err := openWorkspace(vaultPath)
	if err != nil {
		return nil, &ResponseError{Code: codeInternalError, Message: err.Error()}
	}

	s.mu.Lock()
	s.ws = ws
	s.encoding = encoding
	s.initialized = true
	s.mu.Unlock()

	return InitializeResult{
		Capabilities: ServerCapabilities{
			PositionEncoding: encoding,
			TextDocumentSync: TextDocumentSyncOptions{
				OpenClose: true,
				Change:    syncKindFull,
				Save:      SaveOptions{IncludeText: true},
			},
			CompletionProvider: &CompletionOptions{TriggerCharacters: []string{"[", "@"}},
			DefinitionProvider: true,
			ReferencesProvider: true,
			HoverProvider:      true,
			CodeActionProvider: &CodeActionOptions{CodeActionKinds: []string{codeActionKindQuickFix}},
		},
		ServerInfo: ServerInfo{Name: "raven", Version: versioninfo.Current().Version},
	}, nil
}

// resolveVaultPath picks the vault for this session:
// explicit CLI flags > workspace root that looks like a vault > active/default vault.
func (s *Server) resolveVaultPath(params *InitializeParams) (string, error) {
	if p := strings.TrimSpace(s.opts.ExplicitVaultPath); p != "" {
		return p, nil
	}

	for _, root := range workspaceRoots(params) {
		if isVaultDir(root) {
			return root, nil
		}
	}

	if p := strings.TrimSpace(s.opts.FallbackVaultPath); p != "" {
		return p, nil
	}

	return "", fmt.Errorf("no Raven vault found: workspace root is not a vault and no vault is configured (use --vault-path, --vault, or 'rvn vault use')")
}

func workspaceRoots(params *InitializeParams) []string {
	var roots []string
	for _, folder := range params.WorkspaceFolders {
		if p := uriToPath(folder.URI); p != "" {
			roots = append(roots, p)
		}
	}
	if params.RootURI != nil {
		if p := uriToPath(*params.RootURI); p != "" {
			roots = append(roots, p)
		}
	}
	if params.RootPath != nil && *params.RootPath != "" {
		roots = append(roots, *params.RootPath)
	}
	return roots
}

func (s *Server) handleDidOpen(raw json.RawMessage) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return
	}

	s.mu.Lock()
	s.docs[params.TextDocument.URI] = &document{
		uri:     params.TextDocument.URI,
		content: params.TextDocument.Text,
		version: params.TextDocument.Version,
	}
	s.mu.Unlock()

	s.publishDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidChange(raw json.RawMessage) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return
	}
	if len(params.ContentChanges) == 0 {
		return
	}
	// Full sync: the last change event carries the complete document.
	content := params.ContentChanges[len(params.ContentChanges)-1].Text

	s.mu.Lock()
	doc, ok := s.docs[params.TextDocument.URI]
	if !ok {
		doc = &document{uri: params.TextDocument.URI}
		s.docs[params.TextDocument.URI] = doc
	}
	doc.content = content
	doc.version = params.TextDocument.Version
	s.mu.Unlock()

	s.scheduleDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidSave(raw json.RawMessage) {
	var params DidSaveTextDocumentParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return
	}

	s.mu.Lock()
	if params.Text != nil {
		if doc, ok := s.docs[params.TextDocument.URI]; ok {
			doc.content = *params.Text
		}
	}
	ws := s.ws
	s.mu.Unlock()

	if ws != nil {
		s.mu.Lock()
		err := ws.refresh()
		openURIs := make([]string, 0, len(s.docs))
		for uri := range s.docs {
			openURIs = append(openURIs, uri)
		}
		s.mu.Unlock()
		if err != nil {
			// Index writes can contend with a concurrent CLI reindex; diagnostics
			// keep working against the previous caches.
			fmt.Fprintf(os.Stderr, "rvn lsp: refresh after save failed: %v\n", err)
		}

		for _, uri := range openURIs {
			s.publishDiagnostics(uri)
		}
	}
}

func (s *Server) handleDidClose(raw json.RawMessage) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return
	}

	s.mu.Lock()
	delete(s.docs, params.TextDocument.URI)
	if timer, ok := s.diagTimers[params.TextDocument.URI]; ok {
		timer.Stop()
		delete(s.diagTimers, params.TextDocument.URI)
	}
	s.mu.Unlock()

	// Clear stale diagnostics for the closed document.
	s.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

// scheduleDiagnostics debounces diagnostics for a document.
func (s *Server) scheduleDiagnostics(uri string) {
	if s.opts.DiagnosticsDebounce < 0 {
		s.publishDiagnostics(uri)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if timer, ok := s.diagTimers[uri]; ok {
		timer.Stop()
	}
	s.diagTimers[uri] = time.AfterFunc(s.opts.DiagnosticsDebounce, func() {
		s.publishDiagnostics(uri)
	})
}

// snapshot returns immutable views of the workspace caches and one open
// document. Copying the workspace keeps request handlers race-free if a
// diagnostics timer refreshes the live caches concurrently.
func (s *Server) snapshot(uri string) (*workspace, document, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return nil, document{}, false
	}
	doc, ok := s.docs[uri]
	if !ok {
		return nil, document{}, false
	}
	s.ensureWorkspaceCachesFreshLocked(s.ws)
	wsSnapshot := *s.ws
	return &wsSnapshot, *doc, true
}

// ensureWorkspaceCachesFreshLocked refreshes LSP caches after commits made by
// another index handle. The caller must hold s.mu.
func (s *Server) ensureWorkspaceCachesFreshLocked(ws *workspace) {
	if err := ws.ensureCachesFresh(); err != nil && !errors.Is(err, errIndexChanging) {
		fmt.Fprintf(os.Stderr, "rvn lsp: external index refresh failed: %v\n", err)
	}
}
