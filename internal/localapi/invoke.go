package localapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

type invokeRequest struct {
	Command string                 `json:"command"`
	Args    map[string]interface{} `json:"args"`
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		writeResult(w, http.StatusBadRequest, commandexec.Failure(
			codes.ErrInvalidJSON,
			"failed to read request body",
			nil,
			"Retry the request",
		))
		return
	}
	if len(body) > maxRequestBytes {
		writeResult(w, http.StatusBadRequest, commandexec.Failure(
			codes.ErrInvalidInput,
			"request body exceeds 1MB",
			nil,
			"Reduce the payload and retry",
		))
		return
	}

	var req invokeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeResult(w, http.StatusBadRequest, commandexec.Failure(
			codes.ErrInvalidJSON,
			"request body must be JSON {command, args}",
			nil,
			"Send {\"command\":\"read\",\"args\":{...}}",
		))
		return
	}

	commandID := strings.TrimSpace(req.Command)
	if commandID == "" {
		writeResult(w, http.StatusBadRequest, commandexec.Failure(
			codes.ErrMissingArgument,
			"command is required",
			nil,
			"POST {\"command\":\"...\",\"args\":{}}",
		))
		return
	}

	args := req.Args
	if args == nil {
		args = map[string]interface{}{}
	}

	if !CommandAllowed(commandID) {
		writeResult(w, http.StatusOK, commandexec.Failure(
			codes.ErrCommandNotAllowed,
			"command is not available on the local API: "+commandID,
			map[string]interface{}{"command": commandID},
			"GET /v1/commands for the MVP allowlist; use the CLI or MCP for other commands",
		))
		return
	}

	result := s.execute(r.Context(), commandID, args)
	writeResult(w, http.StatusOK, result)
}

func (s *Server) execute(ctx context.Context, commandID string, args map[string]interface{}) commandexec.Result {
	vaultPath := ""
	if commands.RequiresVaultForInvocation(commandID, args) {
		vaultPath = s.vaultPath
	}

	req := commandexec.Request{
		CommandID:  commandID,
		VaultPath:  vaultPath,
		ConfigPath: s.configPath,
		Caller:     commandexec.CallerAPI,
		Args:       args,
	}

	result := s.executeWithIndexLockRetry(ctx, req)
	if vaultPath != "" {
		if result.Meta == nil {
			result.Meta = &commandexec.Meta{}
		}
		if result.Meta.VaultContext == nil {
			result.Meta.VaultContext = &commandexec.VaultContext{
				Path:   vaultPath,
				Source: "pinned",
			}
		}
	}
	return result
}
