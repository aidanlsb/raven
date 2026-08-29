package vaultconfigsvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// DirectoriesInfo is a view of the effective directories configuration.
type DirectoriesInfo struct {
	Configured bool
	Daily      string
	Object     string
	Page       string
	Template   string
}

// CaptureInfo is a view of the effective capture configuration.
type CaptureInfo struct {
	Destination string
	Heading     string
}

// DeletionInfo is a view of the effective deletion configuration.
type DeletionInfo struct {
	Behavior string
	TrashDir string
}

// ShowResult contains the full vault configuration summary.
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
	Exclude               []string
	ExcludeUsed           bool
}

// MutationResult contains common fields for all mutation operations.
type MutationResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
}

// Show returns the complete vault configuration summary.
func Show(rt *vaultruntime.Runtime) (*ShowResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}

	protected := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	exclude := normalizedExcludePatterns(cfg.Exclude)
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
		Exclude:               exclude,
		ExcludeUsed:           len(exclude) > 0,
	}, nil
}

// GetDirectories returns the directories configuration view and metadata.
func GetDirectories(rt *vaultruntime.Runtime) (configPath string, exists bool, directories DirectoriesInfo, err error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return "", false, DirectoriesInfo{}, err
	}
	return configPath, exists, showDirectories(cfg), nil
}

