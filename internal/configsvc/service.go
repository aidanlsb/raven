package configsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
)

type Code string

const (
	CodeConfigInvalid      Code = "CONFIG_INVALID"
	CodeVaultNotFound      Code = "VAULT_NOT_FOUND"
	CodeVaultNotSpecified  Code = "VAULT_NOT_SPECIFIED"
	CodeFileNotFound       Code = "FILE_NOT_FOUND"
	CodeFileReadError      Code = "FILE_READ_ERROR"
	CodeFileWriteError     Code = "FILE_WRITE_ERROR"
	CodeInvalidInput       Code = "INVALID_INPUT"
	CodeMissingArgument    Code = "MISSING_ARGUMENT"
	CodeDuplicateName      Code = "DUPLICATE_NAME"
	CodeConfirmationNeeded Code = "CONFIRMATION_REQUIRED"
)

type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, msg string, err error) *Error {
	return &Error{
		Code:    code,
		Message: msg,
		Err:     err,
	}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if err == nil {
		return nil, false
	}
	if ok := errors.As(err, &svcErr); ok {
		return svcErr, true
	}
	return nil, false
}

type ContextOptions struct {
	ConfigPathOverride string
	StatePathOverride  string
}

type GlobalConfigContext struct {
	Cfg          *config.Config
	ConfigPath   string
	StatePath    string
	ConfigExists bool
}

func (ctx *GlobalConfigContext) Data() map[string]interface{} {
	if ctx == nil {
		return map[string]interface{}{}
	}

	vaults := make(map[string]string)
	for name, p := range ctx.Cfg.Vaults {
		vaults[name] = p
	}

	return map[string]interface{}{
		"config_path":   ctx.ConfigPath,
		"state_path":    ctx.StatePath,
		"exists":        ctx.ConfigExists,
		"default_vault": strings.TrimSpace(ctx.Cfg.DefaultVault),
		"state_file":    strings.TrimSpace(ctx.Cfg.StateFile),
		"vault":         strings.TrimSpace(ctx.Cfg.Vault),
		"vaults":        vaults,
		"editor":        strings.TrimSpace(ctx.Cfg.Editor),
		"editor_mode":   strings.TrimSpace(ctx.Cfg.EditorMode),
		"ui": map[string]interface{}{
			"accent":     strings.TrimSpace(ctx.Cfg.UI.Accent),
			"code_theme": strings.TrimSpace(ctx.Cfg.UI.CodeTheme),
		},
	}
}

func ShowContext(opts ContextOptions) (*GlobalConfigContext, error) {
	return loadGlobalConfigAllowMissing(opts)
}

type InitRequest struct {
	ConfigPathOverride string
}

type InitResult struct {
	ConfigPath string
	Created    bool
}

func Init(req InitRequest) (*InitResult, error) {
	targetPath := config.ResolveConfigPath(req.ConfigPathOverride)
	_, statErr := os.Stat(targetPath)
	existed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, newError(CodeFileReadError, "", statErr)
	}

	createdPath, err := config.CreateDefaultAt(targetPath)
	if err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}

	return &InitResult{
		ConfigPath: createdPath,
		Created:    !existed,
	}, nil
}

func NormalizeEditorMode(raw string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "auto", "terminal", "gui":
		return mode, true
	default:
		return "", false
	}
}

type SetRequest struct {
	ContextOptions
	Editor       *string
	EditorMode   *string
	StateFile    *string
	DefaultVault *string
	UIAccent     *string
	UICodeTheme  *string
}

type SetResult struct {
	Context *GlobalConfigContext
	Changed []string
}

