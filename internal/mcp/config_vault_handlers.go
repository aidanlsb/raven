package mcp

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/configsvc"
)

func (s *Server) directConfigContextOptions() configsvc.ContextOptions {
	opts := configsvc.ContextOptions{}
	for i := 0; i < len(s.baseArgs); i++ {
		arg := strings.TrimSpace(s.baseArgs[i])
		switch {
		case arg == "--config" && i+1 < len(s.baseArgs):
			opts.ConfigPathOverride = strings.TrimSpace(s.baseArgs[i+1])
			i++
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPathOverride = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
		case arg == "--state" && i+1 < len(s.baseArgs):
			opts.StatePathOverride = strings.TrimSpace(s.baseArgs[i+1])
			i++
		case strings.HasPrefix(arg, "--state="):
			opts.StatePathOverride = strings.TrimSpace(strings.TrimPrefix(arg, "--state="))
		}
	}
	return opts
}

func mapConfigSvcError(err error, fallbackSuggestion string) (string, bool) {
	svcErr, ok := configsvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), fallbackSuggestion, nil), true
	}
	suggestion := fallbackSuggestion
	return errorEnvelope(string(svcErr.Code), svcErr.Error(), suggestion, nil), true
}

func (s *Server) callDirectConfigShow(args map[string]interface{}) (string, bool) {
	ctx, err := configsvc.ShowContext(s.directConfigContextOptions())
	if err != nil {
		return mapConfigSvcError(err, "")
	}
	return successEnvelope(ctx.Data(), nil), false
}

func (s *Server) callDirectConfigInit(args map[string]interface{}) (string, bool) {
	result, err := configsvc.Init(configsvc.InitRequest{
		ConfigPathOverride: s.directConfigContextOptions().ConfigPathOverride,
	})
	if err != nil {
		return mapConfigSvcError(err, "")
	}

	return successEnvelope(map[string]interface{}{
		"config_path": result.ConfigPath,
		"created":     result.Created,
	}, nil), false
}

