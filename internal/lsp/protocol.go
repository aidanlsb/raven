// Package lsp implements a Language Server Protocol server for Raven vaults.
//
// The server speaks LSP 3.17 over stdio using Content-Length framed JSON-RPC 2.0
// messages. Only the protocol subset needed by the implemented capabilities is
// defined here (mirroring how internal/mcp hand-rolls its protocol types).
package lsp

import "encoding/json"

// Request is an incoming JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// Response is an outgoing JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *ResponseError   `json:"error,omitempty"`
}

// Notification is an outgoing JSON-RPC 2.0 notification.
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// ResponseError is a JSON-RPC 2.0 error object.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes used by this server.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603

	// LSP-specific error codes.
	codeServerNotInitialized = -32002
)

// Position is a zero-based line/character position.
// Character offsets are counted in the negotiated position encoding.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a start/end position pair.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a document URI plus a range.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier identifies a document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem is the full document sent on didOpen.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams is shared by definition/hover/completion requests.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// InitializeParams is the subset of initialize params the server reads.
type InitializeParams struct {
	RootURI          *string            `json:"rootUri,omitempty"`
	RootPath         *string            `json:"rootPath,omitempty"`
	WorkspaceFolders []WorkspaceFolder  `json:"workspaceFolders,omitempty"`
	Capabilities     ClientCapabilities `json:"capabilities"`
}

// WorkspaceFolder is a workspace root advertised by the client.
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// ClientCapabilities is the subset of client capabilities the server reads.
type ClientCapabilities struct {
	General *GeneralClientCapabilities `json:"general,omitempty"`
}

// GeneralClientCapabilities carries position encoding negotiation.
type GeneralClientCapabilities struct {
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

// InitializeResult is the server's initialize response.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
}

// ServerInfo identifies the server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities advertises the implemented capabilities.
type ServerCapabilities struct {
	PositionEncoding   string                  `json:"positionEncoding,omitempty"`
	TextDocumentSync   TextDocumentSyncOptions `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions      `json:"completionProvider,omitempty"`
	DefinitionProvider bool                    `json:"definitionProvider,omitempty"`
	ReferencesProvider bool                    `json:"referencesProvider,omitempty"`
	HoverProvider      bool                    `json:"hoverProvider,omitempty"`
}

// Text document sync kinds.
const (
	syncKindFull = 1
)

// TextDocumentSyncOptions configures document synchronization.
type TextDocumentSyncOptions struct {
	OpenClose bool        `json:"openClose"`
	Change    int         `json:"change"`
	Save      SaveOptions `json:"save"`
}

// SaveOptions configures didSave behavior.
type SaveOptions struct {
	IncludeText bool `json:"includeText"`
}

// CompletionOptions configures the completion provider.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// DidOpenTextDocumentParams is the didOpen notification payload.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams is the didChange notification payload.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// VersionedTextDocumentIdentifier is a URI plus version.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentContentChangeEvent carries new document content.
// With full sync, Range is nil and Text is the complete document.
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// DidSaveTextDocumentParams is the didSave notification payload.
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// DidCloseTextDocumentParams is the didClose notification payload.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// PublishDiagnosticsParams is the publishDiagnostics notification payload.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnostic severities.
const (
	severityError   = 1
	severityWarning = 2
)

// Diagnostic is a single reported issue.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// ReferenceParams is the textDocument/references request payload.
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// ReferenceContext carries the includeDeclaration flag.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// Hover is the textDocument/hover response payload.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent is markdown or plaintext content.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// CompletionList is the textDocument/completion response payload.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Completion item kinds used by this server.
const (
	completionKindField     = 5
	completionKindKeyword   = 14
	completionKindFile      = 17
	completionKindReference = 18
)

// CompletionItem is a single completion suggestion.
type CompletionItem struct {
	Label      string    `json:"label"`
	Kind       int       `json:"kind,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	FilterText string    `json:"filterText,omitempty"`
	SortText   string    `json:"sortText,omitempty"`
	TextEdit   *TextEdit `json:"textEdit,omitempty"`
}

// TextEdit replaces a range with new text.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}
