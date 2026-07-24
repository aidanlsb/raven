//go:build integration

package mcp_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/mcp"
	"github.com/aidanlsb/raven/internal/testutil"
)

type mcpEnvelope struct {
	OK       bool                   `json:"ok"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    *mcpErrorEnvelope      `json:"error,omitempty"`
	Warnings []mcpWarningEnvelope   `json:"warnings,omitempty"`
}

type mcpErrorEnvelope struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Suggestion string                 `json:"suggestion,omitempty"`
}

type mcpWarningEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

func parseMCPEnvelope(t *testing.T, raw string) *mcpEnvelope {
	t.Helper()
	var env mcpEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("failed to parse MCP envelope: %v\nraw: %s", err, raw)
	}
	return &env
}

func assertEnvelopeParity(t *testing.T, mcpResult toolResult, cliResult *testutil.CLIResult, dataKeys []string) {
	t.Helper()

	env := parseMCPEnvelope(t, mcpResult.Text)

	if env.OK != cliResult.OK {
		t.Fatalf("ok mismatch: mcp=%v cli=%v\nmcp: %s\ncli: %s", env.OK, cliResult.OK, mcpResult.Text, cliResult.RawJSON)
	}
	if mcpResult.IsError != !env.OK {
		t.Fatalf("isError mismatch: isError=%v ok=%v\nmcp: %s", mcpResult.IsError, env.OK, mcpResult.Text)
	}

	if cliResult.Error == nil {
		if env.Error != nil {
			t.Fatalf("expected no error, got mcp error %+v", env.Error)
		}
	} else {
		if env.Error == nil {
			t.Fatalf("expected mcp error code %q, got nil\nmcp: %s\ncli: %s", cliResult.Error.Code, mcpResult.Text, cliResult.RawJSON)
		}
		if env.Error.Code != cliResult.Error.Code {
			t.Fatalf("error code mismatch: mcp=%q cli=%q\nmcp: %s\ncli: %s", env.Error.Code, cliResult.Error.Code, mcpResult.Text, cliResult.RawJSON)
		}
	}

	for _, key := range dataKeys {
		var mcpVal interface{}
		if env.Data != nil {
			mcpVal = env.Data[key]
		}
		var cliVal interface{}
		if cliResult.Data != nil {
			cliVal = cliResult.Data[key]
		}
		if key == "fetched_at" {
			mcpTS, mcpOK := mcpVal.(string)
			cliTS, cliOK := cliVal.(string)
			if mcpOK && cliOK && mcpTS != "" && cliTS != "" {
				if mcpParsed, err := time.Parse(time.RFC3339, mcpTS); err == nil {
					if cliParsed, err := time.Parse(time.RFC3339, cliTS); err == nil {
						diff := mcpParsed.Sub(cliParsed)
						if diff < 0 {
							diff = -diff
						}
						if diff <= 2*time.Second {
							continue
						}
					}
				}
			}
		}
		mcpVal = normalizeParityValue(mcpVal)
		cliVal = normalizeParityValue(cliVal)
		if !reflect.DeepEqual(mcpVal, cliVal) {
			t.Fatalf("data mismatch for key %q: mcp=%#v cli=%#v\nmcp: %s\ncli: %s", key, mcpVal, cliVal, mcpResult.Text, cliResult.RawJSON)
		}
	}

	mcpWarningCodes := make([]string, 0, len(env.Warnings))
	for _, warning := range env.Warnings {
		mcpWarningCodes = append(mcpWarningCodes, warning.Code)
	}
	cliWarningCodes := make([]string, 0, len(cliResult.Warnings))
	for _, warning := range cliResult.Warnings {
		cliWarningCodes = append(cliWarningCodes, warning.Code)
	}
	sort.Strings(mcpWarningCodes)
	sort.Strings(cliWarningCodes)
	if !reflect.DeepEqual(mcpWarningCodes, cliWarningCodes) {
		t.Fatalf("warning code mismatch: mcp=%v cli=%v\nmcp: %s\ncli: %s", mcpWarningCodes, cliWarningCodes, mcpResult.Text, cliResult.RawJSON)
	}
}

func normalizeParityValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			if key == "query_time_ms" {
				continue
			}
			out[key] = normalizeParityValue(value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, value := range typed {
			out[i] = normalizeParityValue(value)
		}
		return out
	default:
		return v
	}
}

func buildDocsArchiveBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q): %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func seedGlobalDocsConfig(t *testing.T, files map[string]string) string {
	t.Helper()
	globalDir := t.TempDir()
	configPath := filepath.Join(globalDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("# test config\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	for relPath, content := range files {
		fullPath := filepath.Join(globalDir, "docs", filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir docs path: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write docs file: %v", err)
		}
	}
	return configPath
}

func baseArgsForConfig(configPath string) []string {
	return []string{"--config", configPath}
}

func runCLIWithConfig(t *testing.T, binary, configPath string, args ...string) *testutil.CLIResult {
	t.Helper()
	cmdArgs := append(baseArgsForConfig(configPath), "--json")
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(binary, cmdArgs...)
	output, err := cmd.CombinedOutput()
	result := &testutil.CLIResult{RawJSON: string(output)}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	var resp struct {
		OK       bool                   `json:"ok"`
		Data     map[string]interface{} `json:"data,omitempty"`
		Error    *testutil.CLIError     `json:"error,omitempty"`
		Warnings []testutil.CLIWarning  `json:"warnings,omitempty"`
		Meta     *testutil.CLIMeta      `json:"meta,omitempty"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		result.OK = false
		result.Error = &testutil.CLIError{
			Code:    "PARSE_ERROR",
			Message: "Failed to parse JSON output: " + err.Error(),
			Details: map[string]interface{}{"raw": string(output)},
		}
		return result
	}
	result.OK = resp.OK
	result.Data = resp.Data
	result.Error = resp.Error
	result.Warnings = resp.Warnings
	result.Meta = resp.Meta
	return result
}

