package commandimpl

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
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
		ConfigPath: req.ConfigPath,
		CLIVersion: version,
	})
	if err != nil {
		svcErr, ok := initsvc.AsError(err)
		if !ok {
			return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
		}
		return commandexec.Failure(svcErr.Code, svcErr.Message, nil, svcErr.Suggestion)
	}

	postInit, setupWarnings := setupInitVault(result.Path, req.ConfigPath, req.StatePath)

	warnings := make([]commandexec.Warning, 0, len(result.Warnings)+len(setupWarnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, commandexec.Warning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	warnings = append(warnings, setupWarnings...)

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"path":            result.Path,
		"status":          result.Status,
		"created_config":  result.CreatedConfig,
		"created_schema":  result.CreatedSchema,
		"gitignore_state": result.GitignoreState,
		"docs":            result.Docs,
		"post_init":       postInit,
	}, warnings, nil)
}

// HandleReindex executes the canonical `reindex` command.
func HandleReindex(ctx context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	start := time.Now()
	result, err := reindexsvc.Run(reindexsvc.RunRequest{
		VaultPath: vaultPath,
		Full:      boolArg(req.Args, "full"),
		DryRun:    boolArg(req.Args, "dry-run"),
		Context:   ctx,
	})
	if err != nil {
		svcErr, ok := reindexsvc.AsError(err)
		if !ok {
			return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
		}
		return commandexec.Failure(svcErr.Code, svcErr.Message, nil, svcErr.Suggestion)
	}

	warnings := make([]commandexec.Warning, 0, len(result.WarningMessages))
	for _, warning := range result.WarningMessages {
		code := indexUpdateFailedWarningCode
		if strings.Contains(warning, "unknown frontmatter key") {
			code = codes.WarnUnknownField
		}
		warnings = append(warnings, commandexec.Warning{
			Code:    code,
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

	return commandexec.Failure(svcErr.Code, svcErr.Message, nil, svcErr.Suggestion)
}

// initPostInitState is the resolved outcome of the first-run vault policy, used
// to build the post_init payload returned by `init`.
type initPostInitState struct {
	cleanPath          string
	suggestedName      string
	registeredName     string
	registeredNow      bool
	isFirstVault       bool
	hasExistingDefault bool
	isDefault          bool
	isActive           bool
	configPath         string
	statePath          string
}

// setupInitVault applies Raven's first-run vault policy after a successful init
// and returns the post_init payload plus any non-fatal warnings.
//
// Policy:
//   - The new vault is always auto-registered under a suggested (collision-free) name.
//   - First vault on the machine (no default, no active vault, no other registered
//     vault): additionally set it as the default and active vault.
//   - Otherwise: register only; the default and active vault are left untouched so a
//     new vault never silently switches an existing setup. Agents must ask the user
//     before activating it or changing the default.
//
// Registration is additive and never fatal: if global config cannot be loaded or
// written, init still succeeds and the payload degrades to manual suggestions.
func setupInitVault(path, configPathOverride, statePathOverride string) (map[string]interface{}, []commandexec.Warning) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return map[string]interface{}{}, nil
	}
	if absPath, err := filepath.Abs(cleanPath); err == nil {
		cleanPath = absPath
	}

	suggestedName := slugs.ComponentSlug(filepath.Base(cleanPath))
	if suggestedName == "" {
		suggestedName = "vault"
	}

	state := initPostInitState{
		cleanPath:     cleanPath,
		suggestedName: suggestedName,
		configPath:    config.ResolveConfigPath(configPathOverride),
	}
	state.statePath = config.ResolveStatePath(statePathOverride, state.configPath, &config.Config{})

	opts := configsvc.ContextOptions{
		ConfigPathOverride: configPathOverride,
		StatePathOverride:  statePathOverride,
	}

	warnings := make([]commandexec.Warning, 0, 1)

	ctx, err := configsvc.LoadVaultContext(opts)
	if err != nil {
		warnings = append(warnings, commandexec.Warning{
			Code:    codes.WarnVaultRegisterFailed,
			Message: fmt.Sprintf("Could not load global config to register the vault: %v. Register it manually with the commands in post_init.", err),
		})
		return buildInitPostInitPayload(state), warnings
	}

	state.configPath = ctx.ConfigPath
	state.statePath = ctx.StatePath

	existingVaults := ctx.Cfg.ListVaults()
	defaultName := configsvc.DefaultVaultName(ctx.Cfg)
	activeName := strings.TrimSpace(ctx.State.ActiveVault)

	otherVaultExists := false
	for name, vaultPath := range existingVaults {
		if filepath.Clean(vaultPath) == filepath.Clean(cleanPath) {
			state.registeredName = name
		} else {
			otherVaultExists = true
		}
	}

	state.hasExistingDefault = defaultName != ""
	hasExistingActive := activeName != ""
	state.isFirstVault = !state.hasExistingDefault && !hasExistingActive && !otherVaultExists

	// Auto-register the new vault when it is not already known by path.
	if state.registeredName == "" {
		name := uniqueVaultName(existingVaults, suggestedName)
		addResult, addErr := configsvc.AddVault(configsvc.VaultAddRequest{
			ContextOptions: opts,
			Name:           name,
			RawPath:        cleanPath,
		})
		if addErr != nil {
			warnings = append(warnings, commandexec.Warning{
				Code:    codes.WarnVaultRegisterFailed,
				Message: fmt.Sprintf("Could not auto-register the vault: %v. Register it manually with the commands in post_init.", addErr),
			})
		} else {
			state.registeredName = addResult.Name
			state.registeredNow = true
		}
	}

	state.isDefault = state.registeredName != "" && state.registeredName == defaultName
	state.isActive = state.registeredName != "" && state.registeredName == activeName

	// First vault on the machine: pin as default and activate.
	if state.isFirstVault && state.registeredName != "" {
		if !state.isDefault {
			if _, pinErr := configsvc.PinVault(opts, state.registeredName); pinErr != nil {
				warnings = append(warnings, commandexec.Warning{
					Code:    codes.WarnVaultRegisterFailed,
					Message: fmt.Sprintf("Registered the vault but could not set it as default: %v.", pinErr),
				})
			} else {
				state.isDefault = true
			}
		}
		if !state.isActive {
			if _, useErr := configsvc.UseVault(opts, state.registeredName); useErr != nil {
				warnings = append(warnings, commandexec.Warning{
					Code:    codes.WarnVaultRegisterFailed,
					Message: fmt.Sprintf("Registered the vault but could not set it active: %v.", useErr),
				})
			} else {
				state.isActive = true
			}
		}
	}

	return buildInitPostInitPayload(state), warnings
}

// uniqueVaultName returns base if free, otherwise the first free base-N variant.
func uniqueVaultName(existing map[string]string, base string) string {
	if strings.TrimSpace(base) == "" {
		base = "vault"
	}
	if _, taken := existing[base]; !taken {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, taken := existing[candidate]; !taken {
			return candidate
		}
	}
}

// buildInitPostInitPayload renders the post_init payload from a resolved state.
func buildInitPostInitPayload(s initPostInitState) map[string]interface{} {
	if s.cleanPath == "" {
		return map[string]interface{}{}
	}

	alreadyRegistered := s.registeredName != ""
	needsActivateChoice := alreadyRegistered && !s.isActive && !s.isFirstVault
	needsDefaultChoice := alreadyRegistered && !s.isDefault && !s.isFirstVault

	nameForCommands := s.registeredName
	if nameForCommands == "" {
		nameForCommands = s.suggestedName
	}
	quotedPath := formatSuggestedCommandPath(s.cleanPath)
	commands := map[string]interface{}{
		"register":          "rvn vault add " + nameForCommands + " " + quotedPath + " --json",
		"register_and_pin":  "rvn vault add " + nameForCommands + " " + quotedPath + " --pin --json",
		"activate":          "rvn vault use " + nameForCommands + " --json",
		"pin":               "rvn vault pin " + nameForCommands + " --json",
		"register_activate": "rvn vault add " + nameForCommands + " " + quotedPath + " --json && rvn vault use " + nameForCommands + " --json",
	}

	// Structured, invocable next actions (command IDs + args), for agents that
	// should act on the payload rather than parse shell strings.
	actions := map[string]interface{}{}
	if alreadyRegistered {
		actions["activate"] = map[string]interface{}{
			"command":     "vault use",
			"args":        map[string]interface{}{"name": s.registeredName},
			"description": "Set this vault as the active vault (machine-local state).",
		}
		actions["set_default"] = map[string]interface{}{
			"command":     "vault pin",
			"args":        map[string]interface{}{"name": s.registeredName},
			"description": "Set this vault as the default vault.",
		}
	} else {
		actions["register"] = map[string]interface{}{
			"command":     "vault add",
			"args":        map[string]interface{}{"name": s.suggestedName, "path": s.cleanPath},
			"description": "Register this vault in global config.",
		}
	}

	nextSteps := make([]string, 0, 3)
	guidance := ""
	switch {
	case !alreadyRegistered:
		guidance = "The new vault could not be registered automatically. Register it before use with the commands below."
		nextSteps = append(nextSteps, "Register this vault globally: "+commands["register"].(string))
		nextSteps = append(nextSteps, "Register and set as default: "+commands["register_and_pin"].(string))
		nextSteps = append(nextSteps, "After registering, make it active: "+commands["activate"].(string))
	case s.isFirstVault:
		guidance = fmt.Sprintf("First vault on this machine: registered as %q, set as the default, and activated. No further vault setup is needed.", s.registeredName)
	default:
		guidance = fmt.Sprintf("Another vault is already configured on this machine. The new vault was registered as %q but was NOT activated or set as default. Ask the user before activating it or changing the default vault.", s.registeredName)
		if needsDefaultChoice {
			nextSteps = append(nextSteps, "Ask the user first, then set this vault as default: "+commands["pin"].(string))
		}
		if needsActivateChoice {
			nextSteps = append(nextSteps, "Ask the user first, then set this vault as active: "+commands["activate"].(string))
		}
	}

	return map[string]interface{}{
		"suggested_name":                 s.suggestedName,
		"registered_name":                s.registeredName,
		"already_registered":             alreadyRegistered,
		"registered":                     s.registeredNow,
		"is_first_vault":                 s.isFirstVault,
		"has_existing_default":           s.hasExistingDefault,
		"is_active":                      s.isActive,
		"is_default":                     s.isDefault,
		"needs_user_choice_for_activate": needsActivateChoice,
		"needs_user_choice_for_default":  needsDefaultChoice,
		"config_path":                    s.configPath,
		"state_path":                     s.statePath,
		"commands":                       commands,
		"actions":                        actions,
		"next_steps":                     nextSteps,
		"guidance":                       guidance,
	}
}

func formatSuggestedCommandPath(path string) string {
	displayPath := strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	return strconv.Quote(displayPath)
}
