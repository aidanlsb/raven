package vaultconfigsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

type Code string

const (
	CodeInvalidInput   Code = "INVALID_INPUT"
	CodeConfigInvalid  Code = "CONFIG_INVALID"
	CodeFileWriteError Code = "FILE_WRITE_ERROR"
	CodePrefixNotFound Code = "PREFIX_NOT_FOUND"
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Err        error
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

func newError(code Code, message, suggestion string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type ShowRequest struct {
	VaultPath string
}

type ShowResult struct {
	ConfigPath            string
	Exists                bool
	AutoReindex           bool
	AutoReindexExplicit   bool
	DailyTemplate         string
	Directories           DirectoriesInfo
	Capture               CaptureInfo
	Deletion              DeletionInfo
	QueriesCount          int
	ProtectedPrefixes     []string
	ProtectedPrefixesUsed bool
}

type DirectoriesInfo struct {
	Configured bool
	Daily      string
	Object     string
	Page       string
	Template   string
}

type CaptureInfo struct {
	Destination string
	Heading     string
}

type DeletionInfo struct {
	Behavior string
	TrashDir string
}

type GetDirectoriesRequest struct {
	VaultPath string
}

type GetDirectoriesResult struct {
	ConfigPath  string
	Exists      bool
	Directories DirectoriesInfo
}

type SetDirectoriesRequest struct {
	VaultPath string
	Daily     *string
	Object    *string
	Page      *string
	Template  *string
}

type SetDirectoriesResult struct {
	ConfigPath  string
	Created     bool
	Changed     bool
	Directories DirectoriesInfo
}

type UnsetDirectoriesRequest struct {
	VaultPath string
	Daily     bool
	Object    bool
	Page      bool
	Template  bool
}

type UnsetDirectoriesResult struct {
	ConfigPath  string
	Changed     bool
	Directories DirectoriesInfo
}

type GetCaptureRequest struct {
	VaultPath string
}

type GetCaptureResult struct {
	ConfigPath string
	Exists     bool
	Configured bool
	Capture    CaptureInfo
}

type SetCaptureRequest struct {
	VaultPath   string
	Destination *string
	Heading     *string
}

type SetCaptureResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
	Configured bool
	Capture    CaptureInfo
}

type UnsetCaptureRequest struct {
	VaultPath   string
	Destination bool
	Heading     bool
}

type UnsetCaptureResult struct {
	ConfigPath string
	Changed    bool
	Configured bool
	Capture    CaptureInfo
}

type GetDeletionRequest struct {
	VaultPath string
}

type GetDeletionResult struct {
	ConfigPath string
	Exists     bool
	Configured bool
	Deletion   DeletionInfo
}

type SetDeletionRequest struct {
	VaultPath string
	Behavior  *string
	TrashDir  *string
}

type SetDeletionResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
	Configured bool
	Deletion   DeletionInfo
}

type UnsetDeletionRequest struct {
	VaultPath string
	Behavior  bool
	TrashDir  bool
}

type UnsetDeletionResult struct {
	ConfigPath string
	Changed    bool
	Configured bool
	Deletion   DeletionInfo
}

type SetAutoReindexRequest struct {
	VaultPath string
	Value     bool
}

type SetAutoReindexResult struct {
	ConfigPath          string
	Created             bool
	Changed             bool
	AutoReindex         bool
	AutoReindexExplicit bool
}

type UnsetAutoReindexRequest struct {
	VaultPath string
}

type UnsetAutoReindexResult struct {
	ConfigPath          string
	Changed             bool
	AutoReindex         bool
	AutoReindexExplicit bool
}

type ListProtectedPrefixesRequest struct {
	VaultPath string
}

type ListProtectedPrefixesResult struct {
	ConfigPath        string
	Exists            bool
	ProtectedPrefixes []string
}

type AddProtectedPrefixRequest struct {
	VaultPath string
	Prefix    string
}

type AddProtectedPrefixResult struct {
	ConfigPath        string
	Created           bool
	Changed           bool
	Prefix            string
	ProtectedPrefixes []string
}

type RemoveProtectedPrefixRequest struct {
	VaultPath string
	Prefix    string
}

type RemoveProtectedPrefixResult struct {
	ConfigPath        string
	Changed           bool
	Removed           string
	ProtectedPrefixes []string
}

