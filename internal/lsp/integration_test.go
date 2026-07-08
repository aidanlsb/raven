//go:build integration

package lsp

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

// lspBinaryClient drives a real `rvn lsp` subprocess over stdio.
type lspBinaryClient struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID int
}

func startBinaryLSP(t *testing.T, vaultPath string) *lspBinaryClient {
	t.Helper()

	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "lsp", "--vault-path", vaultPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to open stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to open stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start rvn lsp: %v", err)
	}

	client := &lspBinaryClient{
		t:      t,
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
	}

	t.Cleanup(func() {
		client.notify("exit", nil)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Error("rvn lsp did not exit after exit notification")
		}
	})

	return client
}

func (c *lspBinaryClient) send(message interface{}) {
	c.t.Helper()
	if err := writeMessage(c.stdin, message); err != nil {
		c.t.Fatalf("failed to write to rvn lsp: %v", err)
	}
}

func (c *lspBinaryClient) notify(method string, params interface{}) {
	c.send(map[string]interface{}{"jsonrpc": "2.0", "method": method, "params": params})
}

// request sends a request and reads until its response arrives, returning the
// result. Notifications received in between are returned to callers via
// waitNotification's buffer-free design: they are simply skipped here, so
// callers needing notifications must read them before issuing requests.
func (c *lspBinaryClient) request(method string, params interface{}) json.RawMessage {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	c.send(map[string]interface{}{"jsonrpc": "2.0", "id": id, "method": method, "params": params})

	for {
		body := c.read()
		var resp struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *ResponseError  `json:"error"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			c.t.Fatalf("invalid message: %v: %s", err, body)
		}
		if resp.ID == nil {
			continue // notification
		}
		if *resp.ID != id {
			c.t.Fatalf("unexpected response id %d (want %d)", *resp.ID, id)
		}
		if resp.Error != nil {
			c.t.Fatalf("%s returned error: %d %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result
	}
}

func (c *lspBinaryClient) waitNotification(method string) json.RawMessage {
	c.t.Helper()
	for {
		body := c.read()
		var msg struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			c.t.Fatalf("invalid message: %v: %s", err, body)
		}
		if msg.ID == nil && msg.Method == method {
			return msg.Params
		}
	}
}

func (c *lspBinaryClient) read() []byte {
	c.t.Helper()
	body, err := readMessage(c.reader)
	if err != nil {
		c.t.Fatalf("failed to read from rvn lsp: %v", err)
	}
	return body
}

// TestBinaryLSPRoundTrip exercises the real binary end to end:
// initialize → didOpen → diagnostics → definition → hover → shutdown.
func TestBinaryLSPRoundTrip(t *testing.T) {
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n\n# Freya\n").
		Build()

	client := startBinaryLSP(t, vault.Path)

	// initialize
	rawInit := client.request("initialize", map[string]interface{}{
		"rootUri": pathToURI(vault.Path),
		"capabilities": map[string]interface{}{
			"general": map[string]interface{}{"positionEncodings": []string{"utf-8"}},
		},
	})
	var initResult InitializeResult
	if err := json.Unmarshal(rawInit, &initResult); err != nil {
		t.Fatalf("invalid initialize result: %v", err)
	}
	if initResult.ServerInfo.Name != "raven" {
		t.Errorf("ServerInfo.Name = %q, want raven", initResult.ServerInfo.Name)
	}
	if initResult.Capabilities.PositionEncoding != "utf-8" {
		t.Errorf("PositionEncoding = %q, want utf-8", initResult.Capabilities.PositionEncoding)
	}
	client.notify("initialized", map[string]interface{}{})

	// didOpen a document with one good and one broken ref.
	content := "See [[people/freya]] and [[people/nobody]].\n"
	notePath := filepath.Join(vault.Path, "note.md")
	noteURI := pathToURI(notePath)
	client.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        noteURI,
			"languageId": "markdown",
			"version":    1,
			"text":       content,
		},
	})

	// Diagnostics should flag exactly the broken ref.
	rawDiag := client.waitNotification("textDocument/publishDiagnostics")
	var diag PublishDiagnosticsParams
	if err := json.Unmarshal(rawDiag, &diag); err != nil {
		t.Fatalf("invalid diagnostics: %v", err)
	}
	if diag.URI != noteURI {
		t.Fatalf("diagnostics URI = %q, want %q", diag.URI, noteURI)
	}
	foundMissing := false
	for _, d := range diag.Diagnostics {
		if d.Code == "missing_reference" && strings.Contains(d.Message, "people/nobody") {
			foundMissing = true
		}
		if strings.Contains(d.Message, "people/freya]]") {
			t.Errorf("unexpected diagnostic for valid ref: %s", d.Message)
		}
	}
	if !foundMissing {
		t.Fatalf("missing_reference diagnostic not found in %+v", diag.Diagnostics)
	}

	// Definition on the valid wikilink.
	rawDef := client.request("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": noteURI},
		"position":     map[string]interface{}{"line": 0, "character": strings.Index(content, "freya")},
	})
	var locations []Location
	if err := json.Unmarshal(rawDef, &locations); err != nil {
		t.Fatalf("invalid definition result: %v", err)
	}
	if len(locations) != 1 || !strings.HasSuffix(locations[0].URI, "people/freya.md") {
		t.Fatalf("definition = %+v, want people/freya.md", locations)
	}

	// Hover on the same wikilink.
	rawHover := client.request("textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": noteURI},
		"position":     map[string]interface{}{"line": 0, "character": strings.Index(content, "freya")},
	})
	var hover Hover
	if err := json.Unmarshal(rawHover, &hover); err != nil {
		t.Fatalf("invalid hover result: %v", err)
	}
	if !strings.Contains(hover.Contents.Value, "people/freya") {
		t.Errorf("hover missing target ID:\n%s", hover.Contents.Value)
	}

	// Completion after "[[".
	completionContent := "Link: [[\n"
	scratchURI := pathToURI(filepath.Join(vault.Path, "scratch.md"))
	client.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        scratchURI,
			"languageId": "markdown",
			"version":    1,
			"text":       completionContent,
		},
	})
	client.waitNotification("textDocument/publishDiagnostics")
	rawCompletion := client.request("textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": scratchURI},
		"position":     map[string]interface{}{"line": 0, "character": len("Link: [[")},
	})
	var completion CompletionList
	if err := json.Unmarshal(rawCompletion, &completion); err != nil {
		t.Fatalf("invalid completion result: %v", err)
	}
	foundFreya := false
	for _, item := range completion.Items {
		if item.Label == "people/freya" {
			foundFreya = true
		}
	}
	if !foundFreya {
		t.Errorf("completion missing people/freya: %+v", completion.Items)
	}

	// Graceful shutdown.
	client.request("shutdown", nil)
}
