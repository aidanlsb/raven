package mcp

import (
	"context"
	"encoding/json"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

func (s *Server) callCanonicalCommand(commandID string, args map[string]interface{}, vaultName, vaultPathOverride string) (string, bool, bool) {
	invoker := app.CommandInvoker()
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

	result := invoker.Execute(context.Background(), commandexec.Request{
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

func marshalCanonicalResult(result commandexec.Result) (string, bool, bool) {
	b, err := json.Marshal(result)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to marshal command result", "", nil), true, true
	}
	return string(b), !result.OK, true
}

func normalizeCanonicalArgs(commandID string, args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}
	if commandID != "update" {
		return args
	}

	if _, ok := args["object_ids"]; ok {
		return args
	}

	normalized := make(map[string]interface{}, len(args)+1)
	for key, value := range args {
		normalized[key] = value
	}
	switch {
	case normalized["trait_ids"] != nil:
		normalized["object_ids"] = normalized["trait_ids"]
	case normalized["trait-ids"] != nil:
		normalized["object_ids"] = normalized["trait-ids"]
	case normalized["ids"] != nil:
		normalized["object_ids"] = normalized["ids"]
	}
	return normalized
}
