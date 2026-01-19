package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeExecutableScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func newTestServerWithVault(t *testing.T) *Server {
	t.Helper()

	tmp := t.TempDir()
	schemaPath := filepath.Join(tmp, "schema.yaml")
	if err := os.WriteFile(schemaPath, []byte("types: {}\ntraits: {}\n"), 0644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	return &Server{vaultPath: tmp}
}

func callResourcesList(t *testing.T, s *Server) []Resource {
	t.Helper()

	buf := &bytes.Buffer{}
	s.out = buf
	s.handleResourcesList(&Request{JSONRPC: "2.0", ID: 1, Method: "resources/list"})

	var resp struct {
		Result struct {
			Resources []Resource `json:"resources"`
		} `json:"result"`
		Error *RPCError `json:"error,omitempty"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &resp); err != nil {
		t.Fatalf("parse resources/list response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("resources/list error: %s", resp.Error.Message)
	}
	if len(resp.Result.Resources) == 0 {
		t.Fatal("resources/list returned no resources")
	}
	return resp.Result.Resources
}

func callResourcesRead(t *testing.T, s *Server, uri string) ResourceContent {
	t.Helper()

	resp := callResourcesReadResponse(t, s, uri)
	if resp.Error != nil {
		t.Fatalf("resources/read error for %s: %s", uri, resp.Error.Message)
	}
	if len(resp.Result.Contents) != 1 {
		t.Fatalf("expected 1 content for %s, got %d", uri, len(resp.Result.Contents))
	}
	return resp.Result.Contents[0]
}

func callResourcesReadResponse(t *testing.T, s *Server, uri string) struct {
	Result struct {
		Contents []ResourceContent `json:"contents"`
	} `json:"result"`
	Error *RPCError `json:"error,omitempty"`
} {
	t.Helper()

	buf := &bytes.Buffer{}
	s.out = buf
	paramsBytes, err := json.Marshal(map[string]string{"uri": uri})
	if err != nil {
		t.Fatalf("marshal resources/read params: %v", err)
	}
	raw := json.RawMessage(paramsBytes)
	s.handleResourcesRead(&Request{JSONRPC: "2.0", ID: 1, Method: "resources/read", Params: &raw})

	var resp struct {
		Result struct {
			Contents []ResourceContent `json:"contents"`
		} `json:"result"`
		Error *RPCError `json:"error,omitempty"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &resp); err != nil {
		t.Fatalf("parse resources/read response: %v", err)
	}
	return resp
}

func TestResourcesListIncludesGuideIndexAndTopics(t *testing.T) {
	s := newTestServerWithVault(t)
	resources := callResourcesList(t, s)

	uris := make(map[string]bool, len(resources))
	for _, resource := range resources {
		uris[resource.URI] = true
	}

	if uris["raven://guide/agent"] {
		t.Fatal("unexpected raven://guide/agent resource in list")
	}

	expected := []string{"raven://guide/index", "raven://schema/current"}
	for _, topic := range guideTopics {
		expected = append(expected, "raven://guide/"+topic.Slug)
	}

	for _, uri := range expected {
		if !uris[uri] {
			t.Fatalf("missing resource in list: %s", uri)
		}
	}
}

func TestResourcesReadGuideIndexAndTopics(t *testing.T) {
	s := newTestServerWithVault(t)

	indexContent := callResourcesRead(t, s, "raven://guide/index")
	if strings.TrimSpace(indexContent.Text) == "" {
		t.Fatal("guide index content is empty")
	}
	if indexContent.MimeType != "text/markdown" {
		t.Fatalf("expected index mimeType text/markdown, got %q", indexContent.MimeType)
	}

	for _, topic := range guideTopics {
		uri := "raven://guide/" + topic.Slug
		if !strings.Contains(indexContent.Text, uri) {
			t.Fatalf("guide index missing topic uri: %s", uri)
		}

		content := callResourcesRead(t, s, uri)
		if strings.TrimSpace(content.Text) == "" {
			t.Fatalf("guide topic %s content is empty", uri)
		}
		if content.MimeType != "text/markdown" {
			t.Fatalf("expected topic %s mimeType text/markdown, got %q", uri, content.MimeType)
		}
	}
}

func TestResourcesReadSchema(t *testing.T) {
	s := newTestServerWithVault(t)
	content := callResourcesRead(t, s, "raven://schema/current")
	if strings.TrimSpace(content.Text) == "" {
		t.Fatal("schema content is empty")
	}
	if content.MimeType != "text/yaml" {
		t.Fatalf("expected schema mimeType text/yaml, got %q", content.MimeType)
	}
	if !strings.Contains(content.Text, "types:") {
		t.Fatalf("expected schema content to include types, got: %q", content.Text)
	}
}

func TestResourcesReadUnknownGuide(t *testing.T) {
	s := newTestServerWithVault(t)
	resp := callResourcesReadResponse(t, s, "raven://guide/does-not-exist")
	if resp.Error == nil {
		t.Fatal("expected error for unknown guide resource")
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestGuideTopicsHaveTopLevelHeading(t *testing.T) {
	for _, topic := range guideTopics {
		content, ok := readAgentGuideFile(topic.Path)
		if !ok {
			t.Fatalf("failed to read guide topic: %s", topic.Path)
		}
		line := firstNonEmptyLine(content)
		if !strings.HasPrefix(line, "# ") {
			t.Fatalf("guide topic %s missing top-level heading, got: %q", topic.Path, line)
		}
	}
}

func TestGuideIndexMatchesTopics(t *testing.T) {
	indexContent, ok := readAgentGuideFile(agentGuideIndexPath)
	if !ok {
		t.Fatalf("failed to read guide index: %s", agentGuideIndexPath)
	}

	indexURIs := extractGuideURIs(indexContent)
	if len(indexURIs) == 0 {
		t.Fatal("guide index contains no guide URIs")
	}

	expected := make(map[string]bool, len(guideTopics))
	for _, topic := range guideTopics {
		expected["raven://guide/"+topic.Slug] = true
	}

	for uri := range indexURIs {
		if !expected[uri] {
			t.Fatalf("guide index references unknown topic: %s", uri)
		}
	}

	for uri := range expected {
		if !indexURIs[uri] {
			t.Fatalf("guide index missing topic: %s", uri)
		}
	}
}

func extractGuideURIs(content string) map[string]bool {
	uris := make(map[string]bool)
	const prefix = "raven://guide/"
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, prefix) {
			continue
		}
		start := strings.Index(line, prefix)
		if start == -1 {
			continue
		}
		end := strings.IndexAny(line[start:], " )]`")
		if end == -1 {
			end = len(line) - start
		}
		uri := line[start : start+end]
		uri = strings.Trim(uri, "`")
		if uri != "" {
			uris[uri] = true
		}
	}
	return uris
}

func firstNonEmptyLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func TestExecuteRvnTreatsOkFalseAsErrorEvenWithExit0(t *testing.T) {
	// Skip on Windows just in case; Raven targets mac/linux.
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on windows in this test")
	}

	tmp := t.TempDir()
	script := writeExecutableScript(t, tmp, "fake-rvn.sh", `#!/bin/sh
echo '{"ok":false,"error":{"code":"REQUIRED_FIELD_MISSING","message":"Missing required fields: name","suggestion":"Provide field: {name: ...}"}}'
exit 0
`)

	s := &Server{vaultPath: tmp, executable: script}
	out, isErr := s.executeRvn([]string{"new", "--json", "--", "person", "Freya"})
	if !isErr {
		t.Fatalf("expected isError=true, got false; out=%s", out)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if okVal, _ := parsed["ok"].(bool); okVal {
		t.Fatalf("expected ok=false, got ok=true; out=%s", out)
	}
}

func TestExecuteRvnWrapsNonJSONOutputOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on windows in this test")
	}

	tmp := t.TempDir()
	script := writeExecutableScript(t, tmp, "fake-rvn.sh", `#!/bin/sh
echo "something went wrong" 1>&2
exit 1
`)

	s := &Server{vaultPath: tmp, executable: script}
	out, isErr := s.executeRvn([]string{"new", "--json", "--", "person", "Freya"})
	if !isErr {
		t.Fatalf("expected isError=true, got false; out=%s", out)
	}

	var parsed struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details struct {
				Output string `json:"output"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if parsed.OK {
		t.Fatalf("expected ok=false, got ok=true; out=%s", out)
	}
	if parsed.Error.Code != "EXECUTION_ERROR" {
		t.Fatalf("expected error.code=EXECUTION_ERROR, got %q; out=%s", parsed.Error.Code, out)
	}
	if parsed.Error.Details.Output == "" {
		t.Fatalf("expected error.details.output to be present; out=%s", out)
	}
}
