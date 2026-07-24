//go:build integration

package mcp_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/mcp"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// TestMCPIntegration_ToolsList tests that the MCP server returns tool schemas.
func TestMCPIntegration_ToolsList(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := mcp.NewServerWithExecutable(v.Path, binary)

	request := mcp.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	var output bytes.Buffer
	server.SetIO(strings.NewReader(""), &output)
	server.HandleRequest(&request)

	var response struct {
		Result struct {
			Tools []mcp.Tool `json:"tools"`
		} `json:"result"`
		Error *mcp.RPCError `json:"error,omitempty"`
	}
	if err := json.NewDecoder(&output).Decode(&response); err != nil {
		t.Fatalf("failed to parse tools/list response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tools/list returned error: %s", response.Error.Message)
	}

	tools := response.Result.Tools
	if len(tools) != 3 {
		t.Fatalf("expected 3 compact tools, got %d", len(tools))
	}

	// Verify compact tools exist
	expectedTools := []string{"raven_discover", "raven_describe", "raven_invoke"}
	foundTools := make(map[string]bool)
	for _, tool := range tools {
		foundTools[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !foundTools[expected] {
			t.Errorf("expected tool %s not found in tool list", expected)
		}
	}
}

// TestMCPIntegration_ServeRejectsLegacyToolNames ensures the live `rvn serve`
// JSON-RPC path only accepts compact tools and rejects legacy names.
func TestMCPIntegration_ServeRejectsLegacyToolNames(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	binary := testutil.BuildCLI(t)

	requests := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"raven_new","arguments":{"type":"page","title":"Legacy Path"}}}`,
	}, "\n") + "\n"

	cmd := exec.Command(binary, "--vault-path", v.Path, "serve")
	cmd.Stdin = strings.NewReader(requests)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("serve command failed: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 JSON-RPC responses, got %d\nstdout: %s", len(lines), stdout.String())
	}

	var initResp struct {
		Result map[string]interface{} `json:"result"`
		Error  *mcp.RPCError          `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed to parse initialize response: %v\nraw: %s", err, lines[0])
	}
	if initResp.Error != nil {
		t.Fatalf("initialize returned rpc error: %+v", initResp.Error)
	}
	serverInfo, ok := initResp.Result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("initialize missing serverInfo: %#v", initResp.Result)
	}
	version, _ := serverInfo["version"].(string)
	wantVersion := versioninfo.CurrentVersionInfoFromExecutable(binary).Version
	if version != wantVersion {
		t.Fatalf("initialize serverInfo.version=%q, want %q", version, wantVersion)
	}

	var toolCallResp struct {
		Result mcp.ToolResult `json:"result"`
		Error  *mcp.RPCError  `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &toolCallResp); err != nil {
		t.Fatalf("failed to parse tools/call response: %v\nraw: %s", err, lines[1])
	}
	if toolCallResp.Error != nil {
		t.Fatalf("tools/call returned rpc error: %+v", toolCallResp.Error)
	}
	if !toolCallResp.Result.IsError {
		t.Fatalf("expected tools/call isError=true for legacy tool name\nresponse: %s", lines[1])
	}
	if len(toolCallResp.Result.Content) == 0 {
		t.Fatalf("expected tool response content, got none: %s", lines[1])
	}

	env := parseMCPEnvelope(t, toolCallResp.Result.Content[0].Text)
	if env.Error == nil || env.Error.Code != "UNKNOWN_TOOL" {
		t.Fatalf("expected UNKNOWN_TOOL envelope error, got: %s", toolCallResp.Result.Content[0].Text)
	}
	if env.Error.Suggestion != "Call raven_discover to list available tools" {
		t.Fatalf("unexpected UNKNOWN_TOOL suggestion: %q", env.Error.Suggestion)
	}
}

// TestMCPIntegration_CreateObject tests creating an object via MCP tool call.
