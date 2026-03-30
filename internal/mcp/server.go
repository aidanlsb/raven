// Package mcp provides an MCP (Model Context Protocol) server for Raven.
// MCP enables LLM agents to interact with Raven through a standardized protocol.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/rvnexec"
)

// Server is an MCP server that wraps Raven CLI commands.
type Server struct {
	vaultPath  string
	baseArgs   []string
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

func resolveExecutablePath() string {
	// Strict resolution: use only the current process binary path.
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if strings.TrimSpace(executable) == "" {
		return ""
	}
	return executable
}

// NewServer creates a new MCP server.
// If vaultPath is non-empty, it is pinned via --vault-path for all command execution.
func NewServer(vaultPath string) *Server {
	baseArgs := []string{}
	if strings.TrimSpace(vaultPath) != "" {
		baseArgs = append(baseArgs, "--vault-path", vaultPath)
	}

	return &Server{
		vaultPath:  vaultPath,
		baseArgs:   baseArgs,
		in:         os.Stdin,
		out:        os.Stdout,
		executable: resolveExecutablePath(),
	}
}

// NewServerWithBaseArgs creates a new MCP server using a set of base CLI flags.
// This is used by `rvn serve` for dynamic vault resolution with optional pass-through flags.
func NewServerWithBaseArgs(baseArgs []string) *Server {
	normalized := append([]string{}, baseArgs...)
	return &Server{
		baseArgs:   normalized,
		in:         os.Stdin,
		out:        os.Stdout,
		executable: resolveExecutablePath(),
	}
}

// NewServerWithExecutable creates a new MCP server with a custom executable path.
// This is primarily used for testing with a built binary.
func NewServerWithExecutable(vaultPath, executable string) *Server {
	baseArgs := []string{}
	if strings.TrimSpace(vaultPath) != "" {
		baseArgs = append(baseArgs, "--vault-path", vaultPath)
	}

	return &Server{
		vaultPath:  vaultPath,
		baseArgs:   baseArgs,
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
	fmt.Fprintln(os.Stderr, s.startupModeMessage())

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

func (s *Server) startupModeMessage() string {
	if vaultPath := strings.TrimSpace(s.vaultPath); vaultPath != "" {
		return fmt.Sprintf("[raven-mcp] Server starting with pinned vault: %s", vaultPath)
	}
	if vaultPath, ok := baseArgValue(s.baseArgs, "--vault-path"); ok {
		return fmt.Sprintf("[raven-mcp] Server starting with pinned vault: %s", vaultPath)
	}
	if vaultName, ok := baseArgValue(s.baseArgs, "--vault"); ok {
		return fmt.Sprintf("[raven-mcp] Server starting with pinned named vault: %s", vaultName)
	}
	return "[raven-mcp] Server starting with dynamic vault resolution"
}

func baseArgValue(args []string, flag string) (string, bool) {
	prefix := flag + "="
	var value string
	found := false

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == flag {
			if i+1 < len(args) {
				if next := strings.TrimSpace(args[i+1]); next != "" {
					value = next
					found = true
				}
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, prefix) {
			if inline := strings.TrimSpace(strings.TrimPrefix(arg, prefix)); inline != "" {
				value = inline
				found = true
			}
		}
	}

	return value, found
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

const vaultAgentInstructionsResourceURI = "raven://vault/agent-instructions"

func (s *Server) handleResourcesList(req *Request) {
	resources := append([]Resource{}, listAgentGuideResources()...)
	resources = append(resources, Resource{
		URI:         "raven://schema/current",
		Name:        "Current Schema",
		Description: "The current schema.yaml defining types and traits for this vault.",
		MimeType:    "text/yaml",
	})
	resources = append(resources, Resource{
		URI:         "raven://queries/saved",
		Name:        "Saved Queries",
		Description: "Saved queries defined in raven.yaml.",
		MimeType:    "application/json",
	})
	resources = append(resources, Resource{
		URI:         "raven://workflows/list",
		Name:        "Workflows",
		Description: "List of workflows defined in raven.yaml. Use raven://workflows/<name> for details.",
		MimeType:    "application/json",
	})
	if agentInstructions, ok := s.agentInstructionsResource(); ok {
		resources = append(resources, agentInstructions)
	}
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
	case "raven://queries/saved":
		queriesContent, err := s.readSavedQueriesResource()
		if err != nil {
			s.sendError(req.ID, -32603, "Failed to read saved queries", err.Error())
			return
		}
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "application/json",
			Text:     queriesContent,
		}
	case "raven://workflows/list":
		workflowsContent, err := s.readWorkflowsListResource()
		if err != nil {
			s.sendError(req.ID, -32603, "Failed to read workflows", err.Error())
			return
		}
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "application/json",
			Text:     workflowsContent,
		}
	case vaultAgentInstructionsResourceURI:
		agentInstructions, err := s.readAgentInstructionsResource()
		if err != nil {
			if os.IsNotExist(err) {
				s.sendError(req.ID, -32602, "Resource not found", params.URI)
				return
			}
			s.sendError(req.ID, -32603, "Failed to read agent instructions", err.Error())
			return
		}
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "text/markdown",
			Text:     agentInstructions,
		}
	default:
		if strings.HasPrefix(params.URI, "raven://workflows/") {
			name := strings.TrimPrefix(params.URI, "raven://workflows/")
			if name == "" {
				s.sendError(req.ID, -32602, "Resource not found", params.URI)
				return
			}
			workflowContent, err := s.readWorkflowResource(name)
			if err != nil {
				s.sendError(req.ID, -32603, "Failed to read workflow", err.Error())
				return
			}
			content = ResourceContent{
				URI:      params.URI,
				MimeType: "application/json",
				Text:     workflowContent,
			}
			break
		}
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

func (s *Server) agentInstructionsResource() (Resource, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return Resource{}, false
	}

	agentInstructionsPath := paths.AgentInstructionsPath(vaultPath)
	info, err := os.Stat(agentInstructionsPath)
	if err != nil || info.IsDir() {
		return Resource{}, false
	}

	return Resource{
		URI:         vaultAgentInstructionsResourceURI,
		Name:        "Agent Instructions",
		Description: "Agent guidance from AGENTS.md in the vault root.",
		MimeType:    "text/markdown",
	}, true
}

func (s *Server) readAgentInstructionsResource() (string, error) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(paths.AgentInstructionsPath(vaultPath))
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (s *Server) readSchemaFile() (string, error) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return "", err
	}

	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Server) callTool(name string, args map[string]interface{}) (string, bool) {
	if out, isErr, handled := s.callCompactTool(name, args); handled {
		return out, isErr
	}

	return errorEnvelope(
		"UNKNOWN_TOOL",
		fmt.Sprintf("unknown tool: %s", name),
		fmt.Sprintf("Call %s to list available tools", compactToolDiscover),
		map[string]interface{}{"tool": name},
	), true
}

