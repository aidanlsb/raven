package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
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
	return callResourcesReadResponseWithParams(t, s, map[string]interface{}{"uri": uri})
}

func callResourcesReadResponseWithParams(t *testing.T, s *Server, params map[string]interface{}) struct {
	Result struct {
		Contents []ResourceContent `json:"contents"`
	} `json:"result"`
	Error *RPCError `json:"error,omitempty"`
} {
	t.Helper()

	buf := &bytes.Buffer{}
	s.out = buf
	paramsBytes, err := json.Marshal(params)
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

func hasResourceURI(resources []Resource, uri string) bool {
	for _, resource := range resources {
		if resource.URI == uri {
			return true
		}
	}
	return false
}

func TestResourcesListIncludesGuideIndexAndTopics(t *testing.T) {
	t.Parallel()
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

func TestResourcesListOmitsAgentInstructionsWhenMissing(t *testing.T) {
	t.Parallel()
	s := newTestServerWithVault(t)
	resources := callResourcesList(t, s)

	if hasResourceURI(resources, vaultAgentInstructionsResourceURI) {
		t.Fatalf("did not expect %s when AGENTS.md is missing", vaultAgentInstructionsResourceURI)
	}
}

func TestResourcesListIncludesAgentInstructionsWhenPresent(t *testing.T) {
	t.Parallel()
	s := newTestServerWithVault(t)
	agentPath := filepath.Join(s.vaultPath, "AGENTS.md")
	if err := os.WriteFile(agentPath, []byte("# Agent Rules\n"), 0644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	resources := callResourcesList(t, s)
	if !hasResourceURI(resources, vaultAgentInstructionsResourceURI) {
		t.Fatalf("expected %s when AGENTS.md exists", vaultAgentInstructionsResourceURI)
	}
}

func TestResourcesReadGuideIndexAndTopics(t *testing.T) {
	t.Parallel()
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

func TestResourcesReadAgentInstructions(t *testing.T) {
	t.Parallel()
	s := newTestServerWithVault(t)
	expected := "# Agent Rules\nAlways run checks.\n"
	agentPath := filepath.Join(s.vaultPath, "AGENTS.md")
	if err := os.WriteFile(agentPath, []byte(expected), 0644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	content := callResourcesRead(t, s, vaultAgentInstructionsResourceURI)
	if content.MimeType != "text/markdown" {
		t.Fatalf("expected mimeType text/markdown, got %q", content.MimeType)
	}
	if content.Text != expected {
		t.Fatalf("unexpected AGENTS.md content: got %q want %q", content.Text, expected)
	}
}

func TestResourcesReadAgentInstructionsMissing(t *testing.T) {
	t.Parallel()
	s := newTestServerWithVault(t)
	resp := callResourcesReadResponse(t, s, vaultAgentInstructionsResourceURI)
	if resp.Error == nil {
		t.Fatal("expected error for missing AGENTS.md resource")
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestResourcesReadSchema(t *testing.T) {
	t.Parallel()
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

func TestResourcesReadSchemaUsesVaultPathOverrideAgainstPinnedVault(t *testing.T) {
	t.Parallel()

	pinnedVault := t.TempDir()
	overrideVault := t.TempDir()
	if err := os.WriteFile(filepath.Join(pinnedVault, "schema.yaml"), []byte("types:\n  pinned:\n    default_path: pinned/\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write pinned schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overrideVault, "schema.yaml"), []byte("types:\n  override:\n    default_path: override/\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write override schema: %v", err)
	}

	s := &Server{vaultPath: pinnedVault}
	resp := callResourcesReadResponseWithParams(t, s, map[string]interface{}{
		"uri":        "raven://schema/current",
		"vault_path": overrideVault,
	})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %s", resp.Error.Message)
	}
	if len(resp.Result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(resp.Result.Contents))
	}
	text := resp.Result.Contents[0].Text
	if !strings.Contains(text, "override:") {
		t.Fatalf("expected override schema content, got %q", text)
	}
	if strings.Contains(text, "pinned:") {
		t.Fatalf("expected override schema to replace pinned schema, got %q", text)
	}
}

func TestResourcesReadSavedQueriesUsesNamedVaultOverrideAgainstPinnedVault(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pinnedVault := filepath.Join(tmp, "pinned")
	namedVault := filepath.Join(tmp, "named")
	for _, vaultPath := range []string{pinnedVault, namedVault} {
		if err := os.MkdirAll(vaultPath, 0o755); err != nil {
			t.Fatalf("mkdir vault %s: %v", vaultPath, err)
		}
	}
	if err := os.WriteFile(filepath.Join(pinnedVault, "raven.yaml"), []byte("queries:\n  pinned_query:\n    query: type:project\n"), 0o644); err != nil {
		t.Fatalf("write pinned raven.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(namedVault, "raven.yaml"), []byte("queries:\n  named_query:\n    query: type:person\n"), 0o644); err != nil {
		t.Fatalf("write named raven.yaml: %v", err)
	}

	configPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf("[vaults]\nwork = %q\n", namedVault)), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	s := &Server{
		vaultPath: pinnedVault,
		baseArgs:  []string{"--config", configPath, "--state", filepath.Join(tmp, "state.toml")},
	}
	resp := callResourcesReadResponseWithParams(t, s, map[string]interface{}{
		"uri":   "raven://queries/saved",
		"vault": "work",
	})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %s", resp.Error.Message)
	}
	if len(resp.Result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(resp.Result.Contents))
	}
	text := resp.Result.Contents[0].Text
	if !strings.Contains(text, `"named_query"`) {
		t.Fatalf("expected named vault queries, got %q", text)
	}
	if strings.Contains(text, `"pinned_query"`) {
		t.Fatalf("expected named vault override to replace pinned vault queries, got %q", text)
	}
}

func TestResourcesReadAgentInstructionsUsesVaultPathOverride(t *testing.T) {
	t.Parallel()

	pinnedVault := t.TempDir()
	overrideVault := t.TempDir()
	expected := "# Override Rules\nAlways verify the target vault.\n"
	if err := os.WriteFile(filepath.Join(pinnedVault, "schema.yaml"), []byte("types: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write pinned schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overrideVault, "schema.yaml"), []byte("types: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write override schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overrideVault, "AGENTS.md"), []byte(expected), 0o644); err != nil {
		t.Fatalf("write override AGENTS.md: %v", err)
	}

	s := &Server{vaultPath: pinnedVault}
	resp := callResourcesReadResponseWithParams(t, s, map[string]interface{}{
		"uri":        vaultAgentInstructionsResourceURI,
		"vault_path": overrideVault,
	})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %s", resp.Error.Message)
	}
	if len(resp.Result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(resp.Result.Contents))
	}
	if resp.Result.Contents[0].Text != expected {
		t.Fatalf("unexpected AGENTS.md content: got %q want %q", resp.Result.Contents[0].Text, expected)
	}
}

func TestResourcesReadRejectsVaultAndVaultPathTogether(t *testing.T) {
	t.Parallel()

	s := newTestServerWithVault(t)
	resp := callResourcesReadResponseWithParams(t, s, map[string]interface{}{
		"uri":        "raven://schema/current",
		"vault":      "work",
		"vault_path": s.vaultPath,
	})
	if resp.Error == nil {
		t.Fatal("expected invalid params error")
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("expected error code -32602, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid params" {
		t.Fatalf("error message = %q, want %q", resp.Error.Message, "Invalid params")
	}
	if data, ok := resp.Error.Data.(string); !ok || data != "vault and vault_path are mutually exclusive" {
		t.Fatalf("error data = %#v, want mutual exclusion message", resp.Error.Data)
	}
}

func TestResourcesReadUnknownGuide(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestStartupModeMessage(t *testing.T) {
	t.Parallel()
	t.Run("uses explicit vaultPath field", func(t *testing.T) {
		s := &Server{vaultPath: "/tmp/explicit"}
		msg := s.startupModeMessage()
		want := "[raven-mcp] Server starting with pinned vault: /tmp/explicit"
		if msg != want {
			t.Fatalf("startup message mismatch\ngot:  %q\nwant: %q", msg, want)
		}
	})

	t.Run("detects --vault-path value in base args", func(t *testing.T) {
		s := &Server{baseArgs: []string{"--vault-path", "/tmp/base"}}
		msg := s.startupModeMessage()
		want := "[raven-mcp] Server starting with pinned vault: /tmp/base"
		if msg != want {
			t.Fatalf("startup message mismatch\ngot:  %q\nwant: %q", msg, want)
		}
	})

	t.Run("detects --vault-path=value in base args", func(t *testing.T) {
		s := &Server{baseArgs: []string{"--vault-path=/tmp/inline"}}
		msg := s.startupModeMessage()
		want := "[raven-mcp] Server starting with pinned vault: /tmp/inline"
		if msg != want {
			t.Fatalf("startup message mismatch\ngot:  %q\nwant: %q", msg, want)
		}
	})

	t.Run("detects --vault name in base args", func(t *testing.T) {
		s := &Server{baseArgs: []string{"--vault", "work"}}
		msg := s.startupModeMessage()
		want := "[raven-mcp] Server starting with pinned named vault: work"
		if msg != want {
			t.Fatalf("startup message mismatch\ngot:  %q\nwant: %q", msg, want)
		}
	})

	t.Run("defaults to dynamic mode", func(t *testing.T) {
		s := &Server{}
		msg := s.startupModeMessage()
		want := "[raven-mcp] Server starting with dynamic vault resolution"
		if msg != want {
			t.Fatalf("startup message mismatch\ngot:  %q\nwant: %q", msg, want)
		}
	})
}

func TestExecuteRvnTreatsOkFalseAsErrorEvenWithExit0(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestExecuteRvnReturnsErrorWhenExecutablePathMissing(t *testing.T) {
	t.Parallel()
	s := &Server{executable: ""}
	out, isErr := s.executeRvn([]string{"stats", "--json"})
	if !isErr {
		t.Fatalf("expected isError=true, got false; out=%s", out)
	}

	var parsed struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
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
	if parsed.Error.Message == "" {
		t.Fatalf("expected error.message to be present; out=%s", out)
	}
}

func TestResolveVaultPathUsesExplicitVaultPathDirectly(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := &Server{baseArgs: []string{"--vault-path", tmp}}

	got, err := s.resolveVaultPath()
	if err != nil {
		t.Fatalf("resolveVaultPath returned error: %v", err)
	}
	if got != tmp {
		t.Fatalf("resolveVaultPath = %q, want %q", got, tmp)
	}
}

func TestResolveVaultPathUsesNamedVaultDirectly(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	vaultPath := filepath.Join(tmp, "work-vault")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	configPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`[vaults]
work = %q
`, vaultPath)), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := &Server{
		baseArgs: []string{"--config", configPath, "--state", filepath.Join(tmp, "state.toml"), "--vault", "work"},
	}

	got, err := s.resolveVaultPath()
	if err != nil {
		t.Fatalf("resolveVaultPath returned error: %v", err)
	}
	if got != vaultPath {
		t.Fatalf("resolveVaultPath = %q, want %q", got, vaultPath)
	}
}

func TestResolveVaultForInvocationVaultPathSource(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := &Server{}

	res, err := s.resolveVaultForInvocation("", tmp)
	if err != nil {
		t.Fatalf("resolveVaultForInvocation error: %v", err)
	}
	if res.path != tmp {
		t.Fatalf("path = %q, want %q", res.path, tmp)
	}
	if res.source != "vault_path" {
		t.Fatalf("source = %q, want %q", res.source, "vault_path")
	}
}

func TestResolveVaultForInvocationNamedVaultSource(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	vaultPath := filepath.Join(tmp, "work-vault")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}

	configPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`[vaults]
work = %q
`, vaultPath)), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := &Server{
		baseArgs: []string{"--config", configPath, "--state", filepath.Join(tmp, "state.toml")},
	}

	res, err := s.resolveVaultForInvocation("work", "")
	if err != nil {
		t.Fatalf("resolveVaultForInvocation error: %v", err)
	}
	if res.path != vaultPath {
		t.Fatalf("path = %q, want %q", res.path, vaultPath)
	}
	if res.source != "vault" {
		t.Fatalf("source = %q, want %q", res.source, "vault")
	}
	if res.name != "work" {
		t.Fatalf("name = %q, want %q", res.name, "work")
	}
}

func TestResolveVaultForInvocationPinnedSource(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := &Server{vaultPath: tmp}

	res, err := s.resolveVaultForInvocation("", "")
	if err != nil {
		t.Fatalf("resolveVaultForInvocation error: %v", err)
	}
	if res.path != tmp {
		t.Fatalf("path = %q, want %q", res.path, tmp)
	}
	if res.source != "pinned" {
		t.Fatalf("source = %q, want %q", res.source, "pinned")
	}
}

func TestResolveVaultForInvocationBaseArgsVaultPathSource(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := &Server{baseArgs: []string{"--vault-path", tmp}}

	res, err := s.resolveVaultForInvocation("", "")
	if err != nil {
		t.Fatalf("resolveVaultForInvocation error: %v", err)
	}
	if res.path != tmp {
		t.Fatalf("path = %q, want %q", res.path, tmp)
	}
	if res.source != "base_args" {
		t.Fatalf("source = %q, want %q", res.source, "base_args")
	}
}

func TestRunCancelsInFlightToolCallAndHandlesPing(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	cancelSeen := make(chan error, 1)
	var startedOnce sync.Once

	registry := commandexec.NewHandlerRegistry()
	registry.Register("reindex", func(ctx context.Context, _ commandexec.Request) commandexec.Result {
		startedOnce.Do(func() { close(started) })
		<-ctx.Done()
		cancelSeen <- ctx.Err()
		return commandexec.Failure("CANCELLED", ctx.Err().Error(), nil, "")
	})

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	server := &Server{
		vaultPath: t.TempDir(),
		in:        inReader,
		out:       outWriter,
		invoker:   commandexec.NewInvoker(registry, nil),
	}

	responses := make(chan rpcTestResponse, 8)
	readDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(outReader)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		for scanner.Scan() {
			var resp rpcTestResponse
			if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
				readDone <- err
				close(responses)
				return
			}
			responses <- resp
		}
		readDone <- scanner.Err()
		close(responses)
	}()

	runDone := make(chan error, 1)
	go func() {
		runDone <- server.Run()
		_ = outWriter.Close()
	}()

	writeRPCLine(t, inWriter, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "raven_invoke",
			"arguments": map[string]interface{}{
				"command": "reindex",
				"args": map[string]interface{}{
					"dry-run": true,
				},
			},
		},
	})

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool call to start")
	}

	writeRPCLine(t, inWriter, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "ping",
	})

	pingResp := waitForRPCResponse(t, responses)
	if got := rpcResponseIDAsInt(t, pingResp.ID); got != 2 {
		t.Fatalf("first response id = %d, want ping response id 2", got)
	}

	writeRPCLine(t, inWriter, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params": map[string]interface{}{
			"requestId": 1,
			"reason":    "test cancellation",
		},
	})

	select {
	case err := <-cancelSeen:
		if err == nil || err.Error() != "context canceled" {
			t.Fatalf("handler cancel err = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler cancellation")
	}

	toolResp := waitForRPCResponse(t, responses)
	if got := rpcResponseIDAsInt(t, toolResp.ID); got != 1 {
		t.Fatalf("tool response id = %d, want 1", got)
	}

	var toolResult ToolResult
	if err := json.Unmarshal(toolResp.Result, &toolResult); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if !toolResult.IsError {
		t.Fatalf("expected canceled tool result to be marked as error: %+v", toolResult)
	}
	if len(toolResult.Content) != 1 || !strings.Contains(toolResult.Content[0].Text, "context canceled") {
		t.Fatalf("expected canceled tool response text, got: %+v", toolResult.Content)
	}

	if err := inWriter.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("server run returned error: %v", err)
	}
	if err := <-readDone; err != nil {
		t.Fatalf("response reader returned error: %v", err)
	}
}

type rpcTestResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func writeRPCLine(t *testing.T, w *io.PipeWriter, payload map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal rpc payload: %v", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		t.Fatalf("write rpc payload: %v", err)
	}
}

func waitForRPCResponse(t *testing.T, responses <-chan rpcTestResponse) rpcTestResponse {
	t.Helper()
	select {
	case resp, ok := <-responses:
		if !ok {
			t.Fatal("response channel closed before receiving RPC response")
		}
		if resp.Error != nil {
			t.Fatalf("unexpected rpc error: %+v", resp.Error)
		}
		return resp
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RPC response")
		return rpcTestResponse{}
	}
}

func rpcResponseIDAsInt(t *testing.T, id interface{}) int {
	t.Helper()
	value, ok := id.(float64)
	if !ok {
		t.Fatalf("expected numeric rpc id, got %#v", id)
	}
	return int(value)
}
