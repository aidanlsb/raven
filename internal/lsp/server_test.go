package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

// testClient drives a Server over OS pipes from tests.
type testClient struct {
	t      *testing.T
	writer *os.File
	reader *bufio.Reader
	nextID int

	// notifications buffered while waiting for responses
	notifications []Notification
	done          chan error
}

type rawNotification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func newTestVault(t *testing.T) string {
	t.Helper()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\nalias: The Queen\n---\n\n# Freya\n\nNotes about Freya.\n").
		WithFile("people/loki.md", "---\ntype: person\nname: Loki\n---\n\nTrickster.\n").
		WithFile("projects/bifrost.md", "---\ntype: project\ntitle: Bifrost\nstatus: active\nowner: \"[[people/freya]]\"\n---\n\n# Bifrost\n\nWorking with [[people/freya]] on the bridge.\n\n## Tasks\n\n- @due(2026-01-15) Fix the rainbow shader\n").
		Build()
	return vault.Path
}

func startTestServer(t *testing.T, vaultPath string) *testClient {
	t.Helper()

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	server := NewServer(Options{
		ExplicitVaultPath:   vaultPath,
		In:                  inR,
		Out:                 outW,
		DiagnosticsDebounce: -1, // synchronous diagnostics in tests
	})

	client := &testClient{
		t:      t,
		writer: inW,
		reader: bufio.NewReader(outR),
		done:   make(chan error, 1),
	}

	go func() {
		client.done <- server.Run()
	}()

	t.Cleanup(func() {
		client.notifyServer("exit", nil)
		select {
		case <-client.done:
		case <-time.After(5 * time.Second):
			t.Error("server did not exit")
		}
		inW.Close()
		inR.Close()
		outW.Close()
		outR.Close()
	})

	return client
}

func (c *testClient) send(message interface{}) {
	c.t.Helper()
	if err := writeMessage(c.writer, message); err != nil {
		c.t.Fatalf("failed to send message: %v", err)
	}
}

func (c *testClient) notifyServer(method string, params interface{}) {
	c.send(Notification{JSONRPC: "2.0", Method: method, Params: params})
}

// request sends a request and returns the raw result, failing on RPC errors.
func (c *testClient) request(method string, params interface{}) json.RawMessage {
	c.t.Helper()
	result, respErr := c.requestRaw(method, params)
	if respErr != nil {
		c.t.Fatalf("%s returned error: %d %s", method, respErr.Code, respErr.Message)
	}
	return result
}

