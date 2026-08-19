package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/datesvc"
	"github.com/aidanlsb/raven/internal/initsvc"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/shellquote"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// HandleInit executes the canonical `init` command.
func HandleInit(_ context.Context, req commandexec.Request) commandexec.Result {
	path := strings.TrimSpace(stringArg(req.Args, "path"))
	if path == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "path is required", nil, "Usage: rvn init <path>")
	}

	version := versioninfo.CurrentVersionInfo().Version
	result, err := initsvc.Initialize(initsvc.InitializeRequest{
		Path:       path,
		ConfigPath: req.ConfigPath,
		CLIVersion: version,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	postInit, setupWarnings, setupErr := setupInitVault(result.Path, req.ConfigPath, req.StatePath)
	if setupErr != nil {
		errorCode := codes.ErrInternal
		var initSetupErr *initVaultSetupError
		if errors.As(setupErr, &initSetupErr) {
			errorCode = initSetupErr.code
		}
		return commandexec.Failure(
			errorCode,
			fmt.Sprintf("vault initialized at %s, but safe global vault setup could not be completed: %v", result.Path, setupErr),
			map[string]interface{}{
				"initialized": true,
				"path":        result.Path,
				"post_init":   postInit,
			},
			"Fix global config/state access and rerun init, or pass --vault/--vault-path explicitly on every vault-scoped command.",
		)
	}

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

	rt, failure := newSchemaFirstCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	start := time.Now()
	result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{
		VaultPath: vaultPath,
		Full:      boolArg(req.Args, "full"),
		DryRun:    boolArg(req.Args, "dry-run"),
		Context:   ctx,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
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

	rt, failure := newConfigCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := datesvc.EnsureDaily(rt, datesvc.EnsureDailyRequest{
		VaultPath:  vaultPath,
		DateArg:    stringArg(req.Args, "date"),
		TemplateID: stringArg(req.Args, "template"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"file":    result.RelativePath,
		"id":      result.Date,
		"date":    result.Date,
		"created": result.Created,
		"opened":  false,
	}, nil)
}

// HandleDate executes the canonical `date` command.
func HandleDate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newConfigOnlyCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := datesvc.DateHub(rt, datesvc.DateHubRequest{
		VaultPath: vaultPath,
		DateArg:   stringArg(req.Args, "date"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
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
		info = versioninfo.CurrentVersionInfoFromExecutable(req.ExecutablePath)
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

// initPostInitState is the resolved outcome of the first-run vault policy, used
// to build the post_init payload returned by `init`.
type initPostInitState struct {
	cleanPath           string
	suggestedName       string
	registeredName      string
	registeredPath      string
	registeredNow       bool
	isFirstVault        bool
	hasExistingDefault  bool
	isDefault           bool
	isActive            bool
	activated           bool
	previousActiveName  string
	previousActivePath  string
	previousVaultName   string
	previousVaultPath   string
	previousVaultSource string
	previousDefaultName string
	previousDefaultPath string
	switchBack          string
	configPath          string
	statePath           string
}

type initVaultSetupError struct {
	code codes.ErrorCode
	err  error
}

func (e *initVaultSetupError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *initVaultSetupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// setupInitVault applies Raven's first-run vault policy after a successful init
// and returns the post_init payload plus any non-fatal warnings.
//
// Policy:
//   - The new vault is always auto-registered under a suggested (collision-free) name.
//   - First vault on the machine (no default, no active vault, no other registered
//     vault): additionally set it as the default and active vault.
//   - Otherwise: set the newly initialized vault active while leaving the default
//     unchanged. The response discloses the previous selection and how to switch back.
//
// Loading global config/state, registration, and activation failures are fatal so init
// cannot report a completed routing switch when ambient commands still target elsewhere.
func setupInitVault(path, configPathOverride, statePathOverride string) (map[string]interface{}, []commandexec.Warning, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return map[string]interface{}{}, nil, nil
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
	state.statePath = config.ResolveStatePath(statePathOverride, state.configPath)

	opts := configsvc.ContextOptions{
		ConfigPathOverride: configPathOverride,
		StatePathOverride:  statePathOverride,
	}

	warnings := make([]commandexec.Warning, 0, 1)

	ctx, err := configsvc.LoadVaultContext(opts)
	if ctx != nil {
		state.configPath = ctx.ConfigPath
		state.statePath = ctx.StatePath
	}
	if err != nil {
		return buildInitPostInitPayload(state), warnings, &initVaultSetupError{
			code: codes.ErrConfigInvalid,
			err:  fmt.Errorf("could not load global config/state: %w", err),
		}
	}

	existingVaults := ctx.Cfg.ListVaults()
	defaultName := configsvc.DefaultVaultName(ctx.Cfg)
	state.previousDefaultName = defaultName
	if defaultPath, defaultErr := ctx.Cfg.GetDefaultVaultPath(); defaultErr == nil {
		state.previousDefaultPath = defaultPath
	}
	activeName := strings.TrimSpace(ctx.State.ActiveVault)
	if activeName != "" {
		if activePath, activeErr := ctx.Cfg.GetVaultPath(activeName); activeErr == nil {
			state.previousActiveName = activeName
			state.previousActivePath = activePath
		}
	}
	if current, currentErr := configsvc.ResolveCurrentVault(ctx.Cfg, ctx.State); currentErr == nil {
		state.previousVaultName = current.Name
		state.previousVaultPath = current.Path
		state.previousVaultSource = current.Source
	}

	otherVaultExists := false
	for name, vaultPath := range existingVaults {
		if configsvc.SameVaultPath(vaultPath, cleanPath) {
			state.registeredName = name
			state.registeredPath = vaultPath
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
			return buildInitPostInitPayload(state), warnings, &initVaultSetupError{
				code: codes.ErrFileWrite,
				err:  fmt.Errorf("could not auto-register vault: %w", addErr),
			}
		}
		state.registeredName = addResult.Name
		state.registeredPath = addResult.Path
		state.registeredNow = true
	}

	state.isDefault = state.registeredName != "" && state.registeredName == defaultName
	state.isActive = state.registeredName != "" && state.registeredName == activeName
	if state.isActive && (state.previousVaultPath == "" || !configsvc.SameVaultPath(state.previousVaultPath, state.registeredPath)) {
		state.activated = true
		setInitSwitchBack(&state)
	}

	// First vault on the machine: pin as default.
	if state.isFirstVault && state.registeredName != "" {
		if !state.isDefault {
			if _, pinErr := configsvc.PinVault(opts, state.registeredName); pinErr != nil {
				return buildInitPostInitPayload(state), warnings, &initVaultSetupError{
					code: codes.ErrFileWrite,
					err:  fmt.Errorf("registered first vault but could not set it as default: %w", pinErr),
				}
			}
			state.isDefault = true
		}
	}

	// Every successfully initialized vault becomes active. For additional vaults,
	// preserve enough prior context to disclose the switch and provide an exact
	// restore command.
	if !state.isActive {
		if _, useErr := configsvc.UseVault(opts, state.registeredName); useErr != nil {
			return buildInitPostInitPayload(state), warnings, &initVaultSetupError{
				code: codes.ErrFileWrite,
				err:  fmt.Errorf("registered vault but could not set it active: %w", useErr),
			}
		}
		state.isActive = true
		state.activated = true
		setInitSwitchBack(&state)
	}

	return buildInitPostInitPayload(state), warnings, nil
}

func setInitSwitchBack(state *initPostInitState) {
	if state == nil || state.isFirstVault {
		return
	}
	if state.previousActiveName != "" && state.previousActivePath != "" {
		state.switchBack = "rvn --json vault use -- " + shellquote.Quote(state.previousActiveName)
		return
	}
	if state.previousVaultName == "" && state.previousDefaultName != "" && state.previousDefaultPath == "" {
		state.switchBack = "rvn --json config unset default_vault && rvn --json vault clear"
		return
	}
	state.switchBack = "rvn --json vault clear"
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
	needsDefaultChoice := alreadyRegistered && !s.isDefault && !s.isFirstVault

	nameForCommands := s.registeredName
	if nameForCommands == "" {
		nameForCommands = s.suggestedName
	}
	quotedPath := formatSuggestedCommandPath(s.cleanPath)
	quotedName := shellquote.Quote(nameForCommands)
	commands := map[string]interface{}{
		"register":          "rvn --json vault add -- " + quotedName + " " + quotedPath,
		"register_and_pin":  "rvn --json vault add --pin -- " + quotedName + " " + quotedPath,
		"activate":          "rvn --json vault use -- " + quotedName,
		"pin":               "rvn --json vault pin -- " + quotedName,
		"register_activate": "rvn --json vault add -- " + quotedName + " " + quotedPath + " && rvn --json vault use -- " + quotedName,
	}
	if s.switchBack != "" {
		commands["switch_back"] = s.switchBack
	}

	// Structured, invocable actions contain only choices that remain after init.
	// Activation is automatic; changing the default remains an explicit decision.
	actions := map[string]interface{}{}
	switch {
	case !alreadyRegistered:
		actions["register"] = map[string]interface{}{
			"command":     "vault add",
			"args":        map[string]interface{}{"name": s.suggestedName, "path": s.cleanPath},
			"description": "Register this vault in global config.",
		}
	default:
		if needsDefaultChoice {
			actions["set_default"] = map[string]interface{}{
				"command":     "vault pin",
				"args":        map[string]interface{}{"name": s.registeredName},
				"description": "Set this vault as the default vault. Ask the user first.",
			}
		}
	}

	nextSteps := make([]string, 0, 3)
	guidance := ""
	switch {
	case !alreadyRegistered:
		guidance = fmt.Sprintf("The new vault could not be registered and activated automatically. Repair global config/state access and rerun init. Until then, target it explicitly with --vault-path %s.", formatSuggestedCommandPath(s.cleanPath))
		nextSteps = append(nextSteps, "Register this vault globally: "+commands["register"].(string))
		nextSteps = append(nextSteps, "Register and set as default: "+commands["register_and_pin"].(string))
		nextSteps = append(nextSteps, "After registering, make it active: "+commands["activate"].(string))
	case s.isFirstVault && s.isActive:
		guidance = fmt.Sprintf("First vault on this machine: registered as %q and set as the default and active vault. It is ready to use now — no further vault setup is needed, and later commands resolve to it automatically.", s.registeredName)
	case s.isFirstVault:
		guidance = fmt.Sprintf("First vault %q was registered but could not be activated. Repair global state access and rerun init; until then, target it explicitly with --vault %s.", s.registeredName, shellquote.Quote(s.registeredName))
	case !s.isActive:
		guidance = fmt.Sprintf("Vault %q was registered at %s but could not be activated. Repair global state access and rerun init; until then, target it explicitly with --vault %s.", s.registeredName, s.registeredPath, shellquote.Quote(s.registeredName))
	case s.activated:
		guidance = fmt.Sprintf("Activated newly initialized vault %q at %s. Ambient CLI commands now target it.", s.registeredName, s.registeredPath)
		if s.previousActiveName != "" {
			guidance += fmt.Sprintf(" The previously active vault was %q at %s.", s.previousActiveName, s.previousActivePath)
		} else if s.previousVaultName != "" {
			guidance += fmt.Sprintf(" No active vault was set; ambient commands previously resolved to %q at %s via %s.", s.previousVaultName, s.previousVaultPath, s.previousVaultSource)
		}
		if s.switchBack != "" {
			guidance += " To switch back, run: " + s.switchBack
			nextSteps = append(nextSteps, "Switch back to the previous vault: "+s.switchBack)
		}
	default:
		guidance = fmt.Sprintf("Vault %q at %s was already registered and active; no activation switch was needed.", s.registeredName, s.registeredPath)
	}

	var activeVault interface{}
	if s.isActive {
		activeVault = initVaultInfoPayload(s.registeredName, s.registeredPath, "active_vault")
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
		"activated":                      s.activated,
		"active_vault":                   activeVault,
		"previous_active_vault":          initVaultInfoPayload(s.previousActiveName, s.previousActivePath, "active_vault"),
		"previous_vault":                 initVaultInfoPayload(s.previousVaultName, s.previousVaultPath, s.previousVaultSource),
		"switch_back":                    s.switchBack,
		"needs_user_choice_for_activate": false,
		"needs_user_choice_for_default":  needsDefaultChoice,
		"config_path":                    s.configPath,
		"state_path":                     s.statePath,
		"commands":                       commands,
		"actions":                        actions,
		"next_steps":                     nextSteps,
		"guidance":                       guidance,
	}
}

func initVaultInfoPayload(name, path, source string) interface{} {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(path) == "" {
		return nil
	}
	info := map[string]interface{}{
		"name": name,
		"path": path,
	}
	if strings.TrimSpace(source) != "" {
		info["source"] = source
	}
	return info
}

func formatSuggestedCommandPath(path string) string {
	displayPath := strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	return shellquote.Quote(displayPath)
}