func Set(req SetRequest) (*SetResult, error) {
	ctx, err := loadGlobalConfigAllowMissing(req.ContextOptions)
	if err != nil {
		return nil, err
	}

	changed := make([]string, 0, 6)

	if req.Editor != nil {
		value := strings.TrimSpace(*req.Editor)
		if value == "" {
			return nil, newError(CodeInvalidInput, "editor cannot be empty; use 'rvn config unset --editor' to clear it", nil)
		}
		ctx.Cfg.Editor = value
		changed = append(changed, "editor")
	}

	if req.EditorMode != nil {
		value, ok := NormalizeEditorMode(*req.EditorMode)
		if !ok {
			return nil, newError(CodeInvalidInput, "editor-mode must be one of: auto, terminal, gui", nil)
		}
		ctx.Cfg.EditorMode = value
		changed = append(changed, "editor_mode")
	}

	if req.StateFile != nil {
		value := strings.TrimSpace(*req.StateFile)
		if value == "" {
			return nil, newError(CodeInvalidInput, "state-file cannot be empty; use 'rvn config unset --state-file' to clear it", nil)
		}
		ctx.Cfg.StateFile = value
		changed = append(changed, "state_file")
	}

	if req.DefaultVault != nil {
		value := strings.TrimSpace(*req.DefaultVault)
		if value == "" {
			return nil, newError(CodeInvalidInput, "default-vault cannot be empty; use 'rvn config unset --default-vault' to clear it", nil)
		}
		if _, err := ctx.Cfg.GetVaultPath(value); err != nil {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("default-vault '%s' is not configured", value), nil)
		}
		ctx.Cfg.DefaultVault = value
		changed = append(changed, "default_vault")
	}

	if req.UIAccent != nil {
		value := strings.TrimSpace(*req.UIAccent)
		if value == "" {
			return nil, newError(CodeInvalidInput, "ui-accent cannot be empty; use 'rvn config unset --ui-accent' to clear it", nil)
		}
		ctx.Cfg.UI.Accent = value
		changed = append(changed, "ui.accent")
	}

	if req.UICodeTheme != nil {
		value := strings.TrimSpace(*req.UICodeTheme)
		if value == "" {
			return nil, newError(CodeInvalidInput, "ui-code-theme cannot be empty; use 'rvn config unset --ui-code-theme' to clear it", nil)
		}
		ctx.Cfg.UI.CodeTheme = value
		changed = append(changed, "ui.code_theme")
	}

	if len(changed) == 0 {
		return nil, newError(CodeMissingArgument, "no fields provided; set at least one --editor/--editor-mode/--state-file/--default-vault/--ui-accent/--ui-code-theme", nil)
	}

	if err := config.SaveTo(ctx.ConfigPath, ctx.Cfg); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}
	ctx.ConfigExists = true

	return &SetResult{
		Context: ctx,
		Changed: changed,
	}, nil
}

type UnsetRequest struct {
	ContextOptions
	Editor       bool
	EditorMode   bool
	StateFile    bool
	DefaultVault bool
	UIAccent     bool
	UICodeTheme  bool
}

type UnsetResult struct {
	Context *GlobalConfigContext
	Changed []string
}

func Unset(req UnsetRequest) (*UnsetResult, error) {
	ctx, err := loadGlobalConfigAllowMissing(req.ContextOptions)
	if err != nil {
		return nil, err
	}
	if !ctx.ConfigExists {
		return nil, newError(CodeFileNotFound, fmt.Sprintf("config file not found: %s", ctx.ConfigPath), nil)
	}

	changed := make([]string, 0, 6)
	if req.Editor {
		ctx.Cfg.Editor = ""
		changed = append(changed, "editor")
	}
	if req.EditorMode {
		ctx.Cfg.EditorMode = ""
		changed = append(changed, "editor_mode")
	}
	if req.StateFile {
		ctx.Cfg.StateFile = ""
		changed = append(changed, "state_file")
	}
	if req.DefaultVault {
		ctx.Cfg.DefaultVault = ""
		changed = append(changed, "default_vault")
	}
	if req.UIAccent {
		ctx.Cfg.UI.Accent = ""
		changed = append(changed, "ui.accent")
	}
	if req.UICodeTheme {
		ctx.Cfg.UI.CodeTheme = ""
		changed = append(changed, "ui.code_theme")
	}

	if len(changed) == 0 {
		return nil, newError(CodeMissingArgument, "no fields selected; pass one or more unset flags", nil)
	}

	if err := config.SaveTo(ctx.ConfigPath, ctx.Cfg); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}

	return &UnsetResult{
		Context: ctx,
		Changed: changed,
	}, nil
}

