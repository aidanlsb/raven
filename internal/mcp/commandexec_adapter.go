package mcp

import (
	"context"
	"encoding/json"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

func (s *Server) callCanonicalCommand(commandID string, args map[string]interface{}, vaultName, vaultPathOverride string) (string, bool, bool) {
	return s.callCanonicalCommandWithContext(context.Background(), commandID, args, vaultName, vaultPathOverride)
}

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
			return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve vault for invocation", err.Error(), nil), true, true
		}
		vaultPath = res.path
		vaultCtx = &commandexec.VaultContext{
			Name:   res.name,
			Path:   res.path,
			Source: res.source,
		}
	}

	args = normalizeCanonicalArgs(commandID, args)
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
	result = adaptCanonicalResultForMCP(commandID, result)

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
	b, err := json.Marshal(result)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to marshal command result", "", nil), true, true
	}
	return string(b), !result.OK, true
}

func normalizeCanonicalArgs(commandID string, args map[string]interface{}) map[string]interface{} {
	_ = commandID
	return args
}
