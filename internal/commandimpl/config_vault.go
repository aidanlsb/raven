package commandimpl

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
)

// HandleConfigShow executes the canonical `config_show` command.
func HandleConfigShow(_ context.Context, req commandexec.Request) commandexec.Result {
	ctx, err := configsvc.ShowContext(configContextOptions(req))
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "")
	}
	return commandexec.Success(ctx.Payload(), nil)
}

// HandleConfigInit executes the canonical `config_init` command.
func HandleConfigInit(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.Init(configsvc.InitRequest{
		ConfigPathOverride: strings.TrimSpace(req.ConfigPath),
	})
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "")
	}
	return commandexec.Success(result.Payload(), nil)
}

// HandleConfigSet executes the canonical `config_set` command.
func HandleConfigSet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.Set(configsvc.SetRequest{
		ContextOptions: configContextOptions(req),
		Settings:       stringSliceArg(req.Args["settings"]),
	})
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "")
	}

	return commandexec.Success(result.Payload(), nil)
}

// HandleConfigUnset executes the canonical `config_unset` command.
func HandleConfigUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.Unset(configsvc.UnsetRequest{
		ContextOptions: configContextOptions(req),
		Keys:           stringSliceArg(req.Args["keys"]),
	})
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn config init' first")
	}

	return commandexec.Success(result.Payload(), nil)
}

// HandleVaultList executes the canonical `vault_list` command.
func HandleVaultList(_ context.Context, req commandexec.Request) commandexec.Result {
	if boolArg(req.Args, "path-only") {
		return commandexec.Success(configsvc.VaultPathResult{
			Path: filepath.Clean(strings.TrimSpace(req.VaultPath)),
		}, nil)
	}

	result, err := configsvc.ListVaults(configContextOptions(req))
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "")
	}
	return commandexec.Success(result.Payload(), &commandexec.Meta{Count: len(result.Vaults)})
}

// HandleVaultUse executes the canonical `vault_use` command.
func HandleVaultUse(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.UseVault(configContextOptions(req), stringArg(req.Args, "name"))
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn vault list' to see configured vaults")
	}
	return commandexec.Success(result.Payload(), nil)
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
		return commandexec.FromServiceErrorWithFallback(err, "Use 'rvn vault list' to see configured vaults")
	}

	hint := ""
	if req.Caller == commandexec.CallerCLI {
		hint = "CLI validates the target only; invoke vault_focus through raven_invoke to update a running MCP server session."
	}
	return commandexec.Success(result.Payload(req.Caller == commandexec.CallerMCP, hint), nil)
}

// HandleVaultClear executes the canonical `vault_clear` command.
func HandleVaultClear(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.ClearActiveVault(configContextOptions(req))
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "")
	}
	return commandexec.Success(result.Payload(), nil)
}

// HandleVaultPin executes the canonical `vault_pin` command.
func HandleVaultPin(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := configsvc.PinVault(configContextOptions(req), stringArg(req.Args, "name"))
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn vault list' to see configured vaults")
	}
	return commandexec.Success(result.Payload(), nil)
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
			case codes.ErrFileNotFound:
				return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn init "+rawPath+"' to create it first")
			case codes.ErrDuplicateName:
				return commandexec.FromServiceErrorWithFallback(err, "Use --replace to update the path")
			}
		}
		return commandexec.FromServiceErrorWithFallback(err, "")
	}
	return commandexec.Success(result.Payload(), nil)
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
		if svcErr, ok := svcerr.AsError(err); ok && svcErr.Code == codes.ErrConfirmationRequired {
			if strings.Contains(svcErr.Message, "default vault") {
				return commandexec.FromServiceErrorWithFallback(err, "Use --clear-default to clear default_vault as part of removal, or pin another vault first")
			}
			if strings.Contains(svcErr.Message, "active vault") {
				return commandexec.FromServiceErrorWithFallback(err, "Use --clear-active to clear active_vault as part of removal, or switch active vault first")
			}
		}
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn vault list' to see configured vaults")
	}
	return commandexec.Success(result.Payload(), nil)
}

func configContextOptions(req commandexec.Request) configsvc.ContextOptions {
	return configsvc.ContextOptions{
		ConfigPathOverride: strings.TrimSpace(req.ConfigPath),
		StatePathOverride:  strings.TrimSpace(req.StatePath),
	}
}
