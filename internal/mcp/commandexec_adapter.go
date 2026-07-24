package mcp

import (
	"context"
	"errors"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

func (s *Server) callCanonicalCommandWithContext(ctx context.Context, commandID string, args map[string]interface{}, vaultName, vaultPathOverride string) (string, bool, bool) {
	invoker := s.commandInvoker()
	if invoker == nil {
		return "", false, false
	}
	if _, ok := invoker.Handlers().Lookup(commandID); !ok {
		return "", false, false
	}

	var vaultCtx *commandexec.VaultContext
	vaultPath := ""
	if commands.RequiresVault(commandID) {
		res, err := s.resolveVaultForInvocation(vaultName, vaultPathOverride)
		if err != nil {
			var vErr *vaultResolutionError
			if errors.As(err, &vErr) {
				return errorEnvelope(vErr.code, vErr.message, vErr.suggestion, nil), true, true
			}
			return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve vault for invocation", err.Error(), nil), true, true
		}
		vaultPath = res.path
		vaultCtx = &commandexec.VaultContext{
			Name:   res.name,
			Path:   res.path,
			Source: res.source,
		}
	}

	configOpts := s.directConfigContextOptions()

	result := invoker.Execute(ctx, commandexec.Request{
		CommandID:      commandID,
		VaultPath:      vaultPath,
		ConfigPath:     configOpts.ConfigPathOverride,
		StatePath:      configOpts.StatePathOverride,
		ExecutablePath: s.executable,
		Caller:         commandexec.CallerMCP,
		Args:           args,
	})
	if commandID == "vault_focus" && result.OK {
		if err := s.applyVaultFocusResult(result); err != nil {
			result = commandexec.Failure("INTERNAL_ERROR", "failed to update MCP session vault focus", nil, err.Error())
		}
	}
	result = adaptCanonicalResultForMCP(commandID, args, result)

	if vaultCtx != nil {
		if result.Meta == nil {
			result.Meta = &commandexec.Meta{}
		}
		result.Meta.VaultContext = vaultCtx
	}

	return marshalCanonicalResult(result)
}

func (s *Server) commandInvoker() *commandexec.Invoker {
	if s.invoker != nil {
		return s.invoker
	}
	return app.CommandInvoker()
}

func marshalCanonicalResult(result commandexec.Result) (string, bool, bool) {
	out, isErr := marshalResultEnvelope(result)
	return out, isErr, true
}
