package vaultconfigsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/paths"
)

type Code = codes.ErrorCode

const (
	CodeInvalidInput   Code = codes.ErrInvalidInput
	CodeConfigInvalid  Code = codes.ErrConfigInvalid
	CodeFileWriteError Code = codes.ErrFileWrite
	CodePrefixNotFound Code = codes.ErrPrefixNotFound
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
	Assets                AssetsInfo
	Capture               CaptureInfo
	Deletion              DeletionInfo
	QueriesCount          int
	ProtectedPrefixes     []string
	ProtectedPrefixesUsed bool
	Exclude               []string
	ExcludeUsed           bool
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

type AssetsInfo struct {
	Configured bool
	Root       string
	Kinds      map[string]AssetKindInfo
}

type AssetKindInfo struct {
	Extensions  []string
	MediaTypes  []string
	DefaultPath string
}

type GetAssetsRequest struct {
	VaultPath string
}

type GetAssetsResult struct {
	ConfigPath string
	Exists     bool
	Assets     AssetsInfo
}

type SetAssetsRequest struct {
	VaultPath string
	Root      *string
}

type SetAssetsResult struct {
	ConfigPath      string
	Created         bool
	Changed         bool
	Assets          AssetsInfo
	ReindexRequired bool
	ReindexCommand  string
}

type SetAssetKindRequest struct {
	VaultPath   string
	Kind        string
	Extensions  *[]string
	MediaTypes  *[]string
	DefaultPath *string
}

type SetAssetKindResult struct {
	ConfigPath      string
	Created         bool
	Changed         bool
	Kind            string
	Assets          AssetsInfo
	ReindexRequired bool
	ReindexCommand  string
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

type ListExcludeRequest struct {
	VaultPath string
}

type ListExcludeResult struct {
	ConfigPath string
	Exists     bool
	Exclude    []string
}

type AddExcludeRequest struct {
	VaultPath string
	Pattern   string
}

type AddExcludeResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
	Pattern    string
	Exclude    []string
}

type RemoveExcludeRequest struct {
	VaultPath string
	Pattern   string
}

type RemoveExcludeResult struct {
	ConfigPath string
	Changed    bool
	Removed    string
	Exclude    []string
}

func Show(req ShowRequest) (*ShowResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	protected := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	exclude := normalizedExcludePatterns(cfg.Exclude)
	directories := showDirectories(cfg)
	assets := showAssets(cfg)
	capture := cfg.GetCaptureConfig()
	deletion := cfg.GetDeletionConfig()
	return &ShowResult{
		ConfigPath:            configPath,
		Exists:                exists,
		AutoReindex:           cfg.IsAutoReindexEnabled(),
		AutoReindexExplicit:   cfg.AutoReindex != nil,
		DailyTemplate:         strings.TrimSpace(cfg.DailyTemplate),
		Directories:           directories,
		Assets:                assets,
		Capture:               CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
		Deletion:              DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
		QueriesCount:          len(cfg.Queries),
		ProtectedPrefixes:     protected,
		ProtectedPrefixesUsed: len(protected) > 0,
		Exclude:               exclude,
		ExcludeUsed:           len(exclude) > 0,
	}, nil
}

func GetAssets(req GetAssetsRequest) (*GetAssetsResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	return &GetAssetsResult{
		ConfigPath: configPath,
		Exists:     exists,
		Assets:     showAssets(cfg),
	}, nil
}

func SetAssets(req SetAssetsRequest) (*SetAssetsResult, error) {
	if req.Root == nil {
		return nil, newError(CodeInvalidInput, "specify at least one assets field", "Use --root", nil)
	}

	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalAssetsConfig(cfg)
	next := copyAssetsConfig(before)
	if next == nil {
		next = &config.AssetsConfig{}
	}

	if req.Root != nil {
		value, err := normalizeAssetRootValue(*req.Root)
		if err != nil {
			return nil, err
		}
		next.Root = value
	}

	next = compactAssetsConfig(next)
	changed := !assetsConfigEqual(before, next)
	if changed {
		cfg.Assets = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &SetAssetsResult{
		ConfigPath:      configPath,
		Created:         !exists && changed,
		Changed:         changed,
		Assets:          showAssets(cfg),
		ReindexRequired: changed,
		ReindexCommand:  "rvn reindex --json",
	}, nil
}

func SetAssetKind(req SetAssetKindRequest) (*SetAssetKindResult, error) {
	if req.Extensions == nil && req.MediaTypes == nil && req.DefaultPath == nil {
		return nil, newError(CodeInvalidInput, "specify at least one asset kind field", "Use --extensions, --media-types, or --default-path", nil)
	}

	kindName, err := normalizeAssetKindName(req.Kind)
	if err != nil {
		return nil, err
	}

	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	before := canonicalAssetsConfig(cfg)
	next := copyAssetsConfig(before)
	if next == nil {
		next = &config.AssetsConfig{}
	}
	if next.Kinds == nil {
		next.Kinds = make(map[string]*config.AssetKindConfig)
	}
	kind := copyAssetKindConfig(next.Kinds[kindName])
	if kind == nil {
		kind = &config.AssetKindConfig{}
	}

	if req.Extensions != nil {
		kind.Extensions = normalizeAssetExtensions(*req.Extensions)
	}
	if req.MediaTypes != nil {
		kind.MediaTypes = normalizeAssetMediaTypes(*req.MediaTypes)
	}
	if req.DefaultPath != nil {
		value, err := normalizeAssetDefaultPathValue(*req.DefaultPath)
		if err != nil {
			return nil, err
		}
		kind.DefaultPath = value
	}
	next.Kinds[kindName] = compactAssetKindConfig(kind)

	next = compactAssetsConfig(next)
	changed := !assetsConfigEqual(before, next)
	if changed {
		cfg.Assets = next
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &SetAssetKindResult{
		ConfigPath:      configPath,
		Created:         !exists && changed,
		Changed:         changed,
		Kind:            kindName,
		Assets:          showAssets(cfg),
		ReindexRequired: changed,
		ReindexCommand:  "rvn reindex --json",
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
		return nil, newError(CodeInvalidInput, "specify at least one directories field", "Use --daily, --type, --page, or --template", nil)
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
		value, err := normalizeDirValue(*req.Object, "type")
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
		return nil, newError(CodeInvalidInput, "specify at least one directories field to clear", "Use --daily, --type, --page, or --template", nil)
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

func ListExclude(req ListExcludeRequest) (*ListExcludeResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	return &ListExcludeResult{
		ConfigPath: configPath,
		Exists:     exists,
		Exclude:    normalizedExcludePatterns(cfg.Exclude),
	}, nil
}

func AddExclude(req AddExcludeRequest) (*AddExcludeResult, error) {
	cfg, exists, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	pattern, err := normalizeExcludePattern(req.Pattern)
	if err != nil {
		return nil, err
	}

	patterns := normalizedExcludePatterns(cfg.Exclude)
	changed := true
	for _, existing := range patterns {
		if existing == pattern {
			changed = false
			break
		}
	}
	if changed {
		patterns = append(patterns, pattern)
		cfg.Exclude = patterns
		if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
			return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
		}
	}

	return &AddExcludeResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
		Pattern:    pattern,
		Exclude:    patterns,
	}, nil
}

func RemoveExclude(req RemoveExcludeRequest) (*RemoveExcludeResult, error) {
	cfg, _, configPath, err := load(req.VaultPath)
	if err != nil {
		return nil, err
	}

	pattern, err := normalizeExcludePattern(req.Pattern)
	if err != nil {
		return nil, err
	}

	patterns := normalizedExcludePatterns(cfg.Exclude)
	next := make([]string, 0, len(patterns))
	found := false
	for _, existing := range patterns {
		if existing == pattern {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return nil, newError(CodePrefixNotFound, fmt.Sprintf("exclude pattern %q not found", pattern), "Run 'rvn vault config exclude list' to see configured patterns", nil)
	}

	cfg.Exclude = next
	if err := config.SaveVaultConfig(req.VaultPath, cfg); err != nil {
		return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
	}

	return &RemoveExcludeResult{
		ConfigPath: configPath,
		Changed:    true,
		Removed:    pattern,
		Exclude:    next,
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

func normalizedExcludePatterns(patterns []string) []string {
	return ravenignore.NormalizePatterns(patterns)
}

func normalizeExcludePattern(raw string) (string, error) {
	normalized := ravenignore.NormalizePatterns([]string{raw})
	if len(normalized) == 0 {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid exclude pattern: %q", raw), "Use a gitignore-style pattern such as 'AGENTS.md', '.cursor/', or '*.plan.md'", nil)
	}
	if _, err := ravenignore.NewMatcher(normalized); err != nil {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid exclude pattern: %q", raw), err.Error(), nil)
	}
	return normalized[0], nil
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

func showAssets(cfg *config.VaultConfig) AssetsInfo {
	assets := cfg.GetAssetsConfig()
	info := AssetsInfo{
		Configured: cfg.Assets != nil,
		Root:       assets.Root,
		Kinds:      make(map[string]AssetKindInfo, len(assets.Kinds)),
	}
	for name, kind := range assets.Kinds {
		if kind == nil {
			continue
		}
		info.Kinds[name] = AssetKindInfo{
			Extensions:  append([]string(nil), kind.Extensions...),
			MediaTypes:  append([]string(nil), kind.MediaTypes...),
			DefaultPath: kind.DefaultPath,
		}
	}
	return info
}

func canonicalAssetsConfig(cfg *config.VaultConfig) *config.AssetsConfig {
	if cfg.Assets == nil {
		return nil
	}

	assets := &config.AssetsConfig{
		Root:  paths.NormalizeDirRoot(cfg.Assets.Root),
		Kinds: make(map[string]*config.AssetKindConfig, len(cfg.Assets.Kinds)),
	}
	for name, kind := range cfg.Assets.Kinds {
		name = strings.TrimSpace(name)
		if name == "" || kind == nil {
			continue
		}
		assets.Kinds[name] = &config.AssetKindConfig{
			Extensions:  normalizeAssetExtensions(kind.Extensions),
			MediaTypes:  normalizeAssetMediaTypes(kind.MediaTypes),
			DefaultPath: normalizeAssetDefaultPath(kind.DefaultPath),
		}
	}
	return compactAssetsConfig(assets)
}

func copyAssetsConfig(cfg *config.AssetsConfig) *config.AssetsConfig {
	if cfg == nil {
		return nil
	}
	cloned := &config.AssetsConfig{
		Root: cfg.Root,
	}
	if len(cfg.Kinds) > 0 {
		cloned.Kinds = make(map[string]*config.AssetKindConfig, len(cfg.Kinds))
		for name, kind := range cfg.Kinds {
			cloned.Kinds[name] = copyAssetKindConfig(kind)
		}
	}
	return cloned
}

func copyAssetKindConfig(kind *config.AssetKindConfig) *config.AssetKindConfig {
	if kind == nil {
		return nil
	}
	return &config.AssetKindConfig{
		Extensions:  append([]string(nil), kind.Extensions...),
		MediaTypes:  append([]string(nil), kind.MediaTypes...),
		DefaultPath: kind.DefaultPath,
	}
}

func compactAssetsConfig(cfg *config.AssetsConfig) *config.AssetsConfig {
	if cfg == nil {
		return nil
	}
	if cfg.Root != "" {
		cfg.Root = paths.NormalizeDirRoot(cfg.Root)
	}
	for name, kind := range cfg.Kinds {
		if compactAssetKindConfig(kind) == nil {
			delete(cfg.Kinds, name)
		}
	}
	if len(cfg.Kinds) == 0 {
		cfg.Kinds = nil
	}
	if cfg.Root == "" && len(cfg.Kinds) == 0 {
		return nil
	}
	return cfg
}

func compactAssetKindConfig(kind *config.AssetKindConfig) *config.AssetKindConfig {
	if kind == nil {
		return nil
	}
	kind.Extensions = normalizeAssetExtensions(kind.Extensions)
	kind.MediaTypes = normalizeAssetMediaTypes(kind.MediaTypes)
	kind.DefaultPath = normalizeAssetDefaultPath(kind.DefaultPath)
	if len(kind.Extensions) == 0 && len(kind.MediaTypes) == 0 && kind.DefaultPath == "" {
		return nil
	}
	return kind
}

func assetsConfigEqual(a, b *config.AssetsConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Root != b.Root || len(a.Kinds) != len(b.Kinds) {
		return false
	}
	for name, aKind := range a.Kinds {
		bKind, ok := b.Kinds[name]
		if !ok || !assetKindConfigEqual(aKind, bKind) {
			return false
		}
	}
	return true
}

func assetKindConfigEqual(a, b *config.AssetKindConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return stringSlicesEqual(a.Extensions, b.Extensions) &&
		stringSlicesEqual(a.MediaTypes, b.MediaTypes) &&
		a.DefaultPath == b.DefaultPath
}

func canonicalDirectoriesConfig(cfg *config.VaultConfig) *config.DirectoriesConfig {
	if cfg.Directories == nil {
		return nil
	}

	daily := paths.NormalizeDirRoot(cfg.Directories.Daily)
	object := cfg.Directories.Object
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

func normalizeAssetRootValue(raw string) (string, error) {
	value := paths.NormalizeDirRoot(paths.NormalizeVaultRelPath(raw))
	if value == "" || !paths.IsValidVaultRelPath(value) {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid assets.root: %q", raw), "Use a vault-relative directory such as 'assets/'", nil)
	}
	return value, nil
}

func normalizeAssetKindName(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", newError(CodeInvalidInput, "asset kind is required", "Use a kind name such as 'image' or 'pdf'", nil)
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid asset kind: %q", raw), "Use lowercase letters, numbers, dashes, or underscores", nil)
	}
	return value, nil
}

func normalizeAssetExtensions(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range splitConfigList(values) {
		value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeAssetMediaTypes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range splitConfigList(values) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeAssetDefaultPathValue(raw string) (string, error) {
	value := normalizeAssetDefaultPath(raw)
	if value == "" && strings.TrimSpace(raw) != "" {
		return "", newError(CodeInvalidInput, fmt.Sprintf("invalid asset default_path: %q", raw), "Use a path relative to assets.root, such as 'images/'", nil)
	}
	return value, nil
}

func normalizeAssetDefaultPath(raw string) string {
	value := paths.NormalizeDirRoot(paths.NormalizeVaultRelPath(raw))
	if value == "" || !paths.IsValidVaultRelPath(value) {
		return ""
	}
	return value
}

func splitConfigList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.Split(value, ",")...)
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
