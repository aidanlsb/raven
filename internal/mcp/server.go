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
			Name:        "raven_new",
			Description: "Create a new typed object (person, project, meeting, etc.). Use this to create new entries. If required fields are missing, an error will list them - ask the user for values and retry with the fields parameter.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Object type (e.g., person, project, meeting)",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Title/name for the new object (e.g., 'Alice Smith', 'Website Redesign')",
					},
					"fields": map[string]interface{}{
						"type":        "object",
						"description": "Optional field values as key-value pairs (e.g., {\"name\": \"Alice Smith\", \"email\": \"alice@example.com\"})",
					},
				},
				Required: []string{"type", "title"},
			},
		},
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
			Description: "Append content to EXISTING files or today's daily note. Only works on files that already exist (except daily notes which are auto-created). For creating new typed objects, use raven_new.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to add (can include @traits and [[references]])",
					},
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Path to EXISTING file (optional, defaults to daily note). File must exist.",
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
		{
			Name:        "raven_delete",
			Description: "Delete an object from the vault. By default moves to trash (configurable). Warns about backlinks.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"object_id": map[string]interface{}{
						"type":        "string",
						"description": "Object ID to delete (e.g., people/alice, projects/website)",
					},
				},
				Required: []string{"object_id"},
			},
		},
		{
			Name:        "raven_schema_add_type",
			Description: "Add a new type to the schema",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the new type",
					},
					"default_path": map[string]interface{}{
						"type":        "string",
						"description": "Default directory for files of this type (e.g., 'people/')",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "raven_schema_add_trait",
			Description: "Add a new trait to the schema",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the new trait",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Trait type: string, date, enum, bool (default: string)",
					},
					"values": map[string]interface{}{
						"type":        "string",
						"description": "For enum traits: comma-separated list of values",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "raven_schema_add_field",
			Description: "Add a new field to an existing type",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"type_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the type to add the field to",
					},
					"field_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the new field",
					},
					"field_type": map[string]interface{}{
						"type":        "string",
						"description": "Field type: string, date, enum, ref, bool (default: string)",
					},
					"required": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the field is required",
					},
					"target": map[string]interface{}{
						"type":        "string",
						"description": "For ref fields: target type name",
					},
				},
				Required: []string{"type_name", "field_name"},
			},
		},
		{
			Name:        "raven_schema_validate",
			Description: "Validate the schema for correctness",
			InputSchema: InputSchema{
				Type: "object",
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
	case "raven_new":
		objType, _ := args["type"].(string)
		title, _ := args["title"].(string)
		cmdArgs = []string{"new", objType, title}
		// Add field values if provided
		if fields, ok := args["fields"].(map[string]interface{}); ok {
			for key, value := range fields {
				cmdArgs = append(cmdArgs, "--field", fmt.Sprintf("%s=%v", key, value))
			}
		}
		cmdArgs = append(cmdArgs, "--json")

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

	case "raven_delete":
		objectID, _ := args["object_id"].(string)
		cmdArgs = []string{"delete", objectID, "--force", "--json"}

	case "raven_schema_add_type":
		name, _ := args["name"].(string)
		cmdArgs = []string{"schema", "add", "type", name}
		if defaultPath, ok := args["default_path"].(string); ok && defaultPath != "" {
			cmdArgs = append(cmdArgs, "--default-path", defaultPath)
		}
		cmdArgs = append(cmdArgs, "--json")

	case "raven_schema_add_trait":
		name, _ := args["name"].(string)
		cmdArgs = []string{"schema", "add", "trait", name}
		if traitType, ok := args["type"].(string); ok && traitType != "" {
			cmdArgs = append(cmdArgs, "--type", traitType)
		}
		if values, ok := args["values"].(string); ok && values != "" {
			cmdArgs = append(cmdArgs, "--values", values)
		}
		cmdArgs = append(cmdArgs, "--json")

	case "raven_schema_add_field":
		typeName, _ := args["type_name"].(string)
		fieldName, _ := args["field_name"].(string)
		cmdArgs = []string{"schema", "add", "field", typeName, fieldName}
		if fieldType, ok := args["field_type"].(string); ok && fieldType != "" {
			cmdArgs = append(cmdArgs, "--type", fieldType)
		}
		if required, ok := args["required"].(bool); ok && required {
			cmdArgs = append(cmdArgs, "--required")
		}
		if target, ok := args["target"].(string); ok && target != "" {
			cmdArgs = append(cmdArgs, "--target", target)
		}
		cmdArgs = append(cmdArgs, "--json")

	case "raven_schema_validate":
		cmdArgs = []string{"schema", "validate", "--json"}

	default:
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
		// Check if output is valid JSON error from rvn
		var result map[string]interface{}
		if json.Unmarshal(output, &result) == nil {
			return string(output), true
		}
		// Otherwise, wrap the error
		errMsg := strings.ReplaceAll(err.Error(), `"`, `\"`)
		return fmt.Sprintf(`{"ok":false,"error":{"code":"EXECUTION_ERROR","message":"%s"}}`, errMsg), true
	}

	fmt.Fprintf(os.Stderr, "[raven-mcp] Command succeeded, output length: %d\n", len(output))
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
