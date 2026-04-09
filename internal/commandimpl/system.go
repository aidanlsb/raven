package commandimpl

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/datesvc"
	"github.com/aidanlsb/raven/internal/initsvc"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// HandleInit executes the canonical `init` command.
func HandleInit(_ context.Context, req commandexec.Request) commandexec.Result {
	path := strings.TrimSpace(stringArg(req.Args, "path"))
	if path == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "path is required", nil, "Usage: rvn init <path>")
	}

	version := maintsvc.CurrentVersionInfo().Version
	result, err := initsvc.Initialize(initsvc.InitializeRequest{
		Path:       path,
		CLIVersion: version,
	})
	if err != nil {
		svcErr, ok := initsvc.AsError(err)
		if !ok {
			return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
		}
		return commandexec.Failure(string(svcErr.Code), svcErr.Message, nil, svcErr.Suggestion)
	}

	warnings := make([]commandexec.Warning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, commandexec.Warning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"path":            result.Path,
		"status":          result.Status,
		"created_config":  result.CreatedConfig,
		"created_schema":  result.CreatedSchema,
		"gitignore_state": result.GitignoreState,
		"docs":            result.Docs,
		"post_init":       buildInitPostInitData(result.Path, req.ConfigPath, req.StatePath),
	}, warnings, nil)
}

// HandleReindex executes the canonical `reindex` command.
func HandleReindex(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	start := time.Now()
	result, err := reindexsvc.Run(reindexsvc.RunRequest{
		VaultPath: vaultPath,
		Full:      boolArg(req.Args, "full"),
		DryRun:    boolArg(req.Args, "dry-run"),
		Context:   context.Background(),
	})
	if err != nil {
		svcErr, ok := reindexsvc.AsError(err)
		if !ok {
			return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
		}
		return commandexec.Failure(string(svcErr.Code), svcErr.Message, nil, svcErr.Suggestion)
	}

	warnings := make([]commandexec.Warning, 0, len(result.WarningMessages))
	for _, warning := range result.WarningMessages {
		warnings = append(warnings, commandexec.Warning{
			Code:    indexUpdateFailedWarningCode,
			Message: warning,
		})
	}

	return commandexec.SuccessWithWarnings(result.Data(), warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleDaily executes the canonical `daily` command.
func HandleDaily(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	result, err := datesvc.EnsureDaily(datesvc.EnsureDailyRequest{
		VaultPath:  vaultPath,
		DateArg:    stringArg(req.Args, "date"),
		TemplateID: stringArg(req.Args, "template"),
	})
	if err != nil {
		return mapDateServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"file":    result.RelativePath,
		"date":    result.Date,
		"created": result.Created,
		"opened":  false,
	}, nil)
}

// HandleDate executes the canonical `date` command.
func HandleDate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	result, err := datesvc.DateHub(datesvc.DateHubRequest{
		VaultPath: vaultPath,
		DateArg:   stringArg(req.Args, "date"),
	})
	if err != nil {
		return mapDateServiceError(err)
	}

	data := map[string]interface{}{
		"date":          result.Date,
		"day_of_week":   result.DayOfWeek,
		"daily_note_id": result.DailyNoteID,
		"daily_path":    result.DailyPath,
		"daily_exists":  result.DailyExists,
		"items":         result.Items,
		"backlinks":     result.Backlinks,
	}
	if result.DailyNote != nil {
		data["daily_note"] = result.DailyNote
	}

	return commandexec.Success(data, &commandexec.Meta{Count: len(result.Items)})
}

// HandleVersion executes the canonical `version` command.
func HandleVersion(_ context.Context, req commandexec.Request) commandexec.Result {
	info := versioninfo.Current()
	if strings.TrimSpace(req.ExecutablePath) != "" {
		info = maintsvc.CurrentVersionInfoFromExecutable(req.ExecutablePath)
	}
	return commandexec.Success(map[string]interface{}{
		"version":     info.Version,
		"module_path": info.ModulePath,
		"commit":      info.Commit,
		"commit_time": info.CommitTime,
		"modified":    info.Modified,
		"go_version":  info.GoVersion,
		"goos":        info.GOOS,
		"goarch":      info.GOARCH,
	}, nil)
}

func mapDateServiceError(err error) commandexec.Result {
	svcErr, ok := datesvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	return commandexec.Failure(string(svcErr.Code), svcErr.Message, nil, svcErr.Suggestion)
}

func buildInitPostInitData(path, configPathOverride, statePathOverride string) map[string]interface{} {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return map[string]interface{}{}
	}

	absPath, err := filepath.Abs(cleanPath)
	if err == nil {
		cleanPath = absPath
	}

	suggestedName := slugs.ComponentSlug(filepath.Base(cleanPath))
	if suggestedName == "" {
		suggestedName = "vault"
	}

	configPath := config.ResolveConfigPath(configPathOverride)
	statePath := config.ResolveStatePath(statePathOverride, configPath, &config.Config{})
	registeredName := ""
	isActive := false
	isDefault := false

	ctx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{
		ConfigPathOverride: configPathOverride,
		StatePathOverride:  statePathOverride,
	})
	if err == nil {
		configPath = ctx.ConfigPath
		statePath = ctx.StatePath
		defaultName := configsvc.DefaultVaultName(ctx.Cfg)
		activeName := strings.TrimSpace(ctx.State.ActiveVault)
		for name, vaultPath := range ctx.Cfg.ListVaults() {
			if filepath.Clean(vaultPath) != filepath.Clean(cleanPath) {
				continue
			}
			registeredName = name
			isDefault = name == defaultName
			isActive = name == activeName
			break
		}
	}

	nameForCommands := registeredName
	if nameForCommands == "" {
		nameForCommands = suggestedName
	}

	quotedPath := formatSuggestedCommandPath(cleanPath)
	commands := map[string]interface{}{
		"register":          "rvn vault add " + nameForCommands + " " + quotedPath + " --json",
		"register_and_pin":  "rvn vault add " + nameForCommands + " " + quotedPath + " --pin --json",
		"activate":          "rvn vault use " + nameForCommands + " --json",
		"pin":               "rvn vault pin " + nameForCommands + " --json",
		"register_activate": "rvn vault add " + nameForCommands + " " + quotedPath + " --json && rvn vault use " + nameForCommands + " --json",
	}

	nextSteps := make([]string, 0, 3)
	if registeredName == "" {
		nextSteps = append(nextSteps, "Register this vault globally: "+commands["register"].(string))
		nextSteps = append(nextSteps, "Register and set as default: "+commands["register_and_pin"].(string))
		nextSteps = append(nextSteps, "After registering, make it active: "+commands["activate"].(string))
	} else {
		if !isDefault {
			nextSteps = append(nextSteps, "Set this vault as default: "+commands["pin"].(string))
		}
		if !isActive {
			nextSteps = append(nextSteps, "Set this vault as active: "+commands["activate"].(string))
		}
	}

	return map[string]interface{}{
		"suggested_name":     suggestedName,
		"registered_name":    registeredName,
		"already_registered": registeredName != "",
		"is_active":          isActive,
		"is_default":         isDefault,
		"config_path":        configPath,
		"state_path":         statePath,
		"commands":           commands,
		"next_steps":         nextSteps,
	}
}

func formatSuggestedCommandPath(path string) string {
	displayPath := strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	return strconv.Quote(displayPath)
}