func (s *Server) executeRvn(args []string) (string, bool) {
	if strings.TrimSpace(s.executable) == "" {
		wrapped := map[string]interface{}{
			"ok": false,
			"error": map[string]interface{}{
				"code":    "EXECUTION_ERROR",
				"message": "failed to resolve current executable path",
			},
		}
		b, _ := json.Marshal(wrapped)
		return string(b), true
	}

	args = s.withBaseArgs(args)

	// Log to stderr for debugging
	fmt.Fprintf(os.Stderr, "[raven-mcp] Executing: %s %v\n", s.executable, args)

	result, err := rvnexec.Run(s.executable, args)

	if err != nil {
		fmt.Fprintf(os.Stderr, "[raven-mcp] Command error: %v, output: %s\n", err, result.OutputString())

		// If the CLI returned structured JSON, pass it through unchanged.
		if result.HasEnvelope && result.OK != nil {
			return result.OutputString(), true
		}

		// Otherwise, wrap the error but KEEP the CLI output so users can see what failed.
		wrapped := map[string]interface{}{
			"ok": false,
			"error": map[string]interface{}{
				"code":    "EXECUTION_ERROR",
				"message": err.Error(),
				"details": map[string]interface{}{
					"output": result.TrimmedOutput(),
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

	fmt.Fprintf(os.Stderr, "[raven-mcp] Command succeeded, output length: %d\n", len(result.Output))

	// If the CLI returned a standard Raven JSON envelope with ok:false, surface it as an MCP tool error.
	// This matters because some Raven commands intentionally exit 0 in --json mode to avoid Cobra printing,
	// and rely on the JSON envelope for error signaling.
	if result.OK != nil && !*result.OK {
		return result.OutputString(), true
	}

	return result.OutputString(), false
}

func (s *Server) withBaseArgs(args []string) []string {
	out := make([]string, 0, len(s.baseArgs)+len(args))
	out = append(out, s.baseArgs...)
	out = append(out, args...)
	return out
}

func (s *Server) resolveVaultPath() (string, error) {
	return s.resolveVaultPathForInvocation("", "")
}

func (s *Server) resolveVaultPathForInvocation(vaultName, vaultPath string) (string, error) {
	if resolvedVaultPath := strings.TrimSpace(vaultPath); resolvedVaultPath != "" {
		return s.validateResolvedVaultPath(resolvedVaultPath)
	}
	if resolvedVaultName := strings.TrimSpace(vaultName); resolvedVaultName != "" {
		return s.namedVaultPath(resolvedVaultName)
	}
	if vaultPath := strings.TrimSpace(s.vaultPath); vaultPath != "" {
		return s.validateResolvedVaultPath(vaultPath)
	}
	if vaultPath, ok := baseArgValue(s.baseArgs, "--vault-path"); ok {
		return s.validateResolvedVaultPath(vaultPath)
	}
	if vaultName, ok := baseArgValue(s.baseArgs, "--vault"); ok {
		return s.namedVaultPath(vaultName)
	}
	return s.currentVaultPath()
}

func (s *Server) currentVaultPath() (string, error) {
	result, err := configsvc.CurrentVault(s.directConfigContextOptions())
	if err != nil {
		return "", err
	}
	return s.validateResolvedVaultPath(result.Current.Path)
}

func (s *Server) namedVaultPath(name string) (string, error) {
	ctx, err := configsvc.LoadVaultContext(s.directConfigContextOptions())
	if err != nil {
		return "", err
	}
	resolved, err := ctx.Cfg.GetVaultPath(strings.TrimSpace(name))
	if err != nil {
		return "", err
	}
	return s.validateResolvedVaultPath(resolved)
}

func (s *Server) validateResolvedVaultPath(vaultPath string) (string, error) {
	resolved := strings.TrimSpace(vaultPath)
	if resolved == "" {
		return "", fmt.Errorf("failed to resolve current vault: empty path")
	}
	if _, err := os.Stat(resolved); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("vault not found: %s", resolved)
		}
		return "", fmt.Errorf("failed to resolve current vault: %w", err)
	}
	return resolved, nil
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
		fmt.Fprintf(os.Stderr, "mcp: failed to marshal JSON-RPC response: %v\n", err)
		fmt.Fprintln(s.out, fallbackRPCResponseJSON(v, err))
		return
	}
	fmt.Fprintln(s.out, string(data))
}

func fallbackRPCResponseJSON(v interface{}, marshalErr error) string {
	idJSON := "null"
	if resp, ok := v.(Response); ok {
		if encoded, ok := encodeFallbackResponseID(resp.ID); ok {
			idJSON = encoded
		}
	}

	return `{"jsonrpc":"2.0","id":` + idJSON + `,"error":{"code":-32603,"message":"failed to marshal response","data":` + strconv.Quote(marshalErr.Error()) + `}}`
}

func encodeFallbackResponseID(id interface{}) (string, bool) {
	switch v := id.(type) {
	case nil:
		return "null", true
	case string:
		return strconv.Quote(v), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case int:
		return strconv.Itoa(v), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
}