// SetDirectories updates one or more directory fields. At least one non-nil field is required.
func SetDirectories(rt *vaultruntime.Runtime, daily, object, page, template *string) (*MutationResult, DirectoriesInfo, error) {
	if daily == nil && object == nil && page == nil && template == nil {
		return nil, DirectoriesInfo{}, svcerr.New(codes.ErrInvalidInput, "specify at least one directories field").WithSuggestion("Use --daily, --type, --page, or --template")
	}

	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, DirectoriesInfo{}, err
	}

	before := canonicalDirectoriesConfig(cfg)
	next := copyDirectoriesConfig(before)
	if next == nil {
		next = &config.DirectoriesConfig{}
	}

	if daily != nil {
		value, err := normalizeDirValue(*daily, "daily")
		if err != nil {
			return nil, DirectoriesInfo{}, err
		}
		next.Daily = value
	}
	if object != nil {
		value, err := normalizeDirValue(*object, "type")
		if err != nil {
			return nil, DirectoriesInfo{}, err
		}
		next.Object = value
	}
	if page != nil {
		value, err := normalizeDirValue(*page, "page")
		if err != nil {
			return nil, DirectoriesInfo{}, err
		}
		next.Page = value
	}
	if template != nil {
		value, err := normalizeDirValue(*template, "template")
		if err != nil {
			return nil, DirectoriesInfo{}, err
		}
		next.Template = value
	}
	next = compactDirectoriesConfig(next)
	changed := !directoriesConfigEqual(before, next)
	if changed {
		cfg.Directories = next
		if err := save(rt, cfg); err != nil {
			return nil, DirectoriesInfo{}, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, showDirectories(cfg), nil
}

// UnsetDirectories clears one or more directory fields. At least one true field is required.
func UnsetDirectories(rt *vaultruntime.Runtime, daily, object, page, template bool) (*MutationResult, DirectoriesInfo, error) {
	if !daily && !object && !page && !template {
		return nil, DirectoriesInfo{}, svcerr.New(codes.ErrInvalidInput, "specify at least one directories field to clear").WithSuggestion("Use --daily, --type, --page, or --template")
	}

	cfg, _, configPath, err := load(rt)
	if err != nil {
		return nil, DirectoriesInfo{}, err
	}

	before := canonicalDirectoriesConfig(cfg)
	next := copyDirectoriesConfig(before)
	if next == nil {
		next = &config.DirectoriesConfig{}
	}

	if daily {
		next.Daily = ""
	}
	if object {
		next.Object = ""
	}
	if page {
		next.Page = ""
	}
	if template {
		next.Template = ""
	}
	next = compactDirectoriesConfig(next)
	changed := !directoriesConfigEqual(before, next)
	if changed {
		cfg.Directories = next
		if err := save(rt, cfg); err != nil {
			return nil, DirectoriesInfo{}, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	return &MutationResult{
		ConfigPath: configPath,
		Changed:    changed,
	}, showDirectories(cfg), nil
}

// GetCapture returns the capture configuration view and metadata.
func GetCapture(rt *vaultruntime.Runtime) (configPath string, exists, configured bool, capture CaptureInfo, err error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return "", false, false, CaptureInfo{}, err
	}
	captureConfig := cfg.GetCaptureConfig()
	return configPath, exists, cfg.Capture != nil, CaptureInfo{Destination: captureConfig.Destination, Heading: captureConfig.Heading}, nil
}

// SetCapture updates one or more capture fields. At least one non-nil field is required.
func SetCapture(rt *vaultruntime.Runtime, destination, heading *string) (*MutationResult, bool, CaptureInfo, error) {
	if destination == nil && heading == nil {
		return nil, false, CaptureInfo{}, svcerr.New(codes.ErrInvalidInput, "specify at least one capture field").WithSuggestion("Use --destination or --heading")
	}

	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, false, CaptureInfo{}, err
	}

	before := canonicalCaptureConfig(cfg)
	next := copyCaptureConfig(before)
	if next == nil {
		next = &config.CaptureConfig{}
	}

	if destination != nil {
		value, err := normalizeCaptureDestination(*destination)
		if err != nil {
			return nil, false, CaptureInfo{}, err
		}
		next.Destination = value
	}
	if heading != nil {
		value, err := normalizeHeading(*heading)
		if err != nil {
			return nil, false, CaptureInfo{}, err
		}
		next.Heading = value
	}

	next = compactCaptureConfig(next)
	changed := !captureConfigEqual(before, next)
	if changed {
		cfg.Capture = next
		if err := save(rt, cfg); err != nil {
			return nil, false, CaptureInfo{}, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	capture := cfg.GetCaptureConfig()
	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, cfg.Capture != nil, CaptureInfo{Destination: capture.Destination, Heading: capture.Heading}, nil
}

// UnsetCapture clears one or more capture fields. At least one true field is required.
func UnsetCapture(rt *vaultruntime.Runtime, destination, heading bool) (*MutationResult, bool, CaptureInfo, error) {
	if !destination && !heading {
		return nil, false, CaptureInfo{}, svcerr.New(codes.ErrInvalidInput, "specify at least one capture field to clear").WithSuggestion("Use --destination or --heading")
	}

	cfg, _, configPath, err := load(rt)
	if err != nil {
		return nil, false, CaptureInfo{}, err
	}

	before := canonicalCaptureConfig(cfg)
	next := copyCaptureConfig(before)
	if next == nil {
		next = &config.CaptureConfig{}
	}

	if destination {
		next.Destination = ""
	}
	if heading {
		next.Heading = ""
	}

	next = compactCaptureConfig(next)
	changed := !captureConfigEqual(before, next)
	if changed {
		cfg.Capture = next
		if err := save(rt, cfg); err != nil {
			return nil, false, CaptureInfo{}, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	capture := cfg.GetCaptureConfig()
	return &MutationResult{
		ConfigPath: configPath,
		Changed:    changed,
	}, cfg.Capture != nil, CaptureInfo{Destination: capture.Destination, Heading: capture.Heading}, nil
}

// GetDeletion returns the deletion configuration view and metadata.
func GetDeletion(rt *vaultruntime.Runtime) (configPath string, exists, configured bool, deletion DeletionInfo, err error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return "", false, false, DeletionInfo{}, err
	}
	deletionConfig := cfg.GetDeletionConfig()
	return configPath, exists, cfg.Deletion != nil, DeletionInfo{Behavior: deletionConfig.Behavior, TrashDir: deletionConfig.TrashDir}, nil
}

// SetDeletion updates one or more deletion fields. At least one non-nil field is required.
func SetDeletion(rt *vaultruntime.Runtime, behavior, trashDir *string) (*MutationResult, bool, DeletionInfo, error) {
	if behavior == nil && trashDir == nil {
		return nil, false, DeletionInfo{}, svcerr.New(codes.ErrInvalidInput, "specify at least one deletion field").WithSuggestion("Use --behavior or --trash-dir")
	}

	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, false, DeletionInfo{}, err
	}

	before := canonicalDeletionConfig(cfg)
	next := copyDeletionConfig(before)
	if next == nil {
		next = &config.DeletionConfig{}
	}

	if behavior != nil {
		value, err := normalizeDeletionBehavior(*behavior)
		if err != nil {
			return nil, false, DeletionInfo{}, err
		}
		next.Behavior = value
	}
	if trashDir != nil {
		value, err := normalizeTrashDir(*trashDir)
		if err != nil {
			return nil, false, DeletionInfo{}, err
		}
		next.TrashDir = value
	}

	next = compactDeletionConfig(next)
	changed := !deletionConfigEqual(before, next)
	if changed {
		cfg.Deletion = next
		if err := save(rt, cfg); err != nil {
			return nil, false, DeletionInfo{}, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	deletion := cfg.GetDeletionConfig()
	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, cfg.Deletion != nil, DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir}, nil
}

// UnsetDeletion clears one or more deletion fields. At least one true field is required.
func UnsetDeletion(rt *vaultruntime.Runtime, behavior, trashDir bool) (*MutationResult, bool, DeletionInfo, error) {
	if !behavior && !trashDir {
		return nil, false, DeletionInfo{}, svcerr.New(codes.ErrInvalidInput, "specify at least one deletion field to clear").WithSuggestion("Use --behavior or --trash-dir")
	}

	cfg, _, configPath, err := load(rt)
	if err != nil {
		return nil, false, DeletionInfo{}, err
	}

	before := canonicalDeletionConfig(cfg)
	next := copyDeletionConfig(before)
	if next == nil {
		next = &config.DeletionConfig{}
	}

	if behavior {
		next.Behavior = ""
	}
	if trashDir {
		next.TrashDir = ""
	}

	next = compactDeletionConfig(next)
	changed := !deletionConfigEqual(before, next)
	if changed {
		cfg.Deletion = next
		if err := save(rt, cfg); err != nil {
			return nil, false, DeletionInfo{}, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	deletion := cfg.GetDeletionConfig()
	return &MutationResult{
		ConfigPath: configPath,
		Changed:    changed,
	}, cfg.Deletion != nil, DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir}, nil
}

// SetAutoReindex sets the auto_reindex flag explicitly.
func SetAutoReindex(rt *vaultruntime.Runtime, value bool) (*MutationResult, bool, bool, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, false, false, err
	}

	changed := cfg.AutoReindex == nil || *cfg.AutoReindex != value
	if changed {
		cfg.AutoReindex = &value
		if err := save(rt, cfg); err != nil {
			return nil, false, false, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, value, true, nil
}

// UnsetAutoReindex clears the explicit auto_reindex setting, reverting to the default.
func UnsetAutoReindex(rt *vaultruntime.Runtime) (*MutationResult, bool, bool, error) {
	cfg, _, configPath, err := load(rt)
	if err != nil {
		return nil, false, false, err
	}

	changed := cfg.AutoReindex != nil
	if changed {
		cfg.AutoReindex = nil
		if err := save(rt, cfg); err != nil {
			return nil, false, false, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	return &MutationResult{
		ConfigPath: configPath,
		Changed:    changed,
	}, cfg.IsAutoReindexEnabled(), false, nil
}

// ListProtectedPrefixes returns the normalized list of protected prefixes.
func ListProtectedPrefixes(rt *vaultruntime.Runtime) (configPath string, exists bool, prefixes []string, err error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return "", false, nil, err
	}
	return configPath, exists, normalizedProtectedPrefixes(cfg.ProtectedPrefixes), nil
}

// AddProtectedPrefix adds a protected prefix to the list (deduplicated).
func AddProtectedPrefix(rt *vaultruntime.Runtime, prefix string) (*MutationResult, string, []string, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, "", nil, err
	}

	normalized, err := normalizeProtectedPrefix(prefix)
	if err != nil {
		return nil, "", nil, err
	}

	prefixes := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	changed := true
	for _, existing := range prefixes {
		if existing == normalized {
			changed = false
			break
		}
	}
	if changed {
		prefixes = append(prefixes, normalized)
		sort.Strings(prefixes)
		cfg.ProtectedPrefixes = prefixes
		if err := save(rt, cfg); err != nil {
			return nil, "", nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, normalized, prefixes, nil
}

// RemoveProtectedPrefix removes a protected prefix from the list.
func RemoveProtectedPrefix(rt *vaultruntime.Runtime, prefix string) (*MutationResult, string, []string, error) {
	cfg, _, configPath, err := load(rt)
	if err != nil {
		return nil, "", nil, err
	}

	normalized, err := normalizeProtectedPrefix(prefix)
	if err != nil {
		return nil, "", nil, err
	}

	prefixes := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	next := make([]string, 0, len(prefixes))
	found := false
	for _, existing := range prefixes {
		if existing == normalized {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return nil, "", nil, svcerr.New(codes.ErrPrefixNotFound, fmt.Sprintf("protected prefix '%s' not found", normalized)).WithSuggestion("Run 'rvn vault config protected-prefixes list' to see configured prefixes")
	}

	cfg.ProtectedPrefixes = next
	if err := save(rt, cfg); err != nil {
		return nil, "", nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
	}

	return &MutationResult{
		ConfigPath: configPath,
		Changed:    true,
	}, normalized, next, nil
}

// ListExclude returns the normalized list of exclude patterns.
func ListExclude(rt *vaultruntime.Runtime) (configPath string, exists bool, patterns []string, err error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return "", false, nil, err
	}
	return configPath, exists, normalizedExcludePatterns(cfg.Exclude), nil
}

// AddExclude adds an exclude pattern to the list (deduplicated).
func AddExclude(rt *vaultruntime.Runtime, pattern string) (*MutationResult, string, []string, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, "", nil, err
	}

	normalized, err := normalizeExcludePattern(pattern)
	if err != nil {
		return nil, "", nil, err
	}

	patterns := normalizedExcludePatterns(cfg.Exclude)
	changed := true
	for _, existing := range patterns {
		if existing == normalized {
			changed = false
			break
		}
	}
	if changed {
		patterns = append(patterns, normalized)
		cfg.Exclude = patterns
		if err := save(rt, cfg); err != nil {
			return nil, "", nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}

	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, normalized, patterns, nil
}

// RemoveExclude removes an exclude pattern from the list.
func RemoveExclude(rt *vaultruntime.Runtime, pattern string) (*MutationResult, string, []string, error) {
	cfg, _, configPath, err := load(rt)
	if err != nil {
		return nil, "", nil, err
	}

	normalized, err := normalizeExcludePattern(pattern)
	if err != nil {
		return nil, "", nil, err
	}

	patterns := normalizedExcludePatterns(cfg.Exclude)
	next := make([]string, 0, len(patterns))
	found := false
	for _, existing := range patterns {
		if existing == normalized {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return nil, "", nil, svcerr.New(codes.ErrPrefixNotFound, fmt.Sprintf("exclude pattern %q not found", normalized)).WithSuggestion("Run 'rvn vault config exclude list' to see configured patterns")
	}

	cfg.Exclude = next
	if err := save(rt, cfg); err != nil {
		return nil, "", nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
	}

	return &MutationResult{
		ConfigPath: configPath,
		Changed:    true,
	}, normalized, next, nil
}

func load(rt *vaultruntime.Runtime) (*config.VaultConfig, bool, string, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, false, "", svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err).WithSuggestion("Resolve a vault before invoking the command")
	}
	if rt.VaultCfg == nil {
		if err := rt.ReloadConfig(); err != nil {
			return nil, rt.VaultConfigExists, rt.VaultConfigPath, svcerr.Wrap(codes.ErrConfigInvalid, "failed to load vault config", err).WithSuggestion("Fix raven.yaml and try again")
		}
	}
	return rt.VaultCfg, rt.VaultConfigExists, rt.VaultConfigPath, nil
}

func save(rt *vaultruntime.Runtime, cfg *config.VaultConfig) error {
	err := config.SaveVaultConfig(rt.VaultPath, cfg)
	reloadErr := rt.ReloadConfig()
	if err != nil {
		return err
	}
	return reloadErr
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
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid protected prefix: %q", raw)).WithSuggestion("Use a vault-relative directory prefix such as 'private/'")
	}
	return normalized, nil
}

func normalizedExcludePatterns(patterns []string) []string {
	return ravenignore.NormalizePatterns(patterns)
}

func normalizeExcludePattern(raw string) (string, error) {
	normalized := ravenignore.NormalizePatterns([]string{raw})
	if len(normalized) == 0 {
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid exclude pattern: %q", raw)).WithSuggestion("Use a gitignore-style pattern such as 'AGENTS.md', '.cursor/', or '*.plan.md'")
	}
	if _, err := ravenignore.NewMatcher(normalized); err != nil {
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid exclude pattern: %q", raw)).WithSuggestion(err.Error())
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

func canonicalDirectoriesConfig(cfg *config.VaultConfig) *config.DirectoriesConfig {
	if cfg.Directories == nil {
		return nil
	}

	dirs := *cfg.Directories

	daily := paths.NormalizeDirRoot(dirs.Daily)
	object := dirs.Object
	object = paths.NormalizeDirRoot(object)

	page := dirs.Page
	if page == "" {
		//nolint:staticcheck // Backward-compatible read of deprecated config key.
		page = dirs.Pages
	}
	page = paths.NormalizeDirRoot(page)

	template := dirs.Template
	if template == "" {
		//nolint:staticcheck // Backward-compatible read of deprecated config key.
		template = dirs.Templates
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

func normalizeDirValue(raw, field string) (string, error) {
	normalized := paths.NormalizeDirRoot(paths.NormalizeVaultRelPath(raw))
	if normalized == "" || !paths.IsValidVaultRelPath(normalized) {
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid directories.%s: %q", field, raw)).WithSuggestion("Use a vault-relative directory such as 'daily/'")
	}
	return normalized, nil
}

func normalizeCaptureDestination(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", svcerr.New(codes.ErrInvalidInput, "capture destination cannot be empty").WithSuggestion("Use 'daily' or a vault-relative file path")
	}
	if value == "daily" {
		return "daily", nil
	}
	value = paths.NormalizeVaultRelPath(value)
	if !paths.IsValidVaultRelPath(value) {
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid capture destination: %q", raw)).WithSuggestion("Use 'daily' or a vault-relative file path such as 'inbox.md'")
	}
	return value, nil
}

func normalizeHeading(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", svcerr.New(codes.ErrInvalidInput, "heading cannot be empty").WithSuggestion("Use a heading such as '## Captured'")
	}
	return value, nil
}

func normalizeDeletionBehavior(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch value {
	case "trash", "permanent":
		return value, nil
	default:
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid deletion behavior: %q", raw)).WithSuggestion("Use 'trash' or 'permanent'")
	}
}

func normalizeTrashDir(raw string) (string, error) {
	value := paths.NormalizeVaultRelPath(raw)
	if value == "" || !paths.IsValidVaultRelPath(value) {
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid trash_dir: %q", raw)).WithSuggestion("Use a vault-relative path such as '.trash' or 'archive/trash'")
	}
	return value, nil
}
