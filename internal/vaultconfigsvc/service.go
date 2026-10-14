package vaultconfigsvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
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

// DirectoriesResult is returned by directories get/set/unset.
type DirectoriesResult struct {
	MutationResult
	Exists      bool
	Directories DirectoriesInfo
}

// CaptureResult is returned by capture get/set/unset.
type CaptureResult struct {
	MutationResult
	Exists     bool
	Configured bool
	Capture    CaptureInfo
}

// DeletionResult is returned by deletion get/set/unset.
type DeletionResult struct {
	MutationResult
	Exists     bool
	Configured bool
	Deletion   DeletionInfo
}

// AutoReindexResult is returned by auto-reindex set/unset.
type AutoReindexResult struct {
	MutationResult
	AutoReindex bool
	Explicit    bool
}

// PrefixesResult is returned by protected-prefix list/add/remove.
type PrefixesResult struct {
	MutationResult
	Exists   bool
	Prefix   string
	Prefixes []string
}

// ExcludeResult is returned by exclude list/add/remove.
type ExcludeResult struct {
	MutationResult
	Exists   bool
	Pattern  string
	Patterns []string
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
func GetDirectories(rt *vaultruntime.Runtime) (*DirectoriesResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	return &DirectoriesResult{
		MutationResult: MutationResult{ConfigPath: configPath},
		Exists:         exists,
		Directories:    showDirectories(cfg),
	}, nil
}

// SetDirectories updates one or more directory fields. At least one non-nil field is required.
func SetDirectories(rt *vaultruntime.Runtime, daily, object, page, template *string) (*DirectoriesResult, error) {
	if daily == nil && object == nil && page == nil && template == nil {
		return nil, svcerr.New(codes.ErrInvalidInput, "specify at least one directories field").WithSuggestion("Use --daily, --type, --page, or --template")
	}
	mutation, err := mutateDirectories(rt, func(next *config.DirectoriesConfig) error {
		if daily != nil {
			value, err := normalizeDirValue(*daily, "daily")
			if err != nil {
				return err
			}
			next.Daily = value
		}
		if object != nil {
			value, err := normalizeDirValue(*object, "type")
			if err != nil {
				return err
			}
			next.Object = value
		}
		if page != nil {
			value, err := normalizeDirValue(*page, "page")
			if err != nil {
				return err
			}
			next.Page = value
		}
		if template != nil {
			value, err := normalizeDirValue(*template, "template")
			if err != nil {
				return err
			}
			next.Template = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return directoriesResult(rt, mutation)
}

// UnsetDirectories clears one or more directory fields. At least one true field is required.
func UnsetDirectories(rt *vaultruntime.Runtime, daily, object, page, template bool) (*DirectoriesResult, error) {
	if !daily && !object && !page && !template {
		return nil, svcerr.New(codes.ErrInvalidInput, "specify at least one directories field to clear").WithSuggestion("Use --daily, --type, --page, or --template")
	}
	mutation, err := mutateDirectories(rt, func(next *config.DirectoriesConfig) error {
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	return directoriesResult(rt, mutation)
}

// GetCapture returns the capture configuration view and metadata.
func GetCapture(rt *vaultruntime.Runtime) (*CaptureResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	return captureView(cfg, exists, MutationResult{ConfigPath: configPath}), nil
}

// SetCapture updates one or more capture fields. At least one non-nil field is required.
func SetCapture(rt *vaultruntime.Runtime, destination, heading *string) (*CaptureResult, error) {
	if destination == nil && heading == nil {
		return nil, svcerr.New(codes.ErrInvalidInput, "specify at least one capture field").WithSuggestion("Use --destination or --heading")
	}
	mutation, err := mutateCapture(rt, func(next *config.CaptureConfig) error {
		if destination != nil {
			value, err := normalizeCaptureDestination(*destination)
			if err != nil {
				return err
			}
			next.Destination = value
		}
		if heading != nil {
			value, err := normalizeHeading(*heading)
			if err != nil {
				return err
			}
			next.Heading = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return captureResult(rt, mutation)
}

// UnsetCapture clears one or more capture fields. At least one true field is required.
func UnsetCapture(rt *vaultruntime.Runtime, destination, heading bool) (*CaptureResult, error) {
	if !destination && !heading {
		return nil, svcerr.New(codes.ErrInvalidInput, "specify at least one capture field to clear").WithSuggestion("Use --destination or --heading")
	}
	mutation, err := mutateCapture(rt, func(next *config.CaptureConfig) error {
		if destination {
			next.Destination = ""
		}
		if heading {
			next.Heading = ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return captureResult(rt, mutation)
}

// GetDeletion returns the deletion configuration view and metadata.
func GetDeletion(rt *vaultruntime.Runtime) (*DeletionResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	return deletionView(cfg, exists, MutationResult{ConfigPath: configPath}), nil
}

// SetDeletion updates one or more deletion fields. At least one non-nil field is required.
func SetDeletion(rt *vaultruntime.Runtime, behavior, trashDir *string) (*DeletionResult, error) {
	if behavior == nil && trashDir == nil {
		return nil, svcerr.New(codes.ErrInvalidInput, "specify at least one deletion field").WithSuggestion("Use --behavior or --trash-dir")
	}
	mutation, err := mutateDeletion(rt, func(next *config.DeletionConfig) error {
		if behavior != nil {
			value, err := normalizeDeletionBehavior(*behavior)
			if err != nil {
				return err
			}
			next.Behavior = value
		}
		if trashDir != nil {
			value, err := normalizeTrashDir(*trashDir)
			if err != nil {
				return err
			}
			next.TrashDir = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deletionResult(rt, mutation)
}

// UnsetDeletion clears one or more deletion fields. At least one true field is required.
func UnsetDeletion(rt *vaultruntime.Runtime, behavior, trashDir bool) (*DeletionResult, error) {
	if !behavior && !trashDir {
		return nil, svcerr.New(codes.ErrInvalidInput, "specify at least one deletion field to clear").WithSuggestion("Use --behavior or --trash-dir")
	}
	mutation, err := mutateDeletion(rt, func(next *config.DeletionConfig) error {
		if behavior {
			next.Behavior = ""
		}
		if trashDir {
			next.TrashDir = ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deletionResult(rt, mutation)
}

// SetAutoReindex sets the auto_reindex flag explicitly.
func SetAutoReindex(rt *vaultruntime.Runtime, value bool) (*AutoReindexResult, error) {
	mutation, err := mutate(rt, func(cfg *config.VaultConfig) error {
		cfg.AutoReindex = &value
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &AutoReindexResult{
		MutationResult: *mutation,
		AutoReindex:    value,
		Explicit:       true,
	}, nil
}

// UnsetAutoReindex clears the explicit auto_reindex setting, reverting to the default.
func UnsetAutoReindex(rt *vaultruntime.Runtime) (*AutoReindexResult, error) {
	mutation, err := mutate(rt, func(cfg *config.VaultConfig) error {
		cfg.AutoReindex = nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	cfg, _, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return &AutoReindexResult{
		MutationResult: *mutation,
		AutoReindex:    cfg.IsAutoReindexEnabled(),
		Explicit:       false,
	}, nil
}

// ListProtectedPrefixes returns the normalized list of protected prefixes.
func ListProtectedPrefixes(rt *vaultruntime.Runtime) (*PrefixesResult, error) {
	configPath, exists, prefixes, err := protectedPrefixes.list(rt)
	if err != nil {
		return nil, err
	}
	return &PrefixesResult{
		MutationResult: MutationResult{ConfigPath: configPath},
		Exists:         exists,
		Prefixes:       prefixes,
	}, nil
}

// AddProtectedPrefix adds a protected prefix to the list (deduplicated).
func AddProtectedPrefix(rt *vaultruntime.Runtime, prefix string) (*PrefixesResult, error) {
	change, err := protectedPrefixes.add(rt, prefix)
	if err != nil {
		return nil, err
	}
	return change.prefixes(), nil
}

// RemoveProtectedPrefix removes a protected prefix from the list.
func RemoveProtectedPrefix(rt *vaultruntime.Runtime, prefix string) (*PrefixesResult, error) {
	change, err := protectedPrefixes.remove(rt, prefix)
	if err != nil {
		return nil, err
	}
	return change.prefixes(), nil
}

// ListExclude returns the normalized list of exclude patterns.
func ListExclude(rt *vaultruntime.Runtime) (*ExcludeResult, error) {
	configPath, exists, patterns, err := excludePatterns.list(rt)
	if err != nil {
		return nil, err
	}
	return &ExcludeResult{
		MutationResult: MutationResult{ConfigPath: configPath},
		Exists:         exists,
		Patterns:       patterns,
	}, nil
}

// AddExclude adds an exclude pattern to the list (deduplicated).
func AddExclude(rt *vaultruntime.Runtime, pattern string) (*ExcludeResult, error) {
	change, err := excludePatterns.add(rt, pattern)
	if err != nil {
		return nil, err
	}
	return change.exclude(), nil
}

// RemoveExclude removes an exclude pattern from the list.
func RemoveExclude(rt *vaultruntime.Runtime, pattern string) (*ExcludeResult, error) {
	change, err := excludePatterns.remove(rt, pattern)
	if err != nil {
		return nil, err
	}
	return change.exclude(), nil
}

func mutateDirectories(rt *vaultruntime.Runtime, apply func(*config.DirectoriesConfig) error) (*MutationResult, error) {
	return mutateSection(rt, canonicalDirectoriesConfig, setDirectoriesConfig, compactDirectoriesConfig, apply)
}

func mutateCapture(rt *vaultruntime.Runtime, apply func(*config.CaptureConfig) error) (*MutationResult, error) {
	return mutateSection(rt, captureConfig, setCaptureConfig, compactCaptureConfig, apply)
}

func mutateDeletion(rt *vaultruntime.Runtime, apply func(*config.DeletionConfig) error) (*MutationResult, error) {
	return mutateSection(rt, deletionConfig, setDeletionConfig, compactDeletionConfig, apply)
}

func directoriesResult(rt *vaultruntime.Runtime, mutation *MutationResult) (*DirectoriesResult, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return &DirectoriesResult{
		MutationResult: *mutation,
		Exists:         exists,
		Directories:    showDirectories(cfg),
	}, nil
}

func captureResult(rt *vaultruntime.Runtime, mutation *MutationResult) (*CaptureResult, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return captureView(cfg, exists, *mutation), nil
}

func captureView(cfg *config.VaultConfig, exists bool, mutation MutationResult) *CaptureResult {
	capture := cfg.GetCaptureConfig()
	return &CaptureResult{
		MutationResult: mutation,
		Exists:         exists,
		Configured:     cfg.Capture != nil,
		Capture:        CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
	}
}

func deletionResult(rt *vaultruntime.Runtime, mutation *MutationResult) (*DeletionResult, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return deletionView(cfg, exists, *mutation), nil
}

func deletionView(cfg *config.VaultConfig, exists bool, mutation MutationResult) *DeletionResult {
	deletion := cfg.GetDeletionConfig()
	return &DeletionResult{
		MutationResult: mutation,
		Exists:         exists,
		Configured:     cfg.Deletion != nil,
		Deletion:       DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
	}
}

func (c *listChange) prefixes() *PrefixesResult {
	return &PrefixesResult{
		MutationResult: c.MutationResult,
		Exists:         c.Exists,
		Prefix:         c.Item,
		Prefixes:       c.Values,
	}
}

func (c *listChange) exclude() *ExcludeResult {
	return &ExcludeResult{
		MutationResult: c.MutationResult,
		Exists:         c.Exists,
		Pattern:        c.Item,
		Patterns:       c.Values,
	}
}