type VaultContext struct {
	Cfg        *config.Config
	State      *config.State
	ConfigPath string
	StatePath  string
}

type VaultRow struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"is_default"`
	IsActive  bool   `json:"is_active"`
}

type CurrentVaultInfo struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Source        string `json:"source"`
	ActiveMissing bool   `json:"active_missing"`
}

func LoadVaultContext(opts ContextOptions) (*VaultContext, error) {
	loadedCfg, resolvedConfigPath, err := loadGlobalConfigWithPath(opts.ConfigPathOverride)
	if err != nil {
		return nil, newError(CodeConfigInvalid, "", err)
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	resolvedStatePath := config.ResolveStatePath(opts.StatePathOverride, resolvedConfigPath, loadedCfg)
	state, err := config.LoadState(resolvedStatePath)
	if err != nil {
		return nil, newError(CodeConfigInvalid, "", err)
	}

	return &VaultContext{
		Cfg:        loadedCfg,
		State:      state,
		ConfigPath: resolvedConfigPath,
		StatePath:  resolvedStatePath,
	}, nil
}

func DefaultVaultName(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.DefaultVault) != "" {
		return strings.TrimSpace(cfg.DefaultVault)
	}
	if strings.TrimSpace(cfg.Vault) != "" && len(cfg.Vaults) == 0 {
		return "default"
	}
	return ""
}

func VaultRows(cfg *config.Config, state *config.State) ([]VaultRow, string, string, bool) {
	vaults := cfg.ListVaults()
	defaultName := DefaultVaultName(cfg)
	activeName := ""
	if state != nil {
		activeName = strings.TrimSpace(state.ActiveVault)
	}

	rows := make([]VaultRow, 0, len(vaults))
	names := make([]string, 0, len(vaults))
	for name := range vaults {
		names = append(names, name)
	}
	sort.Strings(names)

	activeMissing := activeName != ""
	for _, name := range names {
		rows = append(rows, VaultRow{
			Name:      name,
			Path:      vaults[name],
			IsDefault: name == defaultName,
			IsActive:  name == activeName,
		})
		if name == activeName {
			activeMissing = false
		}
	}

	return rows, defaultName, activeName, activeMissing
}

func ResolveCurrentVault(cfg *config.Config, state *config.State) (*CurrentVaultInfo, error) {
	activeName := ""
	if state != nil {
		activeName = strings.TrimSpace(state.ActiveVault)
	}

	if activeName != "" {
		p, err := cfg.GetVaultPath(activeName)
		if err == nil {
			return &CurrentVaultInfo{
				Name:   activeName,
				Path:   p,
				Source: "active_vault",
			}, nil
		}
	}

	defaultPath, err := cfg.GetDefaultVaultPath()
	if err != nil {
		if activeName != "" {
			return nil, newError(CodeVaultNotSpecified, fmt.Sprintf("active vault '%s' not found in config and no default vault configured", activeName), nil)
		}
		return nil, newError(CodeVaultNotSpecified, err.Error(), nil)
	}

	source := "default_vault"
	activeMissing := false
	if activeName != "" {
		source = "default_vault_fallback"
		activeMissing = true
	}

	return &CurrentVaultInfo{
		Name:          DefaultVaultName(cfg),
		Path:          defaultPath,
		Source:        source,
		ActiveMissing: activeMissing,
	}, nil
}

type VaultListResult struct {
	ConfigPath    string     `json:"config_path"`
	StatePath     string     `json:"state_path"`
	DefaultVault  string     `json:"default_vault"`
	ActiveVault   string     `json:"active_vault"`
	ActiveMissing bool       `json:"active_missing"`
	Vaults        []VaultRow `json:"vaults"`
}

func ListVaults(opts ContextOptions) (*VaultListResult, error) {
	ctx, err := LoadVaultContext(opts)
	if err != nil {
		return nil, err
	}
	rows, defaultName, activeName, activeMissing := VaultRows(ctx.Cfg, ctx.State)
	return &VaultListResult{
		ConfigPath:    ctx.ConfigPath,
		StatePath:     ctx.StatePath,
		DefaultVault:  defaultName,
		ActiveVault:   activeName,
		ActiveMissing: activeMissing,
		Vaults:        rows,
	}, nil
}

type VaultCurrentResult struct {
	Current     *CurrentVaultInfo `json:"current"`
	ConfigPath  string            `json:"config_path"`
	StatePath   string            `json:"state_path"`
	ActiveVault string            `json:"active_vault"`
}

func CurrentVault(opts ContextOptions) (*VaultCurrentResult, error) {
	ctx, err := LoadVaultContext(opts)
	if err != nil {
		return nil, err
	}
	current, err := ResolveCurrentVault(ctx.Cfg, ctx.State)
	if err != nil {
		return nil, err
	}
	active := ""
	if ctx.State != nil {
		active = strings.TrimSpace(ctx.State.ActiveVault)
	}
	return &VaultCurrentResult{
		Current:     current,
		ConfigPath:  ctx.ConfigPath,
		StatePath:   ctx.StatePath,
		ActiveVault: active,
	}, nil
}

type VaultUseResult struct {
	ActiveVault string `json:"active_vault"`
	Path        string `json:"path"`
	StatePath   string `json:"state_path"`
}

func UseVault(opts ContextOptions, name string) (*VaultUseResult, error) {
	name = strings.TrimSpace(name)
	ctx, err := LoadVaultContext(opts)
	if err != nil {
		return nil, err
	}

	p, err := ctx.Cfg.GetVaultPath(name)
	if err != nil {
		return nil, newError(CodeVaultNotFound, err.Error(), nil)
	}

	ctx.State.ActiveVault = name
	if err := config.SaveState(ctx.StatePath, ctx.State); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}

	return &VaultUseResult{
		ActiveVault: name,
		Path:        p,
		StatePath:   ctx.StatePath,
	}, nil
}

type VaultClearResult struct {
	Cleared   bool   `json:"cleared"`
	Previous  string `json:"previous"`
	StatePath string `json:"state_path"`
}

func ClearActiveVault(opts ContextOptions) (*VaultClearResult, error) {
	ctx, err := LoadVaultContext(opts)
	if err != nil {
		return nil, err
	}

	prev := strings.TrimSpace(ctx.State.ActiveVault)
	ctx.State.ActiveVault = ""
	if err := config.SaveState(ctx.StatePath, ctx.State); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}

	return &VaultClearResult{
		Cleared:   true,
		Previous:  prev,
		StatePath: ctx.StatePath,
	}, nil
}

type VaultPinResult struct {
	DefaultVault string `json:"default_vault"`
	Path         string `json:"path"`
	ConfigPath   string `json:"config_path"`
}

func PinVault(opts ContextOptions, name string) (*VaultPinResult, error) {
	name = strings.TrimSpace(name)
	ctx, err := LoadVaultContext(opts)
	if err != nil {
		return nil, err
	}

	p, err := ctx.Cfg.GetVaultPath(name)
	if err != nil {
		return nil, newError(CodeVaultNotFound, err.Error(), nil)
	}

	ctx.Cfg.DefaultVault = name
	if err := config.SaveTo(ctx.ConfigPath, ctx.Cfg); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}

	return &VaultPinResult{
		DefaultVault: name,
		Path:         p,
		ConfigPath:   ctx.ConfigPath,
	}, nil
}

type VaultAddRequest struct {
	ContextOptions
	Name    string
	RawPath string
	Replace bool
	Pin     bool
}

type VaultAddResult struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	ConfigPath   string `json:"config_path"`
	Replaced     bool   `json:"replaced"`
	PreviousPath string `json:"previous_path"`
	Pinned       bool   `json:"pinned"`
	DefaultVault string `json:"default_vault"`
}

func AddVault(req VaultAddRequest) (*VaultAddResult, error) {
	name := strings.TrimSpace(req.Name)
	rawPath := strings.TrimSpace(req.RawPath)
	if name == "" {
		return nil, newError(CodeMissingArgument, "vault name is required", nil)
	}
	if rawPath == "" {
		return nil, newError(CodeMissingArgument, "vault path is required", nil)
	}

	ctx, err := LoadVaultContext(req.ContextOptions)
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return nil, newError(CodeInvalidInput, fmt.Sprintf("failed to resolve vault path: %v", err), err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, newError(CodeFileNotFound, fmt.Sprintf("vault path does not exist: %s", absPath), nil)
		}
		return nil, newError(CodeFileReadError, "", err)
	}
	if !info.IsDir() {
		return nil, newError(CodeInvalidInput, fmt.Sprintf("vault path must be a directory: %s", absPath), nil)
	}

	if ctx.Cfg.Vaults == nil {
		ctx.Cfg.Vaults = make(map[string]string)
	}

	prevPath, existed := ctx.Cfg.Vaults[name]
	if existed && !req.Replace {
		return nil, newError(CodeDuplicateName, fmt.Sprintf("vault '%s' already exists", name), nil)
	}

	ctx.Cfg.Vaults[name] = absPath
	if req.Pin {
		ctx.Cfg.DefaultVault = name
	}

	if err := config.SaveTo(ctx.ConfigPath, ctx.Cfg); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}

	return &VaultAddResult{
		Name:         name,
		Path:         absPath,
		ConfigPath:   ctx.ConfigPath,
		Replaced:     existed,
		PreviousPath: prevPath,
		Pinned:       req.Pin,
		DefaultVault: ctx.Cfg.DefaultVault,
	}, nil
}

type VaultRemoveRequest struct {
	ContextOptions
	Name         string
	ClearDefault bool
	ClearActive  bool
}

type VaultRemoveResult struct {
	Name           string `json:"name"`
	RemovedPath    string `json:"removed_path"`
	RemovedLegacy  bool   `json:"removed_legacy"`
	DefaultCleared bool   `json:"default_cleared"`
	ActiveCleared  bool   `json:"active_cleared"`
	ConfigPath     string `json:"config_path"`
	StatePath      string `json:"state_path"`
}

func RemoveVault(req VaultRemoveRequest) (*VaultRemoveResult, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, newError(CodeMissingArgument, "vault name is required", nil)
	}

	ctx, err := LoadVaultContext(req.ContextOptions)
	if err != nil {
		return nil, err
	}

	activeName := strings.TrimSpace(ctx.State.ActiveVault)
	defaultName := DefaultVaultName(ctx.Cfg)
	removingActive := activeName != "" && name == activeName
	removingDefault := defaultName != "" && name == defaultName

	if removingDefault && !req.ClearDefault {
		return nil, newError(CodeConfirmationNeeded, fmt.Sprintf("vault '%s' is the current default vault", name), nil)
	}
	if removingActive && !req.ClearActive {
		return nil, newError(CodeConfirmationNeeded, fmt.Sprintf("vault '%s' is the current active vault", name), nil)
	}

	removedPath := ""
	removedLegacy := false
	if ctx.Cfg.Vaults != nil {
		if p, ok := ctx.Cfg.Vaults[name]; ok {
			removedPath = p
			delete(ctx.Cfg.Vaults, name)
		}
	}
	if removedPath == "" && name == "default" && strings.TrimSpace(ctx.Cfg.Vault) != "" && len(ctx.Cfg.Vaults) == 0 {
		removedPath = strings.TrimSpace(ctx.Cfg.Vault)
		ctx.Cfg.Vault = ""
		removedLegacy = true
	}
	if removedPath == "" {
		return nil, newError(CodeVaultNotFound, fmt.Sprintf("vault '%s' not found in config", name), nil)
	}

	defaultCleared := false
	if removingDefault && req.ClearDefault {
		if strings.TrimSpace(ctx.Cfg.DefaultVault) == name {
			ctx.Cfg.DefaultVault = ""
		}
		defaultCleared = true
	}

	activeCleared := false
	if removingActive && req.ClearActive {
		ctx.State.ActiveVault = ""
		activeCleared = true
	}

	if err := config.SaveTo(ctx.ConfigPath, ctx.Cfg); err != nil {
		return nil, newError(CodeFileWriteError, "", err)
	}
	if activeCleared {
		if err := config.SaveState(ctx.StatePath, ctx.State); err != nil {
			return nil, newError(CodeFileWriteError, "", err)
		}
	}

	return &VaultRemoveResult{
		Name:           name,
		RemovedPath:    removedPath,
		RemovedLegacy:  removedLegacy,
		DefaultCleared: defaultCleared,
		ActiveCleared:  activeCleared,
		ConfigPath:     ctx.ConfigPath,
		StatePath:      ctx.StatePath,
	}, nil
}

func loadGlobalConfigWithPath(configPathOverride string) (*config.Config, string, error) {
	resolvedPath := config.ResolveConfigPath(configPathOverride)

	var loadedCfg *config.Config
	var err error
	if strings.TrimSpace(configPathOverride) != "" {
		if _, statErr := os.Stat(resolvedPath); statErr != nil {
			if os.IsNotExist(statErr) {
				return &config.Config{}, resolvedPath, nil
			}
			return nil, "", statErr
		}
		loadedCfg, err = config.LoadFrom(configPathOverride)
	} else {
		loadedCfg, err = config.Load()
	}
	if err != nil {
		return nil, "", err
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	return loadedCfg, resolvedPath, nil
}

func loadGlobalConfigAllowMissing(opts ContextOptions) (*GlobalConfigContext, error) {
	resolvedPath := config.ResolveConfigPath(opts.ConfigPathOverride)

	if strings.TrimSpace(opts.ConfigPathOverride) == "" {
		loadedCfg, err := config.Load()
		if err != nil {
			return nil, newError(CodeConfigInvalid, "", err)
		}
		if loadedCfg == nil {
			loadedCfg = &config.Config{}
		}
		_, statErr := os.Stat(resolvedPath)
		return &GlobalConfigContext{
			Cfg:          loadedCfg,
			ConfigPath:   resolvedPath,
			StatePath:    config.ResolveStatePath(opts.StatePathOverride, resolvedPath, loadedCfg),
			ConfigExists: statErr == nil,
		}, nil
	}

	_, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			loadedCfg := &config.Config{}
			return &GlobalConfigContext{
				Cfg:          loadedCfg,
				ConfigPath:   resolvedPath,
				StatePath:    config.ResolveStatePath(opts.StatePathOverride, resolvedPath, loadedCfg),
				ConfigExists: false,
			}, nil
		}
		return nil, newError(CodeFileReadError, "", err)
	}

	loadedCfg, err := config.LoadFrom(resolvedPath)
	if err != nil {
		return nil, newError(CodeConfigInvalid, "", err)
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	return &GlobalConfigContext{
		Cfg:          loadedCfg,
		ConfigPath:   resolvedPath,
		StatePath:    config.ResolveStatePath(opts.StatePathOverride, resolvedPath, loadedCfg),
		ConfigExists: true,
	}, nil
}
