package lsp

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSaveRefreshDoesNotBlockInFlightRequests(t *testing.T) {
	vaultPath := newTestVault(t)
	begin := make(chan struct{})
	allow := make(chan struct{})
	client, server := startConfiguredTestServer(t, vaultPath, Options{
		DiagnosticsDebounce: 20 * time.Millisecond,
	}, func(s *Server) {
		s.refreshBegin = begin
		s.refreshAllow = allow
	})
	client.initialize(vaultPath)

	content := "See [[people/freya]] and [[\n"
	uri := client.openDocument(filepath.Join(vaultPath, "note.md"), content)
	client.diagnosticsFor(uri)

	client.notifyServer("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: content}},
	})
	client.notifyServer("textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})

	waitBegin(t, begin)

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				ws, doc, ok := server.snapshot(uri)
				if !ok {
					t.Error("snapshot missed open document during save refresh")
					return
				}
				_ = doc.content
				_ = ws.schema()
				_ = ws.vaultConfig()
				_ = ws.catalog.Objects
				_ = ws.newValidator()
			}
		}()
	}

	// Protocol requests must complete while refresh is still gated. If
	// handleDidSave still held s.mu across SmartReindex, this would time out.
	raw := client.request("textDocument/completion", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: len(content) - 1},
	})
	var list CompletionList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("invalid completion result during save refresh: %v", err)
	}
	client.request("textDocument/hover", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: len("See [[people/")},
	})
	client.request("textDocument/definition", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: len("See [[people/")},
	})

	allow <- struct{}{}
	client.request("textDocument/references", ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: len("See [[people/")},
		},
		Context: ReferenceContext{IncludeDeclaration: true},
	})
	wg.Wait()
	waitRefreshIdle(t, server, 5*time.Second)
}

func TestSaveRefreshCoalesces(t *testing.T) {
	vaultPath := newTestVault(t)
	begin := make(chan struct{})
	allow := make(chan struct{})
	client, server := startConfiguredTestServer(t, vaultPath, Options{
		DiagnosticsDebounce: -1,
	}, func(s *Server) {
		s.refreshBegin = begin
		s.refreshAllow = allow
	})
	client.initialize(vaultPath)

	content := "Link: [[\n"
	uri := client.openDocument(filepath.Join(vaultPath, "scratch.md"), content)
	client.diagnosticsFor(uri)

	save := func() {
		client.notifyServer("textDocument/didSave", DidSaveTextDocumentParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
		})
	}

	save()
	waitBegin(t, begin)

	for range 20 {
		save()
	}

	// A request after the extra saves both drains the dispatch loop (so every
	// didSave has set the pending flag) and proves request handling is not
	// waiting on the gated refresh.
	raw := client.request("textDocument/completion", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: len("Link: [[")},
	})
	var list CompletionList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("invalid completion result: %v", err)
	}

	allow <- struct{}{}
	waitBegin(t, begin)
	allow <- struct{}{}
	waitRefreshIdle(t, server, 5*time.Second)

	if got := server.refreshCount.Load(); got != 2 {
		t.Fatalf("refresh count = %d, want 2 (one in-flight + one coalesced)", got)
	}
}

func waitBegin(t *testing.T, begin <-chan struct{}) {
	t.Helper()
	select {
	case <-begin:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for background refresh to start")
	}
}

func waitRefreshIdle(t *testing.T, s *Server, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.refreshIdle() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for background refresh to become idle")
}
