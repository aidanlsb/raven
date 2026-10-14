package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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

func TestRequestTriggersRefreshAfterExternalReindex(t *testing.T) {
	vaultPath := newTestVault(t)
	client, server := startConfiguredTestServer(t, vaultPath, Options{
		DiagnosticsDebounce: -1,
	}, nil)
	client.initialize(vaultPath)

	content := "Link: [[\n"
	uri := client.openDocument(filepath.Join(vaultPath, "scratch.md"), content)
	client.diagnosticsFor(uri)

	odinPath := filepath.Join(vaultPath, "people", "odin.md")
	odinContent := "---\ntype: person\nname: Odin\n---\n\n# Odin\n"
	if err := os.WriteFile(odinPath, []byte(odinContent), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}
	rt := testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{})
	if _, err := reindexsvc.Run(rt, reindexsvc.RunRequest{}); err != nil {
		t.Fatalf("external incremental reindex: %v", err)
	}

	complete := func() CompletionList {
		t.Helper()
		raw := client.request("textDocument/completion", TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: len("Link: [[")},
		})
		var list CompletionList
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatalf("invalid completion result: %v", err)
		}
		return list
	}

	first := complete()
	if completionHasLabel(first, "people/odin") {
		t.Fatal("first request after external reindex returned rebuilt cache instead of the current pointer")
	}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				ws, _, ok := server.snapshot(uri)
				if !ok {
					t.Error("snapshot missed open document during external refresh")
					return
				}
				_ = ws.catalog.Objects
			}
		}()
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if completionHasLabel(complete(), "people/odin") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("later request did not see people/odin after snapshot-triggered refresh")
		}
	}
	wg.Wait()
	waitRefreshIdle(t, server, 5*time.Second)
}

func completionHasLabel(list CompletionList, label string) bool {
	for _, item := range list.Items {
		if item.Label == label {
			return true
		}
	}
	return false
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
