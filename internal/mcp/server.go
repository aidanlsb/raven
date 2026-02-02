// Package mcp provides an MCP (Model Context Protocol) server for Raven.
// MCP enables LLM agents to interact with Raven through a standardized protocol.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
)

// Server is an MCP server that wraps Raven CLI commands.
type Server struct {
	vaultPath  string
	in         io.Reader
	out        io.Writer
	executable string // Path to the rvn executable
}

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      interface{}      `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  *json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ServerInfo contains server capability information.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities defines what the server can do.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

// ToolsCapability indicates tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates resource support.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the JSON schema for tool input.
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent represents content in a tool result.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewServer creates a new MCP server.
func NewServer(vaultPath string) *Server {
	// Get the path to the current executable so we can call it for tool execution
	executable, err := os.Executable()
	if err != nil {
		// Fall back to "rvn" and hope it's in PATH
		executable = "rvn"
	}

	return &Server{
		vaultPath:  vaultPath,
		in:         os.Stdin,
		out:        os.Stdout,
		executable: executable,
	}
}

// NewServerWithExecutable creates a new MCP server with a custom executable path.
// This is primarily used for testing with a built binary.
func NewServerWithExecutable(vaultPath, executable string) *Server {
	return &Server{
		vaultPath:  vaultPath,
		in:         os.Stdin,
		out:        os.Stdout,
		executable: executable,
	}
}

// SetIO sets the input and output streams for the server.
// This is primarily used for testing.
func (s *Server) SetIO(in io.Reader, out io.Writer) {
	s.in = in
	s.out = out
}

// HandleRequest processes a single MCP request.
// This is exported for testing purposes.
func (s *Server) HandleRequest(req *Request) {
	s.handleRequest(req)
}

// Run starts the MCP server's main loop.
func (s *Server) Run() error {
	scanner := bufio.NewScanner(s.in)
	// MCP uses line-delimited JSON
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	// Log startup to stderr (not stdout which is for protocol)
	fmt.Fprintln(os.Stderr, "[raven-mcp] Server starting for vault:", s.vaultPath)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Debug log incoming requests to stderr
		fmt.Fprintln(os.Stderr, "[raven-mcp] Received:", line)

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintln(os.Stderr, "[raven-mcp] Parse error:", err)
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "[raven-mcp] Scanner error:", err)
		return err
	}

	fmt.Fprintln(os.Stderr, "[raven-mcp] Server shutting down")
	return nil
}

func (s *Server) handleRequest(req *Request) {
	// Check if this is a notification (no ID means no response expected)
	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized", "notifications/initialized":
		// Client notification, no response needed
		return
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "resources/list":
		s.handleResourcesList(req)
	case "resources/read":
		s.handleResourcesRead(req)
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	case "notifications/cancelled":
		// Ignore cancellation notifications
		return
	default:
		// Only send error for requests, not notifications
		if !isNotification {
			s.sendError(req.ID, -32601, "Method not found", req.Method)
		}
	}
}

func (s *Server) handleInitialize(req *Request) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
		"serverInfo": ServerInfo{
			Name:    "raven-mcp",
			Version: "0.1.0",
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) {
	// Generate tools from the registry - single source of truth!
	tools := GenerateToolSchemas()
	s.sendResult(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(req *Request) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if req.Params != nil {
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	result, isError := s.callTool(params.Name, params.Arguments)
	s.sendResult(req.ID, ToolResult{
		Content: []ToolContent{{Type: "text", Text: result}},
		IsError: isError,
	})
}

// Resource represents an MCP resource
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent represents the content of a resource
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

func (s *Server) handleResourcesList(req *Request) {
	resources := append([]Resource{}, listAgentGuideResources()...)
	resources = append(resources, Resource{
		URI:         "raven://schema/current",
		Name:        "Current Schema",
		Description: "The current schema.yaml defining types and traits for this vault.",
		MimeType:    "text/yaml",
	})
	s.sendResult(req.ID, map[string]interface{}{"resources": resources})
}

func (s *Server) handleResourcesRead(req *Request) {
	var params struct {
		URI string `json:"uri"`
	}

	if req.Params != nil {
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	var content ResourceContent
	switch params.URI {
	case "raven://guide/index":
		indexContent, ok := getAgentGuideIndex()
		if !ok {
			s.sendError(req.ID, -32602, "Resource not found", params.URI)
			return
		}
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "text/markdown",
			Text:     indexContent,
		}
	case "raven://schema/current":
		schemaContent, err := s.readSchemaFile()
		if err != nil {
			s.sendError(req.ID, -32603, "Failed to read schema", err.Error())
			return
		}
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "text/yaml",
			Text:     schemaContent,
		}
	default:
		if strings.HasPrefix(params.URI, "raven://guide/") {
			slug := strings.TrimPrefix(params.URI, "raven://guide/")
			if slug == "" {
				s.sendError(req.ID, -32602, "Resource not found", params.URI)
				return
			}
			_, topicContent, ok := getAgentGuideTopic(slug)
			if !ok {
				s.sendError(req.ID, -32602, "Resource not found", params.URI)
				return
			}
			content = ResourceContent{
				URI:      params.URI,
				MimeType: "text/markdown",
				Text:     topicContent,
			}
			break
		}
		s.sendError(req.ID, -32602, "Resource not found", params.URI)
		return
	}

	s.sendResult(req.ID, map[string]interface{}{
		"contents": []ResourceContent{content},
	})
}

func (s *Server) readSchemaFile() (string, error) {
	schemaPath := paths.SchemaPath(s.vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Server) callTool(name string, args map[string]interface{}) (string, bool) {
	// Build CLI args from the registry - single source of truth!
	cmdArgs := BuildCLIArgs(name, args)

	if len(cmdArgs) == 0 {
		return fmt.Sprintf(`{"ok":false,"error":{"code":"UNKNOWN_TOOL","message":"Unknown tool: %s"}}`, name), true
	}

	// Execute the rvn command
	return s.executeRvn(cmdArgs)
}

func (s *Server) executeRvn(args []string) (string, bool) {
	// Add vault path
	args = append([]string{"--vault-path", s.vaultPath}, args...)

	// Use the executable path we determined at startup
	cmd := exec.Command(s.executable, args...)

	// Log to stderr for debugging
	fmt.Fprintf(os.Stderr, "[raven-mcp] Executing: %s %v\n", s.executable, args)

	output, err := cmd.CombinedOutput()

	if err != nil {
		fmt.Fprintf(os.Stderr, "[raven-mcp] Command error: %v, output: %s\n", err, string(output))

		// If the CLI returned structured JSON, pass it through unchanged.
		// Also treat ok:false as an error even if the process exit code was non-zero.
		type envelope struct {
			OK *bool `json:"ok"`
		}
		var env envelope
		if json.Unmarshal(output, &env) == nil && env.OK != nil {
			return string(output), true
		}

		// Otherwise, wrap the error but KEEP the CLI output so users can see what failed.
		wrapped := map[string]interface{}{
			"ok": false,
			"error": map[string]interface{}{
				"code":    "EXECUTION_ERROR",
				"message": err.Error(),
				"details": map[string]interface{}{
					"output": strings.TrimSpace(string(output)),
				},
			},
		}
		b, mErr := json.Marshal(wrapped)
		if mErr != nil {
			// Last resort: escape quotes
			errMsg := strings.ReplaceAll(err.Error(), `"`, `\"`)
			return fmt.Sprintf(`{"ok":false,"error":{"code":"EXECUTION_ERROR","message":"%s"}}`, errMsg), true
		}
		return string(b), true
	}

	fmt.Fprintf(os.Stderr, "[raven-mcp] Command succeeded, output length: %d\n", len(output))

	// If the CLI returned a standard Raven JSON envelope with ok:false, surface it as an MCP tool error.
	// This matters because some Raven commands intentionally exit 0 in --json mode to avoid Cobra printing,
	// and rely on the JSON envelope for error signaling.
	type envelope struct {
		OK *bool `json:"ok"`
	}
	var env envelope
	if json.Unmarshal(output, &env) == nil && env.OK != nil && !*env.OK {
		return string(output), true
	}

	return string(output), false
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.send(resp)
}

func (s *Server) sendError(id interface{}, code int, message, data string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.send(resp)
}

func (s *Server) send(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintln(s.out, string(data))
}
