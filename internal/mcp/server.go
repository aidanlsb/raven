// Package mcp provides an MCP (Model Context Protocol) server for Raven.
// MCP enables LLM agents to interact with Raven through a standardized protocol.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

// Server is an MCP server that wraps Raven CLI commands.
type Server struct {
	vaultPath   string
	baseArgs    []string
	in          io.Reader
	out         io.Writer
	executable  string // Path to the rvn executable
	strictVault bool   // Require an explicit vault for vault-scoped operations
	invoker     *commandexec.Invoker

	sendMu     sync.Mutex
	inFlightMu sync.Mutex
	inFlight   map[string]context.CancelFunc
	runWG      sync.WaitGroup
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

// NewServerWithBaseArgsAndExecutable creates a new MCP server using base CLI flags and a custom executable path.
// This is primarily used for integration tests that need both config context and built-binary version metadata.
func NewServerWithBaseArgsAndExecutable(baseArgs []string, executable string) *Server {
	normalized := append([]string{}, baseArgs...)
	return &Server{
		baseArgs:   normalized,
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

// SetStrictVault toggles strict vault mode. In strict mode, vault-scoped
// operations that lack an explicit vault (per-call vault/vault_path or a
// server-pinned vault) fail with VAULT_AMBIGUOUS instead of falling back to the
// ambient active/default vault.
func (s *Server) SetStrictVault(strict bool) {
	s.strictVault = strict
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

		if req.Method == "tools/call" {
			s.dispatchToolCall(&req)
			continue
		}
		s.handleRequest(&req)
	}

	s.runWG.Wait()

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "[raven-mcp] Scanner error:", err)
		return err
	}

	fmt.Fprintln(os.Stderr, "[raven-mcp] Server shutting down")
	return nil
}

func (s *Server) startupModeMessage() string {
	base := s.startupVaultModeMessage()
	if s.strictVault {
		return base + " (strict vault mode)"
	}
	return base
}

func (s *Server) startupVaultModeMessage() string {
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
		s.handleToolsCall(context.Background(), req)
	case "resources/list":
		s.handleResourcesList(req)
	case "resources/read":
		s.handleResourcesRead(req)
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	case "notifications/cancelled":
		s.handleCancelledNotification(req)
		return
	default:
		// Only send error for requests, not notifications
		if !isNotification {
			s.sendError(req.ID, -32601, "Method not found", req.Method)
		}
	}
}

