// Package initsvc coordinates vault creation and first-run global registration.
package initsvc

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/docsync"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/shellquote"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/svcerr"
)

type DocsResult struct {
	Fetched   bool   `json:"fetched"`
	FileCount int    `json:"file_count"`
	StorePath string `json:"store_path"`
}

type Warning struct {
	Code    codes.WarningCode
	Message string
}

type Result struct {
	Path           string
	Status         string
	CreatedConfig  bool
	CreatedSchema  bool
	GitignoreState string
	Docs           DocsResult
	PostInit       map[string]interface{}
	Warnings       []Warning
}

func (r *Result) Data() map[string]interface{} {
	// Preserve the existing command payload shape: docs was historically an
	// opaque struct in the canonical result and therefore encoded as {}.
	type docsEnvelope struct {
		fetched   bool
		fileCount int
		storePath string
	}
	return map[string]interface{}{
		"path": r.Path, "status": r.Status,
		"created_config": r.CreatedConfig, "created_schema": r.CreatedSchema,
		"gitignore_state": r.GitignoreState,
		"docs": docsEnvelope{
			fetched: r.Docs.Fetched, fileCount: r.Docs.FileCount, storePath: r.Docs.StorePath,
		},
		"post_init": r.PostInit,
	}
}

// Initialize creates the vault files and applies Raven's first-run registration
// and activation policy.
func Initialize(path, configPath, statePath, cliVersion string) (*Result, error) {
	result, err := initializeFiles(path, configPath, cliVersion)
	if err != nil {
		return nil, err
	}
	postInit, setupErr := setupVault(result.Path, configPath, statePath)
	result.PostInit = postInit
	if setupErr != nil {
		svcErr, ok := svcerr.AsError(setupErr)
		code := codes.ErrInternal
		if ok {
			code = svcErr.Code
		}
		return result, svcerr.Wrap(
			code,
			fmt.Sprintf("vault initialized at %s, but safe global vault setup could not be completed: %v", result.Path, setupErr),
			setupErr,
		).WithDetails(map[string]interface{}{
			"initialized": true, "path": result.Path, "post_init": postInit,
		}).WithSuggestion("Fix global config/state access and rerun init, or pass --vault/--vault-path explicitly on every vault-scoped command.")
	}
	return result, nil
}

