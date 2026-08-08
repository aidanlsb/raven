package commandimpl

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
)

// HandleConfigShow executes the canonical `config_show` command.
func HandleConfigShow(_ context.Context, req commandexec.Request) commandexec.Result {
	ctx, err := configsvc.ShowContext(configContextOptions(req))
	if err != nil {
		return mapConfigSvcFailure(err, "")
	}
	return commandexec.Success(ctx.Data(), nil)
}

// HandleConfigInit executes the canonical `config_init` command.
func HandleConfigInit(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.Init(configsvc.InitRequest{
		ConfigPathOverride: strings.TrimSpace(req.ConfigPath),
	})
	if err != nil {
		return mapConfigSvcFailure(err, "")
	}
	return commandexec.Success(map[string]interface{}{
		"config_path": result.ConfigPath,
		"created":     result.Created,
	}, nil)
}

// HandleConfigSet executes the canonical `config_set` command.
func HandleConfigSet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.Set(configsvc.SetRequest{
		ContextOptions: configContextOptions(req),
		Settings:       stringSliceArg(req.Args["settings"]),
	})
	if err != nil {
		return mapConfigSvcFailure(err, "")
	}

	data := result.Context.Data()
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

// HandleConfigUnset executes the canonical `config_unset` command.
func HandleConfigUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.Unset(configsvc.UnsetRequest{
		ContextOptions: configContextOptions(req),
		Keys:           stringSliceArg(req.Args["keys"]),
	})
	if err != nil {
		return mapConfigSvcFailure(err, "Run 'rvn config init' first")
	}

	data := result.Context.Data()
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

// HandleVaultList executes the canonical `vault_list` command.
func HandleVaultList(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.ListVaults(configContextOptions(req))
	if err != nil {
		return mapConfigSvcFailure(err, "")
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":    result.ConfigPath,
		"state_path":     result.StatePath,
		"default_vault":  result.DefaultVault,
		"active_vault":   result.ActiveVault,
		"active_missing": result.ActiveMissing,
		"vaults":         result.Vaults,
	}, &commandexec.Meta{Count: len(result.Vaults)})
}

// HandleVaultCurrent executes the canonical `vault_current` command.
func HandleVaultCurrent(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.CurrentVault(configContextOptions(req))
	if err != nil {
		return mapConfigSvcFailure(err, "Use 'rvn vault use <name>' or 'rvn vault pin <name>'")
	}
	return commandexec.Success(map[string]interface{}{
		"name":           result.Current.Name,
		"path":           result.Current.Path,
		"source":         result.Current.Source,
		"active_vault":   result.ActiveVault,
		"active_missing": result.Current.ActiveMissing,
		"config_path":    result.ConfigPath,
		"state_path":     result.StatePath,
	}, nil)
}

// HandleVaultUse executes the canonical `vault_use` command.
func HandleVaultUse(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.UseVault(configContextOptions(req), stringArg(req.Args, "name"))
	if err != nil {
		return mapConfigSvcFailure(err, "Run 'rvn vault list' to see configured vaults")
	}
	return commandexec.Success(map[string]interface{}{
		"active_vault": result.ActiveVault,
		"path":         result.Path,
		"state_path":   result.StatePath,
	}, nil)
}

// HandleVaultFocus validates a target for the current MCP server's in-memory
// session focus. The MCP adapter applies the validated result to its Server;
// direct CLI calls intentionally perform validation only.
func HandleVaultFocus(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.FocusVault(configsvc.VaultFocusRequest{
		ContextOptions: configContextOptions(req),
		Name:           stringArg(req.Args, "name"),
		Path:           stringArg(req.Args, "path"),
		Clear:          boolArg(req.Args, "clear"),
	})
	if err != nil {
		return mapConfigSvcFailure(err, "Use 'rvn vault list' to see configured vaults")
	}

	data := map[string]interface{}{
		"applied":      req.Caller == commandexec.CallerMCP,
		"cleared":      result.Cleared,
		"scope":        "mcp_session",
		"session_only": true,
	}
	if result.Name != "" {
		data["name"] = result.Name
	}
	if result.Path != "" {
		data["path"] = result.Path
	}
	if req.Caller == commandexec.CallerCLI {
		data["hint"] = "CLI validates the target only; invoke vault_focus through raven_invoke to update a running MCP server session."
	}
	return commandexec.Success(data, nil)
}

