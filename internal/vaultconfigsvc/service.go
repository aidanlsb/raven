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