func (s *Server) handleInitialize(req *Request) {
	version := maintsvc.CurrentVersionInfoFromExecutable(s.executable).Version
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
		"serverInfo": ServerInfo{
			Name:    "raven-mcp",
			Version: version,
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) {
	// Generate tools from the registry - single source of truth!
	tools := GenerateToolSchemas()
	s.sendResult(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) {
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

	result, isError := s.callToolWithContext(ctx, params.Name, params.Arguments)
	s.sendResult(req.ID, ToolResult{
		Content: []ToolContent{{Type: "text", Text: result}},
		IsError: isError,
	})
}

func (s *Server) dispatchToolCall(req *Request) {
	ctx, cancel := context.WithCancel(context.Background())

	requestKey, tracked := requestIDKey(req.ID)
	if tracked {
		s.trackInFlight(requestKey, cancel)
	}

	s.runWG.Add(1)
	go func() {
		defer s.runWG.Done()
		defer cancel()
		if tracked {
			defer s.untrackInFlight(requestKey)
		}
		s.handleToolsCall(ctx, req)
	}()
}

func (s *Server) handleCancelledNotification(req *Request) {
	var params struct {
		RequestID interface{} `json:"requestId"`
		ID        interface{} `json:"id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return
		}
	}

	targetID := params.RequestID
	if targetID == nil {
		targetID = params.ID
	}
	requestKey, ok := requestIDKey(targetID)
	if !ok {
		return
	}
	s.cancelInFlight(requestKey)
}

func (s *Server) trackInFlight(requestKey string, cancel context.CancelFunc) {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	if s.inFlight == nil {
		s.inFlight = make(map[string]context.CancelFunc)
	}
	s.inFlight[requestKey] = cancel
}

func (s *Server) untrackInFlight(requestKey string) {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	if s.inFlight == nil {
		return
	}
	delete(s.inFlight, requestKey)
}

func (s *Server) cancelInFlight(requestKey string) {
	s.inFlightMu.Lock()
	cancel := s.inFlight[requestKey]
	s.inFlightMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func requestIDKey(id interface{}) (string, bool) {
	if id == nil {
		return "", false
	}
	encoded, err := json.Marshal(id)
	if err != nil {
		return "", false
	}
	return string(encoded), true
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

type resourceReadParams struct {
	URI       string `json:"uri"`
	Vault     string `json:"vault,omitempty"`
	VaultPath string `json:"vault_path,omitempty"`
}

func (p resourceReadParams) validate() error {
	if strings.TrimSpace(p.Vault) != "" && strings.TrimSpace(p.VaultPath) != "" {
		return fmt.Errorf("vault and vault_path are mutually exclusive")
	}
	return nil
}

func (s *Server) handleResourcesList(req *Request) {
	var params resourceReadParams
	if req.Params != nil {
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}
	if err := params.validate(); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

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

	// Resolve the vault the list reflects so callers can see which vault the
	// vault-scoped resources map to. This mirrors resources/read, which accepts
	// the same vault/vault_path params. Resolution is best-effort: when it fails
	// (e.g. strict mode without an explicit vault), the vault-independent guide
	// resources are still returned.
	res, resErr := s.resolveVaultForInvocation(params.Vault, params.VaultPath)
	if resErr == nil {
		if agentInstructions, ok := s.agentInstructionsResourceAt(res.path); ok {
			resources = append(resources, agentInstructions)
		}
	}

	result := map[string]interface{}{"resources": resources}
	if resErr == nil {
		result["vault_context"] = vaultContextFromResolution(res)
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleResourcesRead(req *Request) {
	var params resourceReadParams

	if req.Params != nil {
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}
	if err := params.validate(); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	var content ResourceContent
	var vaultCtx *commandexec.VaultContext
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
		res, err := s.resolveVaultForInvocation(params.Vault, params.VaultPath)
		if err != nil {
			s.sendResourceVaultError(req.ID, "Failed to read schema", err)
			return
		}
		schemaContent, exists, err := schema.ReadRawSchema(res.path)
		if err != nil {
			s.sendError(req.ID, -32603, "Failed to read schema", err.Error())
			return
		}
		if !exists {
			s.sendError(req.ID, -32602, "Resource not found", params.URI)
			return
		}
		vaultCtx = vaultContextFromResolution(res)
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "text/yaml",
			Text:     schemaContent,
		}
	case "raven://queries/saved":
		res, err := s.resolveVaultForInvocation(params.Vault, params.VaultPath)
		if err != nil {
			s.sendResourceVaultError(req.ID, "Failed to read saved queries", err)
			return
		}
		queriesContent, err := s.readSavedQueriesResourceAt(res.path)
		if err != nil {
			s.sendError(req.ID, -32603, "Failed to read saved queries", err.Error())
			return
		}
		vaultCtx = vaultContextFromResolution(res)
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "application/json",
			Text:     queriesContent,
		}
	case vaultAgentInstructionsResourceURI:
		res, err := s.resolveVaultForInvocation(params.Vault, params.VaultPath)
		if err != nil {
			s.sendResourceVaultError(req.ID, "Failed to read agent instructions", err)
			return
		}
		agentInstructions, err := s.readAgentInstructionsResourceAt(res.path)
		if err != nil {
			if os.IsNotExist(err) {
				s.sendError(req.ID, -32602, "Resource not found", params.URI)
				return
			}
			s.sendError(req.ID, -32603, "Failed to read agent instructions", err.Error())
			return
		}
		vaultCtx = vaultContextFromResolution(res)
		content = ResourceContent{
			URI:      params.URI,
			MimeType: "text/markdown",
			Text:     agentInstructions,
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

	result := map[string]interface{}{
		"contents": []ResourceContent{content},
	}
	if vaultCtx != nil {
		result["vault_context"] = vaultCtx
	}
	s.sendResult(req.ID, result)
}

// vaultContextFromResolution converts an internal vaultResolution into the
// transport-neutral VaultContext used across Raven responses.
func vaultContextFromResolution(res vaultResolution) *commandexec.VaultContext {
	return &commandexec.VaultContext{
		Name:   res.name,
		Path:   res.path,
		Source: res.source,
	}
}

// sendResourceVaultError surfaces a vault resolution failure for a resource
// read. When the error carries a stable Raven code (e.g. VAULT_AMBIGUOUS from
// strict mode), the code is included in the RPC error data.
func (s *Server) sendResourceVaultError(id interface{}, context string, err error) {
	var vErr *vaultResolutionError
	if errors.As(err, &vErr) {
		resp := Response{
			JSONRPC: "2.0",
			ID:      id,
			Error: &RPCError{
				Code:    -32602,
				Message: context,
				Data: map[string]interface{}{
					"code":       vErr.code,
					"message":    vErr.message,
					"suggestion": vErr.suggestion,
				},
			},
		}
		s.send(resp)
		return
	}
	s.sendError(id, -32603, context, err.Error())
}

func (s *Server) agentInstructionsResourceAt(vaultPath string) (Resource, bool) {
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

func (s *Server) readAgentInstructionsResourceAt(vaultPath string) (string, error) {
	data, err := os.ReadFile(paths.AgentInstructionsPath(vaultPath))
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (s *Server) callToolWithContext(ctx context.Context, name string, args map[string]interface{}) (string, bool) {
	if out, isErr, handled := s.callCompactToolWithContext(ctx, name, args); handled {
		return out, isErr
	}

	return errorEnvelope(
		"UNKNOWN_TOOL",
		fmt.Sprintf("unknown tool: %s", name),
		fmt.Sprintf("Call %s to list available tools", compactToolDiscover),
		map[string]interface{}{"tool": name},
	), true
}

func (s *Server) resolveVaultPath() (string, error) {
	return s.resolveVaultPathForInvocation("", "")
}

func (s *Server) resolveVaultPathForInvocation(vaultName, vaultPath string) (string, error) {
	res, err := s.resolveVaultForInvocation(vaultName, vaultPath)
	if err != nil {
		return "", err
	}
	return res.path, nil
}

// vaultResolution captures how a vault was resolved.
type vaultResolution struct {
	path   string
	source string
	name   string
}

// vaultResolutionError carries a stable Raven error code for vault resolution
// failures so callers can surface it in the JSON envelope instead of a generic
// resolution error.
type vaultResolutionError struct {
	code       string
	message    string
	suggestion string
}

func (e *vaultResolutionError) Error() string {
	return e.message
}

// ambientVaultSources are the resolution sources that come from mutable global
// state rather than an explicit per-call or server-pinned vault. Resolving via
// one of these is the "silent wrong-vault" risk that strict mode blocks.
var ambientVaultSources = map[string]struct{}{
	"active_vault":           {},
	"default_vault":          {},
	"default_vault_fallback": {},
}

func isAmbientVaultSource(source string) bool {
	_, ok := ambientVaultSources[source]
	return ok
}

func (s *Server) resolveVaultForInvocation(vaultName, vaultPath string) (vaultResolution, error) {
	if resolved := strings.TrimSpace(vaultPath); resolved != "" {
		p, err := s.validateResolvedVaultPath(resolved)
		if err != nil {
			return vaultResolution{}, err
		}
		name := s.lookupVaultName(p)
		return vaultResolution{path: p, source: "vault_path", name: name}, nil
	}
	if resolved := strings.TrimSpace(vaultName); resolved != "" {
		p, err := s.namedVaultPath(resolved)
		if err != nil {
			return vaultResolution{}, err
		}
		return vaultResolution{path: p, source: "vault", name: resolved}, nil
	}
	if pinned := strings.TrimSpace(s.vaultPath); pinned != "" {
		p, err := s.validateResolvedVaultPath(pinned)
		if err != nil {
			return vaultResolution{}, err
		}
		name := s.lookupVaultName(p)
		return vaultResolution{path: p, source: "pinned", name: name}, nil
	}
	if vp, ok := baseArgValue(s.baseArgs, "--vault-path"); ok {
		p, err := s.validateResolvedVaultPath(vp)
		if err != nil {
			return vaultResolution{}, err
		}
		name := s.lookupVaultName(p)
		return vaultResolution{path: p, source: "base_args", name: name}, nil
	}
	if vn, ok := baseArgValue(s.baseArgs, "--vault"); ok {
		p, err := s.namedVaultPath(vn)
		if err != nil {
			return vaultResolution{}, err
		}
		return vaultResolution{path: p, source: "base_args", name: vn}, nil
	}
	// No explicit per-call or server-pinned vault. Any resolution from here uses
	// ambient global state (active/default vault). In strict mode we refuse to
	// guess to prevent silent wrong-vault operations.
	if s.strictVault {
		return vaultResolution{}, &vaultResolutionError{
			code:       string(codes.ErrVaultAmbiguous),
			message:    "strict vault mode requires an explicit vault; pass vault or vault_path (or pin one when starting the server)",
			suggestion: "Pass vault (a configured vault name) or vault_path (an absolute vault directory) with this call.",
		}
	}
	return s.currentVaultResolution()
}

func (s *Server) currentVaultResolution() (vaultResolution, error) {
	result, err := configsvc.CurrentVault(s.directConfigContextOptions())
	if err != nil {
		return vaultResolution{}, err
	}
	p, err := s.validateResolvedVaultPath(result.Current.Path)
	if err != nil {
		return vaultResolution{}, err
	}
	return vaultResolution{
		path:   p,
		source: result.Current.Source,
		name:   result.Current.Name,
	}, nil
}

// configuredVaultCount returns the number of vaults configured for this server.
// It is best-effort: on any load error it returns 0.
func (s *Server) configuredVaultCount() int {
	ctx, err := configsvc.LoadVaultContext(s.directConfigContextOptions())
	if err != nil {
		return 0
	}
	return len(ctx.Cfg.ListVaults())
}

// lookupVaultName attempts a best-effort reverse lookup of vault name from path.
func (s *Server) lookupVaultName(path string) string {
	ctx, err := configsvc.LoadVaultContext(s.directConfigContextOptions())
	if err != nil {
		return ""
	}
	for name, vp := range ctx.Cfg.ListVaults() {
		if vp == path {
			return name
		}
	}
	return ""
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
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

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