func (s *Server) callDirectConfigSet(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	opts := s.directConfigContextOptions()
	req := configsvc.SetRequest{
		ContextOptions: opts,
	}

	if raw, ok := normalized["editor"]; ok {
		v := toString(raw)
		req.Editor = &v
	}
	if raw, ok := normalized["editor-mode"]; ok {
		v := toString(raw)
		req.EditorMode = &v
	}
	if raw, ok := normalized["state-file"]; ok {
		v := toString(raw)
		req.StateFile = &v
	}
	if raw, ok := normalized["default-vault"]; ok {
		v := toString(raw)
		req.DefaultVault = &v
	}
	if raw, ok := normalized["ui-accent"]; ok {
		v := toString(raw)
		req.UIAccent = &v
	}
	if raw, ok := normalized["ui-code-theme"]; ok {
		v := toString(raw)
		req.UICodeTheme = &v
	}

	result, err := configsvc.Set(req)
	if err != nil {
		if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeInvalidInput && strings.Contains(svcErr.Message, "not configured") {
			return mapConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
		}
		return mapConfigSvcError(err, "")
	}

	data := result.Context.Data()
	data["changed"] = result.Changed
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectConfigUnset(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	req := configsvc.UnsetRequest{
		ContextOptions: configsvc.ContextOptions{
			ConfigPathOverride: s.directConfigContextOptions().ConfigPathOverride,
			StatePathOverride:  s.directConfigContextOptions().StatePathOverride,
		},
		Editor:       boolValue(normalized["editor"]),
		EditorMode:   boolValue(normalized["editor-mode"]),
		StateFile:    boolValue(normalized["state-file"]),
		DefaultVault: boolValue(normalized["default-vault"]),
		UIAccent:     boolValue(normalized["ui-accent"]),
		UICodeTheme:  boolValue(normalized["ui-code-theme"]),
	}

	result, err := configsvc.Unset(req)
	if err != nil {
		return mapConfigSvcError(err, "Run 'rvn config init' first")
	}
	data := result.Context.Data()
	data["changed"] = result.Changed
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectVaultList(args map[string]interface{}) (string, bool) {
	result, err := configsvc.ListVaults(s.directConfigContextOptions())
	if err != nil {
		return mapConfigSvcError(err, "")
	}
	return successEnvelope(map[string]interface{}{
		"config_path":    result.ConfigPath,
		"state_path":     result.StatePath,
		"default_vault":  result.DefaultVault,
		"active_vault":   result.ActiveVault,
		"active_missing": result.ActiveMissing,
		"vaults":         result.Vaults,
	}, nil), false
}

func (s *Server) callDirectVaultCurrent(args map[string]interface{}) (string, bool) {
	result, err := configsvc.CurrentVault(s.directConfigContextOptions())
	if err != nil {
		return mapConfigSvcError(err, "Use 'rvn vault use <name>' or set default_vault in config.toml")
	}
	return successEnvelope(map[string]interface{}{
		"name":           result.Current.Name,
		"path":           result.Current.Path,
		"source":         result.Current.Source,
		"active_missing": result.Current.ActiveMissing,
		"config_path":    result.ConfigPath,
		"state_path":     result.StatePath,
	}, nil), false
}

func (s *Server) callDirectVaultUse(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	name := strings.TrimSpace(toString(normalized["name"]))
	result, err := configsvc.UseVault(s.directConfigContextOptions(), name)
	if err != nil {
		return mapConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
	}
	return successEnvelope(map[string]interface{}{
		"active_vault": result.ActiveVault,
		"path":         result.Path,
		"state_path":   result.StatePath,
	}, nil), false
}

func (s *Server) callDirectVaultClear(args map[string]interface{}) (string, bool) {
	result, err := configsvc.ClearActiveVault(s.directConfigContextOptions())
	if err != nil {
		return mapConfigSvcError(err, "")
	}
	return successEnvelope(map[string]interface{}{
		"cleared":    result.Cleared,
		"previous":   result.Previous,
		"state_path": result.StatePath,
	}, nil), false
}

func (s *Server) callDirectVaultPin(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	name := strings.TrimSpace(toString(normalized["name"]))
	result, err := configsvc.PinVault(s.directConfigContextOptions(), name)
	if err != nil {
		return mapConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
	}
	return successEnvelope(map[string]interface{}{
		"default_vault": result.DefaultVault,
		"path":          result.Path,
		"config_path":   result.ConfigPath,
	}, nil), false
}

func (s *Server) callDirectVaultAdd(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	name := strings.TrimSpace(toString(normalized["name"]))
	rawPath := strings.TrimSpace(toString(normalized["path"]))
	result, err := configsvc.AddVault(configsvc.VaultAddRequest{
		ContextOptions: s.directConfigContextOptions(),
		Name:           name,
		RawPath:        rawPath,
		Replace:        boolValue(normalized["replace"]),
		Pin:            boolValue(normalized["pin"]),
	})
	if err != nil {
		if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeFileNotFound {
			return mapConfigSvcError(err, "Run 'rvn init "+rawPath+"' to create it first")
		}
		if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeDuplicateName {
			return mapConfigSvcError(err, "Use --replace to update the path")
		}
		return mapConfigSvcError(err, "")
	}
	return successEnvelope(map[string]interface{}{
		"name":          result.Name,
		"path":          result.Path,
		"config_path":   result.ConfigPath,
		"replaced":      result.Replaced,
		"previous_path": result.PreviousPath,
		"pinned":        result.Pinned,
		"default_vault": result.DefaultVault,
	}, nil), false
}

func (s *Server) callDirectVaultRemove(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	name := strings.TrimSpace(toString(normalized["name"]))
	result, err := configsvc.RemoveVault(configsvc.VaultRemoveRequest{
		ContextOptions: s.directConfigContextOptions(),
		Name:           name,
		ClearDefault:   boolValue(normalized["clear-default"]),
		ClearActive:    boolValue(normalized["clear-active"]),
	})
	if err != nil {
		if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeConfirmationNeeded {
			if strings.Contains(svcErr.Message, "default vault") {
				return mapConfigSvcError(err, "Use --clear-default to clear default_vault as part of removal, or pin another vault first")
			}
			if strings.Contains(svcErr.Message, "active vault") {
				return mapConfigSvcError(err, "Use --clear-active to clear active_vault as part of removal, or switch active vault first")
			}
		}
		return mapConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
	}
	return successEnvelope(map[string]interface{}{
		"name":            result.Name,
		"removed_path":    result.RemovedPath,
		"removed_legacy":  result.RemovedLegacy,
		"default_cleared": result.DefaultCleared,
		"active_cleared":  result.ActiveCleared,
		"config_path":     result.ConfigPath,
		"state_path":      result.StatePath,
	}, nil), false
}

func (s *Server) callDirectVaultPath(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_NOT_SPECIFIED", err.Error(), "Use --vault-path, --vault, active_vault, or default_vault", nil), true
	}
	return successEnvelope(map[string]interface{}{"path": filepath.Clean(vaultPath)}, nil), false
}