func (c *testClient) requestRaw(method string, params interface{}) (json.RawMessage, *ResponseError) {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	c.send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})

	for {
		body := c.readMessage()
		var resp struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
			Result json.RawMessage  `json:"result"`
			Error  *ResponseError   `json:"error"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			c.t.Fatalf("invalid message from server: %v: %s", err, body)
		}
		if resp.ID == nil {
			c.notifications = append(c.notifications, Notification{Method: resp.Method, Params: resp.Params})
			continue
		}
		var gotID int
		if err := json.Unmarshal(*resp.ID, &gotID); err != nil || gotID != id {
			c.t.Fatalf("unexpected response id %s (want %d)", *resp.ID, id)
		}
		return resp.Result, resp.Error
	}
}

// waitNotification returns the next notification with the given method,
// consuming buffered notifications first.
func (c *testClient) waitNotification(method string) json.RawMessage {
	c.t.Helper()

	for i, n := range c.notifications {
		if n.Method == method {
			c.notifications = append(c.notifications[:i], c.notifications[i+1:]...)
			params, _ := json.Marshal(n.Params)
			return params
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body := c.readMessage()
		var raw rawNotification
		if err := json.Unmarshal(body, &raw); err != nil {
			c.t.Fatalf("invalid message from server: %v: %s", err, body)
		}
		if raw.Method == method {
			return raw.Params
		}
		c.notifications = append(c.notifications, Notification{Method: raw.Method, Params: raw.Params})
	}
	c.t.Fatalf("timed out waiting for notification %s", method)
	return nil
}

func (c *testClient) readMessage() []byte {
	c.t.Helper()
	body, err := readMessage(c.reader)
	if err != nil {
		c.t.Fatalf("failed to read message from server: %v", err)
	}
	return body
}

// initialize performs the initialize handshake and returns the result.
func (c *testClient) initialize(rootPath string) InitializeResult {
	c.t.Helper()
	params := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"general": map[string]interface{}{
				"positionEncodings": []string{"utf-8"},
			},
		},
	}
	if rootPath != "" {
		params["rootUri"] = pathToURI(rootPath)
	}
	raw := c.request("initialize", params)
	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		c.t.Fatalf("invalid initialize result: %v", err)
	}
	c.notifyServer("initialized", map[string]interface{}{})
	return result
}

func (c *testClient) openDocument(path, content string) string {
	c.t.Helper()
	uri := pathToURI(path)
	c.notifyServer("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: content},
	})
	return uri
}

func (c *testClient) diagnosticsFor(uri string) PublishDiagnosticsParams {
	c.t.Helper()
	for {
		raw := c.waitNotification("textDocument/publishDiagnostics")
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(raw, &params); err != nil {
			c.t.Fatalf("invalid publishDiagnostics params: %v", err)
		}
		if params.URI == uri {
			return params
		}
	}
}

func TestServerLifecycleAndCapabilities(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)

	result := client.initialize(vaultPath)
	if result.Capabilities.PositionEncoding != encodingUTF8 {
		t.Errorf("PositionEncoding = %q, want utf-8", result.Capabilities.PositionEncoding)
	}
	if !result.Capabilities.DefinitionProvider || !result.Capabilities.ReferencesProvider || !result.Capabilities.HoverProvider {
		t.Error("expected definition/references/hover providers to be enabled")
	}
	if result.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
	}
	if result.ServerInfo.Name != "raven" {
		t.Errorf("ServerInfo.Name = %q, want raven", result.ServerInfo.Name)
	}
}

func TestServerRequiresInitialize(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)

	_, respErr := client.requestRaw("textDocument/definition", TextDocumentPositionParams{})
	if respErr == nil || respErr.Code != codeServerNotInitialized {
		t.Fatalf("expected server-not-initialized error, got %v", respErr)
	}
}

func TestDiagnostics(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)
	client.initialize(vaultPath)

	t.Run("missing reference with precise range", func(t *testing.T) {
		content := "Linking to [[people/nobody]] here.\n"
		uri := client.openDocument(filepath.Join(vaultPath, "note.md"), content)
		params := client.diagnosticsFor(uri)

		var found *Diagnostic
		for i, d := range params.Diagnostics {
			if d.Code == "missing_reference" {
				found = &params.Diagnostics[i]
			}
		}
		if found == nil {
			t.Fatalf("no missing_reference diagnostic in %+v", params.Diagnostics)
		}
		wantStart := strings.Index(content, "[[")
		wantEnd := strings.Index(content, "]]") + 2
		if found.Range.Start.Character != wantStart || found.Range.End.Character != wantEnd {
			t.Errorf("range = [%d, %d), want [%d, %d)",
				found.Range.Start.Character, found.Range.End.Character, wantStart, wantEnd)
		}
		if found.Severity != severityError {
			t.Errorf("severity = %d, want %d", found.Severity, severityError)
		}
		if found.Source != "raven" {
			t.Errorf("source = %q, want raven", found.Source)
		}
	})

	t.Run("undefined trait with annotation range", func(t *testing.T) {
		content := "- @nonexistent(5) do something\n"
		uri := client.openDocument(filepath.Join(vaultPath, "note2.md"), content)
		params := client.diagnosticsFor(uri)

		var found *Diagnostic
		for i, d := range params.Diagnostics {
			if d.Code == "undefined_trait" {
				found = &params.Diagnostics[i]
			}
		}
		if found == nil {
			t.Fatalf("no undefined_trait diagnostic in %+v", params.Diagnostics)
		}
		wantStart := strings.Index(content, "@")
		wantEnd := strings.Index(content, ")") + 1
		if found.Range.Start.Character != wantStart || found.Range.End.Character != wantEnd {
			t.Errorf("range = [%d, %d), want [%d, %d)",
				found.Range.Start.Character, found.Range.End.Character, wantStart, wantEnd)
		}
		if found.Severity != severityWarning {
			t.Errorf("severity = %d, want %d", found.Severity, severityWarning)
		}
	})

	t.Run("unknown frontmatter key", func(t *testing.T) {
		content := "---\ntype: person\nname: Thor\nhammer: mjolnir\n---\n"
		uri := client.openDocument(filepath.Join(vaultPath, "people/thor.md"), content)
		params := client.diagnosticsFor(uri)

		found := false
		for _, d := range params.Diagnostics {
			if d.Code == "unknown_frontmatter_key" {
				found = true
			}
		}
		if !found {
			t.Errorf("no unknown_frontmatter_key diagnostic in %+v", params.Diagnostics)
		}
	})

	t.Run("clean document has no diagnostics", func(t *testing.T) {
		content := "Linking to [[people/freya]].\n"
		uri := client.openDocument(filepath.Join(vaultPath, "note3.md"), content)
		params := client.diagnosticsFor(uri)
		for _, d := range params.Diagnostics {
			t.Errorf("unexpected diagnostic: %s %s", d.Code, d.Message)
		}
	})

	t.Run("didChange updates diagnostics", func(t *testing.T) {
		uri := client.openDocument(filepath.Join(vaultPath, "note4.md"), "Nothing here.\n")
		client.diagnosticsFor(uri)

		client.notifyServer("textDocument/didChange", DidChangeTextDocumentParams{
			TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
			ContentChanges: []TextDocumentContentChangeEvent{{Text: "Now [[people/missing]] appears.\n"}},
		})
		params := client.diagnosticsFor(uri)
		if len(params.Diagnostics) == 0 {
			t.Error("expected diagnostics after didChange introduced a broken ref")
		}
	})

	t.Run("didClose clears diagnostics", func(t *testing.T) {
		uri := client.openDocument(filepath.Join(vaultPath, "note5.md"), "Broken [[nope/nothing]].\n")
		params := client.diagnosticsFor(uri)
		if len(params.Diagnostics) == 0 {
			t.Fatal("expected diagnostics for broken ref")
		}

		client.notifyServer("textDocument/didClose", DidCloseTextDocumentParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
		})
		params = client.diagnosticsFor(uri)
		if len(params.Diagnostics) != 0 {
			t.Errorf("expected cleared diagnostics, got %+v", params.Diagnostics)
		}
	})
}

func TestDefinition(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)
	client.initialize(vaultPath)

	content := "See [[people/freya]] and [[The Queen]] and [[people/nobody]].\n"
	uri := client.openDocument(filepath.Join(vaultPath, "note.md"), content)
	client.diagnosticsFor(uri)

	requestDefinition := func(character int) []Location {
		raw := client.request("textDocument/definition", TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: character},
		})
		var locations []Location
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &locations); err != nil {
				t.Fatalf("invalid definition result: %v", err)
			}
		}
		return locations
	}

	t.Run("path ref resolves to file", func(t *testing.T) {
		locations := requestDefinition(strings.Index(content, "people/freya") + 3)
		if len(locations) != 1 {
			t.Fatalf("got %d locations, want 1", len(locations))
		}
		wantURI := pathToURI(filepath.Join(vaultPath, "people/freya.md"))
		if locations[0].URI != wantURI {
			t.Errorf("URI = %q, want %q", locations[0].URI, wantURI)
		}
	})

	t.Run("alias ref resolves", func(t *testing.T) {
		locations := requestDefinition(strings.Index(content, "The Queen") + 2)
		if len(locations) != 1 {
			t.Fatalf("got %d locations, want 1", len(locations))
		}
		if !strings.HasSuffix(locations[0].URI, "people/freya.md") {
			t.Errorf("alias resolved to %q, want people/freya.md", locations[0].URI)
		}
	})

	t.Run("missing ref returns nothing", func(t *testing.T) {
		locations := requestDefinition(strings.Index(content, "people/nobody") + 3)
		if len(locations) != 0 {
			t.Errorf("got %d locations, want 0", len(locations))
		}
	})

	t.Run("cursor outside a ref returns nothing", func(t *testing.T) {
		locations := requestDefinition(0)
		if len(locations) != 0 {
			t.Errorf("got %d locations, want 0", len(locations))
		}
	})

	t.Run("section ref resolves to heading line", func(t *testing.T) {
		sectionContent := "Jump to [[projects/bifrost#tasks]].\n"
		sectionURI := client.openDocument(filepath.Join(vaultPath, "note6.md"), sectionContent)
		client.diagnosticsFor(sectionURI)

		raw := client.request("textDocument/definition", TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: sectionURI},
			Position:     Position{Line: 0, Character: strings.Index(sectionContent, "bifrost#tasks")},
		})
		var locations []Location
		if err := json.Unmarshal(raw, &locations); err != nil {
			t.Fatalf("invalid definition result: %v", err)
		}
		if len(locations) != 1 {
			t.Fatalf("got %d locations, want 1", len(locations))
		}
		if !strings.HasSuffix(locations[0].URI, "projects/bifrost.md") {
			t.Errorf("URI = %q, want projects/bifrost.md", locations[0].URI)
		}
		// "## Tasks" is on 1-indexed line 12 → 0-indexed 11.
		if locations[0].Range.Start.Line != 11 {
			t.Errorf("section line = %d, want 11", locations[0].Range.Start.Line)
		}
	})
}

func TestReferences(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)
	client.initialize(vaultPath)

	// Open freya.md itself; references should come from bifrost.md
	// (body wikilink and frontmatter owner field).
	freyaPath := filepath.Join(vaultPath, "people/freya.md")
	content, err := os.ReadFile(freyaPath)
	if err != nil {
		t.Fatal(err)
	}
	uri := client.openDocument(freyaPath, string(content))
	client.diagnosticsFor(uri)

	raw := client.request("textDocument/references", ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 6, Character: 0}, // in the body, not on a link
		},
		Context: ReferenceContext{IncludeDeclaration: false},
	})
	var locations []Location
	if err := json.Unmarshal(raw, &locations); err != nil {
		t.Fatalf("invalid references result: %v", err)
	}

	if len(locations) < 2 {
		t.Fatalf("got %d reference locations, want >= 2: %+v", len(locations), locations)
	}
	bifrostURI := pathToURI(filepath.Join(vaultPath, "projects/bifrost.md"))
	var bodyRef *Location
	for i, loc := range locations {
		if loc.URI != bifrostURI {
			t.Errorf("unexpected reference URI %q", loc.URI)
			continue
		}
		if loc.Range.Start.Line == 9 { // body wikilink line (0-indexed)
			bodyRef = &locations[i]
		}
	}
	if bodyRef == nil {
		t.Fatalf("missing body reference at line 9 in %+v", locations)
	}
	// The body ref should have a precise column range around [[people/freya]].
	if bodyRef.Range.End.Character <= bodyRef.Range.Start.Character {
		t.Errorf("expected non-empty column range, got [%d, %d)",
			bodyRef.Range.Start.Character, bodyRef.Range.End.Character)
	}
}

func TestHover(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)
	client.initialize(vaultPath)

	content := "See [[people/freya]].\n"
	uri := client.openDocument(filepath.Join(vaultPath, "note.md"), content)
	client.diagnosticsFor(uri)

	raw := client.request("textDocument/hover", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: strings.Index(content, "freya")},
	})
	var hover Hover
	if err := json.Unmarshal(raw, &hover); err != nil {
		t.Fatalf("invalid hover result: %v", err)
	}

	if hover.Contents.Kind != "markdown" {
		t.Errorf("hover kind = %q, want markdown", hover.Contents.Kind)
	}
	for _, want := range []string{"people/freya", "person", "Freya"} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Errorf("hover missing %q:\n%s", want, hover.Contents.Value)
		}
	}
	if hover.Range == nil {
		t.Fatal("expected hover range")
	}
	wantStart := strings.Index(content, "[[")
	if hover.Range.Start.Character != wantStart {
		t.Errorf("hover range start = %d, want %d", hover.Range.Start.Character, wantStart)
	}
}

func TestCompletion(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)
	client.initialize(vaultPath)

	complete := func(content string, line, character int) CompletionList {
		t.Helper()
		uri := client.openDocument(filepath.Join(vaultPath, fmt.Sprintf("scratch-%d.md", client.nextID)), content)
		client.diagnosticsFor(uri)
		raw := client.request("textDocument/completion", TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: line, Character: character},
		})
		var list CompletionList
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatalf("invalid completion result: %v", err)
		}
		return list
	}

	labels := func(list CompletionList) map[string]CompletionItem {
		out := map[string]CompletionItem{}
		for _, item := range list.Items {
			out[item.Label] = item
		}
		return out
	}

	t.Run("wikilink completion lists objects and aliases", func(t *testing.T) {
		content := "Link: [[\n"
		list := complete(content, 0, len("Link: [["))
		items := labels(list)
		if _, ok := items["people/freya"]; !ok {
			t.Errorf("missing people/freya in %v", list.Items)
		}
		if _, ok := items["The Queen"]; !ok {
			t.Errorf("missing alias The Queen in %v", list.Items)
		}
	})

	t.Run("wikilink completion filters by prefix", func(t *testing.T) {
		content := "Link: [[freya\n"
		list := complete(content, 0, len("Link: [[freya"))
		items := labels(list)
		if _, ok := items["people/freya"]; !ok {
			t.Errorf("missing people/freya in %v", list.Items)
		}
		if _, ok := items["people/loki"]; ok {
			t.Errorf("people/loki should be filtered out: %v", list.Items)
		}
		freya := items["people/freya"]
		if freya.TextEdit == nil {
			t.Fatal("expected text edit")
		}
		if freya.TextEdit.Range.Start.Character != len("Link: [[") {
			t.Errorf("edit start = %d, want %d", freya.TextEdit.Range.Start.Character, len("Link: [["))
		}
	})

	t.Run("trait completion lists schema traits", func(t *testing.T) {
		content := "- @\n"
		list := complete(content, 0, len("- @"))
		items := labels(list)
		if _, ok := items["due"]; !ok {
			t.Errorf("missing due in %v", list.Items)
		}
		if _, ok := items["priority"]; !ok {
			t.Errorf("missing priority in %v", list.Items)
		}
		if items["priority"].Detail == "" {
			t.Error("expected detail for enum trait")
		}
	})

	t.Run("frontmatter key completion uses declared type", func(t *testing.T) {
		content := "---\ntype: project\n\n---\n"
		list := complete(content, 2, 0)
		items := labels(list)
		for _, want := range []string{"title", "status", "owner", "alias"} {
			if _, ok := items[want]; !ok {
				t.Errorf("missing %q in %v", want, list.Items)
			}
		}
		if _, ok := items["email"]; ok {
			t.Errorf("email belongs to person, not project: %v", list.Items)
		}
	})

	t.Run("no completion in plain text", func(t *testing.T) {
		content := "Just some text\n"
		list := complete(content, 0, 4)
		if len(list.Items) != 0 {
			t.Errorf("expected no items, got %v", list.Items)
		}
	})
}

func TestDidSaveRefreshesIndex(t *testing.T) {
	vaultPath := newTestVault(t)
	client := startTestServer(t, vaultPath)
	client.initialize(vaultPath)

	// A ref to a not-yet-existing person is a missing_reference.
	content := "See [[people/odin]].\n"
	noteURI := client.openDocument(filepath.Join(vaultPath, "note.md"), content)
	params := client.diagnosticsFor(noteURI)
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected missing_reference diagnostic before save")
	}

	// Create the target file on disk and notify a save for it.
	odinPath := filepath.Join(vaultPath, "people/odin.md")
	odinContent := "---\ntype: person\nname: Odin\n---\n"
	if err := os.WriteFile(odinPath, []byte(odinContent), 0644); err != nil {
		t.Fatal(err)
	}
	odinURI := client.openDocument(odinPath, odinContent)
	client.diagnosticsFor(odinURI)
	client.notifyServer("textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: odinURI},
	})

	// didSave republished diagnostics for all open docs; the note's broken
	// ref should now resolve.
	deadline := time.Now().Add(5 * time.Second)
	for {
		params = client.diagnosticsFor(noteURI)
		if len(params.Diagnostics) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("diagnostics still present after save: %+v", params.Diagnostics)
		}
	}
}

func TestVaultResolutionFromWorkspaceRoot(t *testing.T) {
	vaultPath := newTestVault(t)

	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	server := NewServer(Options{
		// No explicit vault: the workspace root should be detected.
		In:                  inR,
		Out:                 outW,
		DiagnosticsDebounce: -1,
	})
	client := &testClient{t: t, writer: inW, reader: bufio.NewReader(outR), done: make(chan error, 1)}
	go func() { client.done <- server.Run() }()
	t.Cleanup(func() {
		client.notifyServer("exit", nil)
		<-client.done
		inW.Close()
		inR.Close()
		outW.Close()
		outR.Close()
	})

	result := client.initialize(vaultPath)
	if result.ServerInfo.Name != "raven" {
		t.Errorf("initialize failed with workspace-root vault detection")
	}
}

func TestVaultResolutionFailure(t *testing.T) {
	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	server := NewServer(Options{In: inR, Out: outW, DiagnosticsDebounce: -1})
	client := &testClient{t: t, writer: inW, reader: bufio.NewReader(outR), done: make(chan error, 1)}
	go func() { client.done <- server.Run() }()
	t.Cleanup(func() {
		client.notifyServer("exit", nil)
		<-client.done
		inW.Close()
		inR.Close()
		outW.Close()
		outR.Close()
	})

	// Non-vault root and no configured vault.
	nonVault := t.TempDir()
	_, respErr := client.requestRaw("initialize", map[string]interface{}{
		"rootUri":      pathToURI(nonVault),
		"capabilities": map[string]interface{}{},
	})
	if respErr == nil {
		t.Fatal("expected initialize to fail without a vault")
	}
	if !strings.Contains(respErr.Message, "vault") {
		t.Errorf("error message should mention vault: %s", respErr.Message)
	}
}
