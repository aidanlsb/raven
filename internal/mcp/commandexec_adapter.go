package mcp

import (
	"context"
	"encoding/json"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func (s *Server) callCanonicalCommand(commandID string, args map[string]interface{}, vaultName, vaultPathOverride string) (string, bool, bool) {
	invoker := app.CommandInvoker()
	if invoker == nil {
		return "", false, false
	}
	if _, ok := invoker.Handlers().Lookup(commandID); !ok {
		return "", false, false
	}

	vaultPath := ""
	if canonicalCommandNeedsVaultPath(commandID) {
		resolvedVaultPath, err := s.resolveVaultPathForInvocation(vaultName, vaultPathOverride)
		if err != nil {
			return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve vault for invocation", err.Error(), nil), true, true
		}
		vaultPath = resolvedVaultPath
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
	return marshalCanonicalResult(result)
}

func canonicalCommandNeedsVaultPath(commandID string) bool {
	switch commandID {
	case "init", "version",
		"config_show", "config_init", "config_set", "config_unset",
		"vault_list", "vault_current", "vault_use", "vault_add", "vault_remove", "vault_pin", "vault_clear",
		"skill_list", "skill_install", "skill_remove", "skill_doctor":
		return false
	default:
		return true
	}
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
