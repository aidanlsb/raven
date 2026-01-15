package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/parser"
)

// LSP Protocol Types
// These are simplified versions - a full implementation would use go.lsp.dev/protocol

type InitializeParams struct {
	RootURI string `json:"rootUri"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

type ServerCapabilities struct {
	TextDocumentSync   int                `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions `json:"completionProvider,omitempty"`
	DefinitionProvider bool               `json:"definitionProvider"`
	HoverProvider      bool               `json:"hoverProvider"`
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"` // Full content (we use full sync)
}

type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CompletionParams struct {
	TextDocumentPositionParams
	Context *CompletionContext `json:"context,omitempty"`
}

type CompletionContext struct {
	TriggerKind      int    `json:"triggerKind"`
	TriggerCharacter string `json:"triggerCharacter,omitempty"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Completion item kinds
const (
	CompletionKindText      = 1
	CompletionKindMethod    = 2
	CompletionKindFunction  = 3
	CompletionKindField     = 4
	CompletionKindVariable  = 6
	CompletionKindClass     = 7
	CompletionKindInterface = 8
	CompletionKindModule    = 9
	CompletionKindProperty  = 10
	CompletionKindFile      = 17
	CompletionKindReference = 18
	CompletionKindFolder    = 19
	CompletionKindEvent     = 23
	CompletionKindKeyword   = 14
)

// Diagnostic severities
const (
	DiagnosticSeverityError       = 1
	DiagnosticSeverityWarning     = 2
	DiagnosticSeverityInformation = 3
	DiagnosticSeverityHint        = 4
)

// Handler implementations

func (s *Server) handleInitialize(msg jsonRPCMessage) error {
	var params InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, -32602, "Invalid params")
	}

	// If rootUri is provided and we don't have a vault path, use it
	if s.vaultPath == "" && params.RootURI != "" {
		s.vaultPath = s.uriToPath(params.RootURI)
		if err := s.initialize(); err != nil {
			s.logDebug("Failed to initialize with rootUri: %v", err)
		}
	}

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: 1, // Full sync
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{"[", "@"},
			},
			DefinitionProvider: true,
			HoverProvider:      true,
		},
	}

	return s.sendResult(msg.ID, result)
}

func (s *Server) handleDidOpen(msg jsonRPCMessage) error {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}

	s.documents.Open(
		params.TextDocument.URI,
		params.TextDocument.Text,
		params.TextDocument.Version,
	)

	s.logDebug("Opened: %s", params.TextDocument.URI)

	// Publish initial diagnostics
	s.publishDiagnostics(params.TextDocument.URI)

	return nil
}

func (s *Server) handleDidChange(msg jsonRPCMessage) error {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}

	// We use full sync, so take the last content change
	if len(params.ContentChanges) > 0 {
		content := params.ContentChanges[len(params.ContentChanges)-1].Text
		s.documents.Update(
			params.TextDocument.URI,
			content,
			params.TextDocument.Version,
		)
	}

	return nil
}

func (s *Server) handleDidSave(msg jsonRPCMessage) error {
	var params DidSaveTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}

	s.logDebug("Saved: %s", params.TextDocument.URI)

	// Reindex this file
	path := s.uriToPath(params.TextDocument.URI)
	if err := s.reindexFile(path); err != nil {
		s.logDebug("Failed to reindex: %v", err)
	}

	// Publish diagnostics
	s.publishDiagnostics(params.TextDocument.URI)

	return nil
}

func (s *Server) handleDidClose(msg jsonRPCMessage) error {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}

	s.documents.Close(params.TextDocument.URI)
	s.logDebug("Closed: %s", params.TextDocument.URI)

	return nil
}

func (s *Server) handleCompletion(msg jsonRPCMessage) error {
	var params CompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, -32602, "Invalid params")
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		return s.sendResult(msg.ID, nil)
	}

	line := getLineAt(doc.Content, params.Position.Line)
	col := params.Position.Character

	var items []CompletionItem

	// Check if we're in a reference context [[
	if isInReference(line, col) {
		items = s.completeReferences(line, col)
	} else if isInTrait(line, col) {
		items = s.completeTraits(line, col)
	}

	return s.sendResult(msg.ID, CompletionList{
		IsIncomplete: false,
		Items:        items,
	})
}

func (s *Server) handleDefinition(msg jsonRPCMessage) error {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, -32602, "Invalid params")
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		return s.sendResult(msg.ID, nil)
	}

	line := getLineAt(doc.Content, params.Position.Line)
	ref := findRefAtPosition(line, params.Position.Character)
	if ref == "" {
		return s.sendResult(msg.ID, nil)
	}

	// Resolve the reference
	resolver := s.getResolver()
	result := resolver.Resolve(ref)
	if result.TargetID == "" || result.Ambiguous {
		return s.sendResult(msg.ID, nil)
	}

	// Build target location
	targetPath := filepath.Join(s.vaultPath, result.TargetID+".md")
	location := Location{
		URI: s.pathToURI(targetPath),
		Range: Range{
			Start: Position{Line: 0, Character: 0},
			End:   Position{Line: 0, Character: 0},
		},
	}

	return s.sendResult(msg.ID, location)
}

func (s *Server) handleHover(msg jsonRPCMessage) error {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, -32602, "Invalid params")
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		return s.sendResult(msg.ID, nil)
	}

	line := getLineAt(doc.Content, params.Position.Line)
	ref := findRefAtPosition(line, params.Position.Character)
	if ref == "" {
		return s.sendResult(msg.ID, nil)
	}

	// Resolve the reference
	resolver := s.getResolver()
	result := resolver.Resolve(ref)
	if result.TargetID == "" && !result.Ambiguous {
		return s.sendResult(msg.ID, Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: fmt.Sprintf("**Not found:** `%s`", ref),
			},
		})
	}
	if result.Ambiguous {
		return s.sendResult(msg.ID, Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: fmt.Sprintf("**Ambiguous:** `%s`\n\nMatches: %v", ref, result.Matches),
			},
		})
	}

	// Read the target file to get info
	targetPath := filepath.Join(s.vaultPath, result.TargetID+".md")
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return s.sendResult(msg.ID, nil)
	}

	// Parse to get frontmatter
	parsed, err := parser.ParseDocument(string(content), targetPath, s.vaultPath)
	if err != nil || len(parsed.Objects) == 0 {
		return s.sendResult(msg.ID, nil)
	}

	obj := parsed.Objects[0]
	var hover strings.Builder
	hover.WriteString(fmt.Sprintf("### %s\n\n", result.TargetID))
	hover.WriteString(fmt.Sprintf("**Type:** `%s`\n\n", obj.ObjectType))

	if len(obj.Fields) > 0 {
		hover.WriteString("**Fields:**\n")
		for k, v := range obj.Fields {
			hover.WriteString(fmt.Sprintf("- `%s`: %v\n", k, v))
		}
	}

	return s.sendResult(msg.ID, Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: hover.String(),
		},
	})
}

// Completion helpers

func (s *Server) completeReferences(line string, col int) []CompletionItem {
	objectIDs, err := s.db.AllObjectIDs()
	if err != nil {
		s.logDebug("Failed to get object IDs: %v", err)
		return nil
	}

	// Get any partial text after [[
	prefix := extractReferencePrefix(line, col)

	var items []CompletionItem
	for _, id := range objectIDs {
		if prefix == "" || strings.Contains(strings.ToLower(id), strings.ToLower(prefix)) {
			items = append(items, CompletionItem{
				Label:      id,
				Kind:       CompletionKindReference,
				InsertText: id + "]]",
			})
		}
	}

	return items
}

func (s *Server) completeTraits(line string, col int) []CompletionItem {
	if s.schema == nil {
		return nil
	}

	prefix := extractTraitPrefix(line, col)

	var items []CompletionItem
	for name, trait := range s.schema.Traits {
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			detail := string(trait.Type)
			if trait.Type == "enum" && len(trait.Values) > 0 {
				detail = fmt.Sprintf("enum: %v", trait.Values)
			}
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   CompletionKindProperty,
				Detail: detail,
			})
		}
	}

	return items
}

// Diagnostics

func (s *Server) publishDiagnostics(uri string) {
	doc := s.documents.Get(uri)
	if doc == nil {
		return
	}

	// Parse the document
	path := s.uriToPath(uri)
	parsed, err := parser.ParseDocument(doc.Content, path, s.vaultPath)
	if err != nil {
		s.logDebug("Failed to parse for diagnostics: %v", err)
		return
	}

	// Run validator
	objectIDs, _ := s.db.AllObjectIDs()
	validator := check.NewValidator(s.schema, objectIDs)
	if s.cfg != nil {
		validator.SetDailyDirectory(s.cfg.DailyDirectory)
	}
	issues := validator.ValidateDocument(parsed)

	// Convert to LSP diagnostics
	diagnostics := make([]Diagnostic, 0, len(issues))
	for _, issue := range issues {
		severity := DiagnosticSeverityWarning
		if issue.Level == check.LevelError {
			severity = DiagnosticSeverityError
		}

		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: issue.Line - 1, Character: 0},
				End:   Position{Line: issue.Line - 1, Character: 1000},
			},
			Severity: severity,
			Source:   "raven",
			Message:  issue.Message,
		})
	}

	if err := s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	}); err != nil {
		s.logDebug("Failed to publish diagnostics: %v", err)
	}
}

// Reindexing

func (s *Server) reindexFile(path string) error {
	return s.watcher.ReindexFile(path)
}

// Text helpers

func getLineAt(content string, lineNum int) string {
	lines := strings.Split(content, "\n")
	if lineNum < 0 || lineNum >= len(lines) {
		return ""
	}
	return lines[lineNum]
}

func isInReference(line string, col int) bool {
	// Check if we're after [[ and before ]]
	if col < 2 {
		return false
	}
	prefix := line[:col]
	lastOpen := strings.LastIndex(prefix, "[[")
	if lastOpen == -1 {
		return false
	}
	lastClose := strings.LastIndex(prefix, "]]")
	return lastClose < lastOpen
}

func isInTrait(line string, col int) bool {
	// Check if we're after @ and not in a reference
	if col < 1 {
		return false
	}
	prefix := line[:col]
	lastAt := strings.LastIndex(prefix, "@")
	if lastAt == -1 {
		return false
	}
	// Make sure we're not inside a reference
	lastOpen := strings.LastIndex(prefix, "[[")
	lastClose := strings.LastIndex(prefix, "]]")
	if lastOpen > lastClose && lastOpen < lastAt {
		return false // @ is inside a reference
	}
	return true
}

func findRefAtPosition(line string, col int) string {
	// Find [[ before position and ]] after
	before := line[:col]
	after := ""
	if col < len(line) {
		after = line[col:]
	}

	start := strings.LastIndex(before, "[[")
	if start == -1 {
		return ""
	}

	// Check if there's a closing ]] in between
	between := before[start+2:]
	if strings.Contains(between, "]]") {
		return ""
	}

	// Find the closing ]]
	end := strings.Index(after, "]]")
	if end == -1 {
		// Still typing, use rest of word
		end = len(after)
		for i, c := range after {
			if c == ' ' || c == '\n' || c == '\t' {
				end = i
				break
			}
		}
	}

	ref := between + after[:end]
	return strings.TrimSpace(ref)
}

func extractReferencePrefix(line string, col int) string {
	if col < 2 {
		return ""
	}
	before := line[:col]
	start := strings.LastIndex(before, "[[")
	if start == -1 {
		return ""
	}
	return before[start+2:]
}

func extractTraitPrefix(line string, col int) string {
	if col < 1 {
		return ""
	}
	before := line[:col]
	start := strings.LastIndex(before, "@")
	if start == -1 {
		return ""
	}
	prefix := before[start+1:]
	// Stop at ( or space
	for i, c := range prefix {
		if c == '(' || c == ' ' || c == '\t' {
			return prefix[:i]
		}
	}
	return prefix
}