type postInitState struct {
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

func setupVault(path, configPathOverride, statePathOverride string) (map[string]interface{}, error) {
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
	state := postInitState{
		cleanPath: cleanPath, suggestedName: suggestedName,
		configPath: config.ResolveConfigPath(configPathOverride),
	}
	state.statePath = config.ResolveStatePath(statePathOverride, state.configPath)
	opts := configsvc.ContextOptions{
		ConfigPathOverride: configPathOverride, StatePathOverride: statePathOverride,
	}
	ctx, err := configsvc.LoadVaultContext(opts)
	if ctx != nil {
		state.configPath = ctx.ConfigPath
		state.statePath = ctx.StatePath
	}
	if err != nil {
		return buildPostInitPayload(state), svcerr.Wrap(codes.ErrConfigInvalid, fmt.Sprintf("could not load global config/state: %v", err), err)
	}

	existingVaults := ctx.Cfg.ListVaults()
	defaultName := configsvc.DefaultVaultName(ctx.Cfg)
	state.previousDefaultName = defaultName
	if defaultPath, err := ctx.Cfg.GetDefaultVaultPath(); err == nil {
		state.previousDefaultPath = defaultPath
	}
	activeName := strings.TrimSpace(ctx.State.ActiveVault)
	if activeName != "" {
		if activePath, err := ctx.Cfg.GetVaultPath(activeName); err == nil {
			state.previousActiveName = activeName
			state.previousActivePath = activePath
		}
	}
	if current, err := configsvc.ResolveCurrentVault(ctx.Cfg, ctx.State); err == nil {
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
	state.isFirstVault = !state.hasExistingDefault && activeName == "" && !otherVaultExists

	if state.registeredName == "" {
		name := uniqueVaultName(existingVaults, suggestedName)
		addResult, err := configsvc.AddVault(configsvc.VaultAddRequest{
			ContextOptions: opts, Name: name, RawPath: cleanPath,
		})
		if err != nil {
			return buildPostInitPayload(state), svcerr.Wrap(codes.ErrFileWrite, fmt.Sprintf("could not auto-register vault: %v", err), err)
		}
		state.registeredName = addResult.Name
		state.registeredPath = addResult.Path
		state.registeredNow = true
	}

	state.isDefault = state.registeredName != "" && state.registeredName == defaultName
	state.isActive = state.registeredName != "" && state.registeredName == activeName
	if state.isActive && (state.previousVaultPath == "" || !configsvc.SameVaultPath(state.previousVaultPath, state.registeredPath)) {
		state.activated = true
		setSwitchBack(&state)
	}
	if state.isFirstVault && state.registeredName != "" && !state.isDefault {
		if _, err := configsvc.PinVault(opts, state.registeredName); err != nil {
			return buildPostInitPayload(state), svcerr.Wrap(codes.ErrFileWrite, fmt.Sprintf("registered first vault but could not set it as default: %v", err), err)
		}
		state.isDefault = true
	}
	if !state.isActive {
		if _, err := configsvc.UseVault(opts, state.registeredName); err != nil {
			return buildPostInitPayload(state), svcerr.Wrap(codes.ErrFileWrite, fmt.Sprintf("registered vault but could not set it active: %v", err), err)
		}
		state.isActive = true
		state.activated = true
		setSwitchBack(&state)
	}
	return buildPostInitPayload(state), nil
}

// SetupVault applies first-run registration to an already initialized vault.
// It is exported separately so callers can verify or recover the post-init step.
func SetupVault(path, configPathOverride, statePathOverride string) (map[string]interface{}, []Warning, error) {
	data, err := setupVault(path, configPathOverride, statePathOverride)
	return data, nil, err
}

func setSwitchBack(state *postInitState) {
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

func buildPostInitPayload(s postInitState) map[string]interface{} {
	if s.cleanPath == "" {
		return map[string]interface{}{}
	}
	alreadyRegistered := s.registeredName != ""
	needsDefaultChoice := alreadyRegistered && !s.isDefault && !s.isFirstVault
	nameForCommands := s.registeredName
	if nameForCommands == "" {
		nameForCommands = s.suggestedName
	}
	quotedPath := FormatSuggestedCommandPath(s.cleanPath)
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
	actions := map[string]interface{}{}
	if !alreadyRegistered {
		actions["register"] = map[string]interface{}{
			"command": "vault add", "args": map[string]interface{}{"name": s.suggestedName, "path": s.cleanPath},
			"description": "Register this vault in global config.",
		}
	} else if needsDefaultChoice {
		actions["set_default"] = map[string]interface{}{
			"command": "vault pin", "args": map[string]interface{}{"name": s.registeredName},
			"description": "Set this vault as the default vault. Ask the user first.",
		}
	}

	nextSteps := make([]string, 0, 3)
	guidance := ""
	switch {
	case !alreadyRegistered:
		guidance = fmt.Sprintf("The new vault could not be registered and activated automatically. Repair global config/state access and rerun init. Until then, target it explicitly with --vault-path %s.", FormatSuggestedCommandPath(s.cleanPath))
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
		activeVault = vaultInfoPayload(s.registeredName, s.registeredPath, "active_vault")
	}
	return map[string]interface{}{
		"suggested_name": s.suggestedName, "registered_name": s.registeredName,
		"already_registered": alreadyRegistered, "registered": s.registeredNow,
		"is_first_vault": s.isFirstVault, "has_existing_default": s.hasExistingDefault,
		"is_active": s.isActive, "is_default": s.isDefault, "activated": s.activated,
		"active_vault":          activeVault,
		"previous_active_vault": vaultInfoPayload(s.previousActiveName, s.previousActivePath, "active_vault"),
		"previous_vault":        vaultInfoPayload(s.previousVaultName, s.previousVaultPath, s.previousVaultSource),
		"switch_back":           s.switchBack, "needs_user_choice_for_activate": false,
		"needs_user_choice_for_default": needsDefaultChoice,
		"config_path":                   s.configPath, "state_path": s.statePath,
		"commands": commands, "actions": actions, "next_steps": nextSteps, "guidance": guidance,
	}
}

func vaultInfoPayload(name, path, source string) interface{} {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(path) == "" {
		return nil
	}
	info := map[string]interface{}{"name": name, "path": path}
	if strings.TrimSpace(source) != "" {
		info["source"] = source
	}
	return info
}

// FormatSuggestedCommandPath normalizes and quotes a vault path for guidance.
func FormatSuggestedCommandPath(path string) string {
	displayPath := strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	return shellquote.Quote(displayPath)
}

func initializeFiles(path, configPath, cliVersion string) (*Result, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "path is required").WithSuggestion("Usage: rvn init <path>")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create vault directory", err).
			WithSuggestion("Check that the destination path is writable")
	}
	if err := os.MkdirAll(filepath.Join(path, ".raven"), 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create .raven directory", err).
			WithSuggestion("Check that the destination path is writable")
	}

	gitignorePath := filepath.Join(path, ".gitignore")
	gitignoreState := "created"
	entries := []string{".raven/", ".trash/"}
	existingContent := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existingContent = string(data)
	}
	var missing []string
	for _, entry := range entries {
		if !strings.Contains(existingContent, entry) {
			missing = append(missing, entry)
		}
	}
	if len(missing) > 0 {
		var content string
		if existingContent == "" {
			content = "# Raven (auto-generated)\n# These are derived files - your markdown is the source of truth\n\n# Index database (rebuilt with 'rvn reindex')\n.raven/\n\n# Trashed files\n.trash/\n"
		} else {
			gitignoreState = "updated"
			addition := "\n# Raven\n" + strings.Join(missing, "\n") + "\n"
			content = strings.TrimRight(existingContent, "\n") + "\n" + addition
		}
		if err := os.WriteFile(gitignorePath, []byte(content), 0o644); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write .gitignore", err).
				WithSuggestion("Check write permissions for .gitignore")
		}
	} else if existingContent != "" {
		gitignoreState = "unchanged"
	}

	createdConfig, err := config.CreateDefaultVaultConfig(path)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create raven.yaml", err)
	}
	createdSchema, err := schema.CreateDefault(path)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create schema.yaml", err)
	}
	result := &Result{
		Path: path, CreatedConfig: createdConfig, CreatedSchema: createdSchema,
		GitignoreState: gitignoreState, Warnings: []Warning{},
	}
	if createdConfig || createdSchema {
		result.Status = "initialized"
	} else {
		result.Status = "existing"
	}
	fetchResult, err := docsync.Fetch(docsync.FetchOptions{
		ConfigPath: strings.TrimSpace(configPath), CLIVersion: strings.TrimSpace(cliVersion),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	})
	if err != nil {
		result.Warnings = append(result.Warnings, Warning{
			Code:    codes.WarnDocsFetchFailed,
			Message: fmt.Sprintf("Docs fetch failed: %v. Run 'rvn docs fetch' to retry.", err),
		})
	} else {
		result.Docs = DocsResult{Fetched: true, FileCount: fetchResult.FileCount, StorePath: fetchResult.DocsPath}
	}
	return result, nil
}
