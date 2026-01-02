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
)

// Server is an MCP server that wraps Raven CLI commands.
type Server struct {
	vaultPath string
	in        io.Reader
	out       io.Writer
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

// Notification is a response without ID (for notifications).
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
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
	return &Server{
		vaultPath: vaultPath,
		in:        os.Stdin,
		out:       os.Stdout,
	}
}

// Run starts the MCP server's main loop.
func (s *Server) Run() error {
	scanner := bufio.NewScanner(s.in)
	// MCP uses line-delimited JSON
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// Client notification, no response needed
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	default:
		s.sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func (s *Server) handleInitialize(req *Request) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": ServerCapabilities{
			Tools: &ToolsCapability{},
		},
		"serverInfo": ServerInfo{
			Name:    "raven-mcp",
			Version: "0.1.0",
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) {
	tools := []Tool{
		{
			Name:        "raven_read",
			Description: "Read raw file content from the vault",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "File path relative to vault (e.g., daily/2025-02-01.md)",
					},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "raven_add",
			Description: "Append content to any file in the vault. Default: today's daily note.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to add (can include @traits and [[references]])",
					},
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Target file path (optional, defaults to daily note)",
					},
				},
				Required: []string{"text"},
			},
		},
		{
			Name:        "raven_trait",
			Description: "Query traits by type",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"trait_type": map[string]interface{}{
						"type":        "string",
						"description": "Trait type to query (e.g., due, status, priority)",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Optional value filter (e.g., today, past, this-week)",
					},
				},
				Required: []string{"trait_type"},
			},
		},
		{
			Name:        "raven_query",
			Description: "Run a saved query",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"query_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the saved query (e.g., tasks, overdue)",
					},
				},
				Required: []string{"query_name"},
			},
		},
		{
			Name:        "raven_type",
			Description: "List objects by type",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"type_name": map[string]interface{}{
						"type":        "string",
						"description": "Type name (e.g., person, project, meeting)",
					},
				},
				Required: []string{"type_name"},
			},
		},
		{
			Name:        "raven_backlinks",
			Description: "Find objects that reference a target",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"target": map[string]interface{}{
						"type":        "string",
						"description": "Target object ID (e.g., people/alice)",
					},
				},
				Required: []string{"target"},
			},
		},
		{
			Name:        "raven_date",
			Description: "Get all activity for a specific date",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"date": map[string]interface{}{
						"type":        "string",
						"description": "Date (today, yesterday, YYYY-MM-DD)",
					},
				},
				Required: []string{"date"},
			},
		},
		{
			Name:        "raven_tag",
			Description: "Query objects by tag",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"tag": map[string]interface{}{
						"type":        "string",
						"description": "Tag name (without #)",
					},
				},
				Required: []string{"tag"},
			},
		},
		{
			Name:        "raven_stats",
			Description: "Get vault statistics",
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name:        "raven_schema",
			Description: "Get schema information (types, traits, queries)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"subcommand": map[string]interface{}{
						"type":        "string",
						"description": "Optional: types, traits, commands, type <name>, trait <name>",
					},
				},
			},
		},
	}

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

func (s *Server) callTool(name string, args map[string]interface{}) (string, bool) {
	var cmdArgs []string

	switch name {
	case "raven_read":
		path, _ := args["path"].(string)
		cmdArgs = []string{"read", path, "--json"}

	case "raven_add":
		text, _ := args["text"].(string)
		cmdArgs = []string{"add", text, "--json"}
		if to, ok := args["to"].(string); ok && to != "" {
			cmdArgs = append(cmdArgs, "--to", to)
		}

	case "raven_trait":
		traitType, _ := args["trait_type"].(string)
		cmdArgs = []string{"trait", traitType, "--json"}
		if value, ok := args["value"].(string); ok && value != "" {
			cmdArgs = append(cmdArgs, "--value", value)
		}

	case "raven_query":
		queryName, _ := args["query_name"].(string)
		cmdArgs = []string{"query", queryName, "--json"}

	case "raven_type":
		typeName, _ := args["type_name"].(string)
		cmdArgs = []string{"type", typeName, "--json"}

	case "raven_backlinks":
		target, _ := args["target"].(string)
		cmdArgs = []string{"backlinks", target, "--json"}

	case "raven_date":
		date, _ := args["date"].(string)
		cmdArgs = []string{"date", date, "--json"}

	case "raven_tag":
		tag, _ := args["tag"].(string)
		cmdArgs = []string{"tag", tag, "--json"}

	case "raven_stats":
		cmdArgs = []string{"stats", "--json"}

	case "raven_schema":
		subcommand, _ := args["subcommand"].(string)
		if subcommand != "" {
			parts := strings.Fields(subcommand)
			cmdArgs = append([]string{"schema"}, parts...)
		} else {
			cmdArgs = []string{"schema"}
		}
		cmdArgs = append(cmdArgs, "--json")

	default:
		return fmt.Sprintf(`{"ok":false,"error":{"code":"UNKNOWN_TOOL","message":"Unknown tool: %s"}}`, name), true
	}

	// Execute the rvn command
	return s.executeRvn(cmdArgs)
}

func (s *Server) executeRvn(args []string) (string, bool) {
	// Add vault path
	args = append([]string{"--vault-path", s.vaultPath}, args...)

	cmd := exec.Command("rvn", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if output is valid JSON error from rvn
		var result map[string]interface{}
		if json.Unmarshal(output, &result) == nil {
			return string(output), true
		}
		// Otherwise, wrap the error
		return fmt.Sprintf(`{"ok":false,"error":{"code":"EXECUTION_ERROR","message":"%s"}}`, err.Error()), true
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