// HandleVaultClear executes the canonical `vault_clear` command.
func HandleVaultClear(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.ClearActiveVault(configContextOptions(req))
	if err != nil {
		return mapConfigSvcFailure(err, "")
	}
	return commandexec.Success(map[string]interface{}{
		"cleared":    result.Cleared,
		"previous":   result.Previous,
		"state_path": result.StatePath,
	}, nil)
}

// HandleVaultPin executes the canonical `vault_pin` command.
func HandleVaultPin(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.PinVault(configContextOptions(req), stringArg(req.Args, "name"))
	if err != nil {
		return mapConfigSvcFailure(err, "Run 'rvn vault list' to see configured vaults")
	}
	return commandexec.Success(map[string]interface{}{
		"default_vault": result.DefaultVault,
		"path":          result.Path,
		"config_path":   result.ConfigPath,
	}, nil)
}

// HandleVaultAdd executes the canonical `vault_add` command.
func HandleVaultAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	rawPath := strings.TrimSpace(stringArg(req.Args, "path"))
	result, err := configsvc.AddVault(configsvc.VaultAddRequest{
		ContextOptions: configContextOptions(req),
		Name:           stringArg(req.Args, "name"),
		RawPath:        rawPath,
		Replace:        boolArg(req.Args, "replace"),
		Pin:            boolArg(req.Args, "pin"),
	})
	if err != nil {
		if svcErr, ok := svcerr.AsError(err); ok {
			switch svcErr.Code {
			case configsvc.CodeFileNotFound:
				return mapConfigSvcFailure(err, "Run 'rvn init "+rawPath+"' to create it first")
			case configsvc.CodeDuplicateName:
				return mapConfigSvcFailure(err, "Use --replace to update the path")
			}
		}
		return mapConfigSvcFailure(err, "")
	}
	return commandexec.Success(map[string]interface{}{
		"name":          result.Name,
		"path":          result.Path,
		"config_path":   result.ConfigPath,
		"replaced":      result.Replaced,
		"previous_path": result.PreviousPath,
		"pinned":        result.Pinned,
		"default_vault": result.DefaultVault,
	}, nil)
}

// HandleVaultRemove executes the canonical `vault_remove` command.
func HandleVaultRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.RemoveVault(configsvc.VaultRemoveRequest{
		ContextOptions: configContextOptions(req),
		Name:           stringArg(req.Args, "name"),
		ClearDefault:   boolArg(req.Args, "clear-default"),
		ClearActive:    boolArg(req.Args, "clear-active"),
	})
	if err != nil {
		if svcErr, ok := svcerr.AsError(err); ok && svcErr.Code == configsvc.CodeConfirmationNeeded {
			if strings.Contains(svcErr.Message, "default vault") {
				return mapConfigSvcFailure(err, "Use --clear-default to clear default_vault as part of removal, or pin another vault first")
			}
			if strings.Contains(svcErr.Message, "active vault") {
				return mapConfigSvcFailure(err, "Use --clear-active to clear active_vault as part of removal, or switch active vault first")
			}
		}
		return mapConfigSvcFailure(err, "Run 'rvn vault list' to see configured vaults")
	}
	return commandexec.Success(map[string]interface{}{
		"name":            result.Name,
		"removed_path":    result.RemovedPath,
		"default_cleared": result.DefaultCleared,
		"active_cleared":  result.ActiveCleared,
		"config_path":     result.ConfigPath,
		"state_path":      result.StatePath,
	}, nil)
}

// HandleVaultPath executes the canonical `vault_path` command. The shared
// invoker validateRequest step guarantees req.VaultPath is non-empty for
// vault-scoped commands, so this handler can render the resolved path directly.
func HandleVaultPath(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	return commandexec.Success(map[string]interface{}{"path": filepath.Clean(vaultPath)}, nil)
}

func configContextOptions(req commandexec.Request) configsvc.ContextOptions {
	return configsvc.ContextOptions{
		ConfigPathOverride: strings.TrimSpace(req.ConfigPath),
		StatePathOverride:  strings.TrimSpace(req.StatePath),
	}
}

func mapConfigSvcFailure(err error, fallbackSuggestion string) commandexec.Result {
	return commandexec.FromServiceErrorWithFallback(err, fallbackSuggestion)
}