func Show(req ShowRequest) (*ShowResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	protected := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	directories := showDirectories(cfg)
	capture := cfg.GetCaptureConfig()
	deletion := cfg.GetDeletionConfig()
	return &ShowResult{
		ConfigPath:            configPath,
		Exists:                exists,
		AutoReindex:           cfg.IsAutoReindexEnabled(),
		AutoReindexExplicit:   cfg.AutoReindex != nil,
		DailyTemplate:         strings.TrimSpace(cfg.DailyTemplate),
		Directories:           directories,
		Capture:               CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
		Deletion:              DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
		QueriesCount:          len(cfg.Queries),
		ProtectedPrefixes:     protected,
		ProtectedPrefixesUsed: len(protected) > 0,
	}, nil
}

func GetDirectories(req GetDirectoriesRequest) (*GetDirectoriesResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	return &GetDirectoriesResult{
		ConfigPath:  configPath,
		Exists:      exists,
		Directories: showDirectories(cfg),
	}, nil
}

func SetDirectories(req SetDirectoriesRequest) (*SetDirectoriesResult, error) {
	if req.Daily == nil && req.Object == nil && req.Page == nil && req.Template == nil {
		return nil, newError(CodeInvalidInput, "specify at least one directories field", "Use --daily, --object, --page, or --template", nil)
	}

	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalDirectoriesConfig(cfg)
	next := copyDirectoriesConfig(before)
	if next == nil {
		next = &config.DirectoriesConfig{}
	}

	if req.Daily != nil {
		value, err := normalizeDirValue(*req.Daily, "daily")
		if err != nil {
			return nil, err
		}
		next.Daily = value
	}
	if req.Object != nil {
		value, err := normalizeDirValue(*req.Object, "object")
		if err != nil {
			return nil, err
		}
		next.Object = value
	}
	if req.Page != nil {
		value, err := normalizeDirValue(*req.Page, "page")
		if err != nil {
			return nil, err
		}
		next.Page = value
	}
	if req.Template != nil {
		value, err := normalizeDirValue(*req.Template, "template")
		if err != nil {
			return nil, err
		}
		next.Template = value
	}

	next = compactDirectoriesConfig(next)
	changed := !directoriesConfigEqual(before, next)
	if changed {
		cfg.Directories = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &SetDirectoriesResult{
		ConfigPath:  configPath,
		Created:     !exists && changed,
		Changed:     changed,
		Directories: showDirectories(cfg),
	}, nil
}

func UnsetDirectories(req UnsetDirectoriesRequest) (*UnsetDirectoriesResult, error) {
	if !req.Daily && !req.Object && !req.Page && !req.Template {
		return nil, newError(CodeInvalidInput, "specify at least one directories field to clear", "Use --daily, --object, --page, or --template", nil)
	}

	cfg, _, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalDirectoriesConfig(cfg)
	next := copyDirectoriesConfig(before)
	if next == nil {
		next = &config.DirectoriesConfig{}
	}

	if req.Daily {
		next.Daily = ""
	}
	if req.Object {
		next.Object = ""
	}
	if req.Page {
		next.Page = ""
	}
	if req.Template {
		next.Template = ""
	}

	next = compactDirectoriesConfig(next)
	changed := !directoriesConfigEqual(before, next)
	if changed {
		cfg.Directories = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &UnsetDirectoriesResult{
		ConfigPath:  configPath,
		Changed:     changed,
		Directories: showDirectories(cfg),
	}, nil
}

func GetCapture(req GetCaptureRequest) (*GetCaptureResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	capture := cfg.GetCaptureConfig()
	return &GetCaptureResult{
		ConfigPath: configPath,
		Exists:     exists,
		Configured: cfg.Capture != nil,
		Capture:    CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
	}, nil
}

func SetCapture(req SetCaptureRequest) (*SetCaptureResult, error) {
	if req.Destination == nil && req.Heading == nil {
		return nil, newError(CodeInvalidInput, "specify at least one capture field", "Use --destination or --heading", nil)
	}

	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalCaptureConfig(cfg)
	next := copyCaptureConfig(before)
	if next == nil {
		next = &config.CaptureConfig{}
	}

	if req.Destination != nil {
		value, err := normalizeCaptureDestination(*req.Destination)
		if err != nil {
			return nil, err
		}
		next.Destination = value
	}
	if req.Heading != nil {
		value, err := normalizeHeading(*req.Heading)
		if err != nil {
			return nil, err
		}
		next.Heading = value
	}

	next = compactCaptureConfig(next)
	changed := !captureConfigEqual(before, next)
	if changed {
		cfg.Capture = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	capture := cfg.GetCaptureConfig()
	return &SetCaptureResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
		Configured: cfg.Capture != nil,
		Capture:    CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
	}, nil
}

func UnsetCapture(req UnsetCaptureRequest) (*UnsetCaptureResult, error) {
	if !req.Destination && !req.Heading {
		return nil, newError(CodeInvalidInput, "specify at least one capture field to clear", "Use --destination or --heading", nil)
	}

	cfg, _, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalCaptureConfig(cfg)
	next := copyCaptureConfig(before)
	if next == nil {
		next = &config.CaptureConfig{}
	}

	if req.Destination {
		next.Destination = ""
	}
	if req.Heading {
		next.Heading = ""
	}

	next = compactCaptureConfig(next)
	changed := !captureConfigEqual(before, next)
	if changed {
		cfg.Capture = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	capture := cfg.GetCaptureConfig()
	return &UnsetCaptureResult{
		ConfigPath: configPath,
		Changed:    changed,
		Configured: cfg.Capture != nil,
		Capture:    CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
	}, nil
}

func GetDeletion(req GetDeletionRequest) (*GetDeletionResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	deletion := cfg.GetDeletionConfig()
	return &GetDeletionResult{
		ConfigPath: configPath,
		Exists:     exists,
		Configured: cfg.Deletion != nil,
		Deletion:   DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
	}, nil
}

func SetDeletion(req SetDeletionRequest) (*SetDeletionResult, error) {
	if req.Behavior == nil && req.TrashDir == nil {
		return nil, newError(CodeInvalidInput, "specify at least one deletion field", "Use --behavior or --trash-dir", nil)
	}

	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalDeletionConfig(cfg)
	next := copyDeletionConfig(before)
	if next == nil {
		next = &config.DeletionConfig{}
	}

	if req.Behavior != nil {
		value, err := normalizeDeletionBehavior(*req.Behavior)
		if err != nil {
			return nil, err
		}
		next.Behavior = value
	}
	if req.TrashDir != nil {
		value, err := normalizeTrashDir(*req.TrashDir)
		if err != nil {
			return nil, err
		}
		next.TrashDir = value
	}

	next = compactDeletionConfig(next)
	changed := !deletionConfigEqual(before, next)
	if changed {
		cfg.Deletion = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	deletion := cfg.GetDeletionConfig()
	return &SetDeletionResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
		Configured: cfg.Deletion != nil,
		Deletion:   DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
	}, nil
}

func UnsetDeletion(req UnsetDeletionRequest) (*UnsetDeletionResult, error) {
	if !req.Behavior && !req.TrashDir {
		return nil, newError(CodeInvalidInput, "specify at least one deletion field to clear", "Use --behavior or --trash-dir", nil)
	}

	cfg, _, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalDeletionConfig(cfg)
	next := copyDeletionConfig(before)
	if next == nil {
		next = &config.DeletionConfig{}
	}

	if req.Behavior {
		next.Behavior = ""
	}
	if req.TrashDir {
		next.TrashDir = ""
	}

	next = compactDeletionConfig(next)
	changed := !deletionConfigEqual(before, next)
	if changed {
		cfg.Deletion = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	deletion := cfg.GetDeletionConfig()
	return &UnsetDeletionResult{
		ConfigPath: configPath,
		Changed:    changed,
		Configured: cfg.Deletion != nil,
		Deletion:   DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
	}, nil
}

func SetAutoReindex(req SetAutoReindexRequest) (*SetAutoReindexResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	changed := cfg.AutoReindex == nil || *cfg.AutoReindex != req.Value
	if changed {
		value := req.Value
		cfg.AutoReindex = &value
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &SetAutoReindexResult{
		ConfigPath:          configPath,
		Created:             !exists && changed,
		Changed:             changed,
		AutoReindex:         req.Value,
		AutoReindexExplicit: true,
	}, nil
}

func UnsetAutoReindex(req UnsetAutoReindexRequest) (*UnsetAutoReindexResult, error) {
	cfg, _, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	changed := cfg.AutoReindex != nil
	if changed {
		cfg.AutoReindex = nil
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &UnsetAutoReindexResult{
		ConfigPath:          configPath,
		Changed:             changed,
		AutoReindex:         cfg.IsAutoReindexEnabled(),
		AutoReindexExplicit: false,
	}, nil
}

func ListProtectedPrefixes(req ListProtectedPrefixesRequest) (*ListProtectedPrefixesResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	return &ListProtectedPrefixesResult{
		ConfigPath:        configPath,
		Exists:            exists,
		ProtectedPrefixes: normalizedProtectedPrefixes(cfg.ProtectedPrefixes),
	}, nil
}

func AddProtectedPrefix(req AddProtectedPrefixRequest) (*AddProtectedPrefixResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	prefix, err := normalizeProtectedPrefix(req.Prefix)
	if err != nil {
		return nil, err
	}

	prefixes := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	changed := true
	for _, existing := range prefixes {
		if existing == prefix {
			changed = false
			break
		}
	}
	if changed {
		prefixes = append(prefixes, prefix)
		sort.Strings(prefixes)
		cfg.ProtectedPrefixes = prefixes
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &AddProtectedPrefixResult{
		ConfigPath:        configPath,
		Created:           !exists && changed,
		Changed:           changed,
		Prefix:            prefix,
		ProtectedPrefixes: prefixes,
	}, nil
}

func RemoveProtectedPrefix(req RemoveProtectedPrefixRequest) (*RemoveProtectedPrefixResult, error) {
	cfg, _, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	prefix, err := normalizeProtectedPrefix(req.Prefix)
	if err != nil {
		return nil, err
	}

	prefixes := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	next := make([]string, 0, len(prefixes))
	found := false
	for _, existing := range prefixes {
		if existing == prefix {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return nil, newError(CodePrefixNotFound, fmt.Sprintf("protected prefix '%s' not found", prefix), "Run 'rvn vault config protected-prefixes list' to see configured prefixes", nil)
	}

	cfg.ProtectedPrefixes = next
	if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
		return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
	}

	return &RemoveProtectedPrefixResult{
		ConfigPath:        configPath,
		Changed:           true,
		Removed:           prefix,
		ProtectedPrefixes: next,
	}, nil
}

func load(vaultPath string) (*config.VaultConfig, bool, string, error) {
	if strings.TrimSpace(vaultPath) == "" {
		return nil, false, "", newError(CodeInvalidInput, "vault path is required", "Resolve a vault before invoking the command", nil)
	}

	configPath := filepath.Join(vaultPath, "raven.yaml")
	_, statErr := os.Stat(configPath)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, false, configPath, newError(CodeConfigInvalid, "failed to stat vault config", "Check raven.yaml permissions and try again", statErr)
	}

	cfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, exists, configPath, newError(CodeConfigInvalid, "failed to load vault config", "Fix raven.yaml and try again", err)
	}
	return cfg, exists, configPath, nil
}

func normalizedProtectedPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(prefixes))
	out := make([]string, 0, len(prefixes))
	for _, raw := range prefixes {
		prefix, err := normalizeProtectedPrefix(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		out = append(out, prefix)
	}
	sort.Strings(out)
	return out
}

func normalizeProtectedPrefix(raw string) (string, error) {
	normalized := paths.NormalizeDirRoot(paths.NormalizeVaultRelPath(raw))
	if normalized == "" || !paths.IsValidVaultRelPath(normalized) {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid protected prefix: %q", raw), "Use a vault-relative directory prefix such as 'private/'", nil)
	}
	return normalized, nil
}

func showDirectories(cfg *config.VaultConfig) DirectoriesInfo {
	dirs := cfg.GetDirectoriesConfig()
	info := DirectoriesInfo{
		Configured: dirs != nil,
		Daily:      paths.NormalizeDirRoot(cfg.GetDailyDirectory()),
		Template:   cfg.GetTemplateDirectory(),
	}
	if dirs != nil {
		info.Object = dirs.Object
		info.Page = dirs.Page
		if dirs.Template != "" {
			info.Template = dirs.Template
		}
	}
	return info
}

func canonicalDirectoriesConfig(cfg *config.VaultConfig) *config.DirectoriesConfig {
	if cfg.Directories == nil {
		return nil
	}

	daily := paths.NormalizeDirRoot(cfg.Directories.Daily)
	object := cfg.Directories.Object
	if object == "" {
		//nolint:staticcheck // Backward-compatible read of deprecated config key.
		object = cfg.Directories.Objects
	}
	object = paths.NormalizeDirRoot(object)

	page := cfg.Directories.Page
	if page == "" {
		//nolint:staticcheck // Backward-compatible read of deprecated config key.
		page = cfg.Directories.Pages
	}
	page = paths.NormalizeDirRoot(page)

	template := cfg.Directories.Template
	if template == "" {
		//nolint:staticcheck // Backward-compatible read of deprecated config key.
		template = cfg.Directories.Templates
	}
	template = paths.NormalizeDirRoot(template)

	return &config.DirectoriesConfig{
		Daily:    daily,
		Object:   object,
		Page:     page,
		Template: template,
	}
}

func copyDirectoriesConfig(cfg *config.DirectoriesConfig) *config.DirectoriesConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func compactDirectoriesConfig(cfg *config.DirectoriesConfig) *config.DirectoriesConfig {
	if cfg == nil {
		return nil
	}
	//nolint:staticcheck // Clearing deprecated aliases keeps saved config canonical.
	cfg.Objects = ""
	//nolint:staticcheck // Clearing deprecated aliases keeps saved config canonical.
	cfg.Pages = ""
	//nolint:staticcheck // Clearing deprecated aliases keeps saved config canonical.
	cfg.Templates = ""
	if cfg.Daily == "" && cfg.Object == "" && cfg.Page == "" && cfg.Template == "" {
		return nil
	}
	return cfg
}

func directoriesConfigEqual(a, b *config.DirectoriesConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Daily == b.Daily &&
		a.Object == b.Object &&
		a.Page == b.Page &&
		a.Template == b.Template
}

func canonicalCaptureConfig(cfg *config.VaultConfig) *config.CaptureConfig {
	if cfg.Capture == nil {
		return nil
	}
	return &config.CaptureConfig{
		Destination: cfg.Capture.Destination,
		Heading:     cfg.Capture.Heading,
	}
}

func copyCaptureConfig(cfg *config.CaptureConfig) *config.CaptureConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func compactCaptureConfig(cfg *config.CaptureConfig) *config.CaptureConfig {
	if cfg == nil {
		return nil
	}
	if cfg.Destination == "" && cfg.Heading == "" {
		return nil
	}
	return cfg
}

func captureConfigEqual(a, b *config.CaptureConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Destination == b.Destination && a.Heading == b.Heading
}

func canonicalDeletionConfig(cfg *config.VaultConfig) *config.DeletionConfig {
	if cfg.Deletion == nil {
		return nil
	}
	return &config.DeletionConfig{
		Behavior: cfg.Deletion.Behavior,
		TrashDir: cfg.Deletion.TrashDir,
	}
}

func copyDeletionConfig(cfg *config.DeletionConfig) *config.DeletionConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func compactDeletionConfig(cfg *config.DeletionConfig) *config.DeletionConfig {
	if cfg == nil {
		return nil
	}
	if cfg.Behavior == "" && cfg.TrashDir == "" {
		return nil
	}
	return cfg
}

func deletionConfigEqual(a, b *config.DeletionConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Behavior == b.Behavior && a.TrashDir == b.TrashDir
}

func normalizeDirValue(raw, field string) (string, error) {
	normalized := paths.NormalizeDirRoot(paths.NormalizeVaultRelPath(raw))
	if normalized == "" || !paths.IsValidVaultRelPath(normalized) {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid directories.%s: %q", field, raw), "Use a vault-relative directory such as 'daily/'", nil)
	}
	return normalized, nil
}

func normalizeCaptureDestination(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", newError(CodeInvalidInput, "capture destination cannot be empty", "Use 'daily' or a vault-relative file path", nil)
	}
	if value == "daily" {
		return "daily", nil
	}
	value = paths.NormalizeVaultRelPath(value)
	if !paths.IsValidVaultRelPath(value) {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid capture destination: %q", raw), "Use 'daily' or a vault-relative file path such as 'inbox.md'", nil)
	}
	return value, nil
}

func normalizeHeading(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", newError(CodeInvalidInput, "heading cannot be empty", "Use a heading such as '## Captured'", nil)
	}
	return value, nil
}

func normalizeDeletionBehavior(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch value {
	case "trash", "permanent":
		return value, nil
	default:
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid deletion behavior: %q", raw), "Use 'trash' or 'permanent'", nil)
	}
}

func normalizeTrashDir(raw string) (string, error) {
	value := paths.NormalizeVaultRelPath(raw)
	if value == "" || !paths.IsValidVaultRelPath(value) {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid trash_dir: %q", raw), "Use a vault-relative path such as '.trash' or 'archive/trash'", nil)
	}
	return value, nil
}