// testServer wraps the MCP Server for testing purposes.
type testServer struct {
	t          *testing.T
	vaultPath  string
	baseArgs   []string
	executable string
}

// toolResult represents the result of a tool call.
type toolResult struct {
	Text    string
	IsError bool
}

// newTestServer creates a test server with a custom executable path.
func newTestServer(t *testing.T, vaultPath, executable string) *testServer {
	return &testServer{
		t:          t,
		vaultPath:  vaultPath,
		executable: executable,
	}
}

func newTestServerWithBaseArgs(t *testing.T, baseArgs []string, executable string) *testServer {
	return &testServer{
		t:          t,
		baseArgs:   append([]string{}, baseArgs...),
		executable: executable,
	}
}

// callTool invokes a tool by simulating the MCP JSON-RPC protocol.
func (s *testServer) callTool(name string, args map[string]interface{}) toolResult {
	s.t.Helper()

	requestName := name
	requestArgs := args
	if name != "raven_discover" && name != "raven_describe" && name != "raven_invoke" {
		requestName = "raven_invoke"
		requestArgs = map[string]interface{}{"command": name}
		if args != nil {
			requestArgs["args"] = args
		}
	}

	// Create a real MCP server but with custom executable.
	var server *mcp.Server
	if len(s.baseArgs) > 0 {
		server = mcp.NewServerWithBaseArgsAndExecutable(s.baseArgs, s.executable)
	} else {
		server = mcp.NewServerWithExecutable(s.vaultPath, s.executable)
	}

	// Create a simulated JSON-RPC request
	paramsBytes, _ := json.Marshal(map[string]interface{}{
		"name":      requestName,
		"arguments": requestArgs,
	})
	params := json.RawMessage(paramsBytes)

	request := mcp.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  &params,
	}

	// Capture output
	var output bytes.Buffer
	server.SetIO(strings.NewReader(""), &output)

	// Handle the request directly
	server.HandleRequest(&request)

	// Parse the response
	var response struct {
		Result mcp.ToolResult `json:"result"`
	}
	if err := json.NewDecoder(&output).Decode(&response); err != nil {
		return toolResult{Text: "Failed to parse MCP response: " + err.Error(), IsError: true}
	}

	text := ""
	if len(response.Result.Content) > 0 {
		text = response.Result.Content[0].Text
	}

	return toolResult{
		Text:    text,
		IsError: response.Result.IsError,
	}
}

// Verify the integration test helpers compile correctly by importing from mcp package
var _ = mcp.GenerateToolSchemas

// testServerInterface is used to verify we're implementing the expected pattern.
type testServerInterface interface {
	callTool(name string, args map[string]interface{}) toolResult
}

var _ testServerInterface = (*testServer)(nil)
