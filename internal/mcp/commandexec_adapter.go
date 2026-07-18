package mcp

import (
	"context"
	"errors"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/codes"
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
	var fallbackWarning *commandexec.Warning
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
		fallbackWarning = s.vaultFallbackWarning(commandID, res)
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
	result = adaptCanonicalResultForMCP(commandID, args, result)

	if vaultCtx != nil {
		if result.Meta == nil {
			result.Meta = &commandexec.Meta{}
		}
		result.Meta.VaultContext = vaultCtx
	}
	if fallbackWarning != nil {
		result.Warnings = append(result.Warnings, *fallbackWarning)
	}

	return marshalCanonicalResult(result)
}

// vaultFallbackWarning returns a VAULT_FALLBACK warning when a write command
// resolved its vault from ambient global state (active/default vault) while more
// than one vault is configured. This surfaces the silent wrong-vault risk in the
// default (non-strict) mode without failing the call. It returns nil when the
// vault was explicitly chosen, only one vault exists, or the command is
// read-only.
func (s *Server) vaultFallbackWarning(commandID string, res vaultResolution) *commandexec.Warning {
	if !isAmbientVaultSource(res.source) {
		return nil
	}
	if meta, ok := commands.EffectiveMeta(commandID); ok && meta.Access == commands.AccessRead {
		return nil
	}
	if s.configuredVaultCount() <= 1 {
		return nil
	}
	name := res.name
	if name == "" {
		name = res.path
	}
	return &commandexec.Warning{
		Code:    codes.WarnVaultFallback,
		Message: "vault was resolved from ambient state (" + res.source + " -> " + name + "); pass vault or vault_path to target a vault explicitly",
	}
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

func normalizeCanonicalArgs(commandID string, args map[string]interface{}) map[string]interface{} {
	_ = commandID
	return args
}
