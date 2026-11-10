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
	Configured bool   `json:"configured"`
	Daily      string `json:"daily"`
	Type       string `json:"type"`
	Page       string `json:"page"`
	Template   string `json:"template"`
}

// CaptureInfo is a view of the effective capture configuration.
type CaptureInfo struct {
	Destination string `json:"destination"`
	Heading     string `json:"heading"`
}

// DeletionInfo is a view of the effective deletion configuration.
type DeletionInfo struct {
	Behavior string `json:"behavior"`
	TrashDir string `json:"trash_dir"`
}

// ShowResult contains the full vault configuration summary.
type ShowResult struct {
	ConfigPath             string          `json:"config_path"`
	Exists                 bool            `json:"exists"`
	AutoReindex            bool            `json:"auto_reindex"`
	AutoReindexExplicit    bool            `json:"auto_reindex_explicit"`
	DailyTemplate          string          `json:"daily_template"`
	Directories            DirectoriesInfo `json:"directories"`
	Capture                CaptureInfo     `json:"capture"`
	Deletion               DeletionInfo    `json:"deletion"`
	QueriesCount           int             `json:"queries_count"`
	ProtectedPrefixes      []string        `json:"protected_prefixes"`
	ProtectedPrefixesCount int             `json:"protected_prefixes_count"`
	Exclude                []string        `json:"exclude"`
	ExcludeCount           int             `json:"exclude_count"`
	ProtectedPrefixesUsed  bool            `json:"-"`
	ExcludeUsed            bool            `json:"-"`
}

// MutationResult contains common fields for all mutation operations.
type MutationResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
}

type mutationPointers int

const (
	pointersNone mutationPointers = iota
	pointersSet
	pointersUnset
)

// DirectoriesResult is returned by directories get/set/unset.
type DirectoriesResult struct {
	ConfigPath     string          `json:"config_path"`
	Exists         bool            `json:"exists"`
	Configured     bool            `json:"configured"`
	Daily          string          `json:"daily"`
	Type           string          `json:"type"`
	Page           string          `json:"page"`
	Template       string          `json:"template"`
	Created        *bool           `json:"created,omitempty"`
	Changed        *bool           `json:"changed,omitempty"`
	Directories    DirectoriesInfo `json:"-"`
	MutationResult `json:"-"`
}

// CaptureResult is returned by capture get/set/unset.
type CaptureResult struct {
	ConfigPath     string      `json:"config_path"`
	Exists         bool        `json:"exists"`
	Configured     bool        `json:"configured"`
	Destination    string      `json:"destination"`
	Heading        string      `json:"heading"`
	Created        *bool       `json:"created,omitempty"`
	Changed        *bool       `json:"changed,omitempty"`
	Capture        CaptureInfo `json:"-"`
	MutationResult `json:"-"`
}

// DeletionResult is returned by deletion get/set/unset.
type DeletionResult struct {
	ConfigPath     string       `json:"config_path"`
	Exists         bool         `json:"exists"`
	Configured     bool         `json:"configured"`
	Behavior       string       `json:"behavior"`
	TrashDir       string       `json:"trash_dir"`
	Created        *bool        `json:"created,omitempty"`
	Changed        *bool        `json:"changed,omitempty"`
	Deletion       DeletionInfo `json:"-"`
	MutationResult `json:"-"`
}

// AutoReindexResult is returned by auto-reindex set/unset.
type AutoReindexResult struct {
	ConfigPath          string `json:"config_path"`
	Created             *bool  `json:"created,omitempty"`
	Changed             bool   `json:"changed"`
	AutoReindex         bool   `json:"auto_reindex"`
	AutoReindexExplicit bool   `json:"auto_reindex_explicit"`
	MutationResult      `json:"-"`
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
		ConfigPath:             configPath,
		Exists:                 exists,
		AutoReindex:            cfg.IsAutoReindexEnabled(),
		AutoReindexExplicit:    cfg.AutoReindex != nil,
		DailyTemplate:          strings.TrimSpace(cfg.DailyTemplate),
		Directories:            directories,
		Capture:                CaptureInfo{Destination: capture.Destination, Heading: capture.Heading},
		Deletion:               DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir},
		QueriesCount:           len(cfg.Queries),
		ProtectedPrefixes:      protected,
		ProtectedPrefixesCount: len(protected),
		Exclude:                exclude,
		ExcludeCount:           len(exclude),
		ProtectedPrefixesUsed:  len(protected) > 0,
		ExcludeUsed:            len(exclude) > 0,
	}, nil
}

// GetDirectories returns the directories configuration view and metadata.
func GetDirectories(rt *vaultruntime.Runtime) (*DirectoriesResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	return directoriesView(showDirectories(cfg), exists, MutationResult{ConfigPath: configPath}, pointersNone), nil
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
	return directoriesResult(rt, mutation, pointersSet)
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
	return directoriesResult(rt, mutation, pointersUnset)
}

// GetCapture returns the capture configuration view and metadata.
func GetCapture(rt *vaultruntime.Runtime) (*CaptureResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	return captureView(cfg, exists, MutationResult{ConfigPath: configPath}, pointersNone), nil
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
	return captureResult(rt, mutation, pointersSet)
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
	return captureResult(rt, mutation, pointersUnset)
}

// GetDeletion returns the deletion configuration view and metadata.
func GetDeletion(rt *vaultruntime.Runtime) (*DeletionResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	return deletionView(cfg, exists, MutationResult{ConfigPath: configPath}, pointersNone), nil
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
	return deletionResult(rt, mutation, pointersSet)
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
	return deletionResult(rt, mutation, pointersUnset)
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
	created := mutation.Created
	return &AutoReindexResult{
		ConfigPath:          mutation.ConfigPath,
		Created:             &created,
		Changed:             mutation.Changed,
		AutoReindex:         value,
		AutoReindexExplicit: true,
		MutationResult:      *mutation,
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
		ConfigPath:          mutation.ConfigPath,
		Changed:             mutation.Changed,
		AutoReindex:         cfg.IsAutoReindexEnabled(),
		AutoReindexExplicit: false,
		MutationResult:      *mutation,
	}, nil
}

// ListProtectedPrefixes returns the normalized list of protected prefixes.
func ListProtectedPrefixes(rt *vaultruntime.Runtime) (*ProtectedPrefixesListResult, error) {
	configPath, exists, prefixes, err := protectedPrefixes.list(rt)
	if err != nil {
		return nil, err
	}
	return &ProtectedPrefixesListResult{
		ConfigPath:        configPath,
		Exists:            exists,
		ProtectedPrefixes: prefixes,
	}, nil
}

// AddProtectedPrefix adds a protected prefix to the list (deduplicated).
func AddProtectedPrefix(rt *vaultruntime.Runtime, prefix string) (*ProtectedPrefixAddResult, error) {
	change, err := protectedPrefixes.add(rt, prefix)
	if err != nil {
		return nil, err
	}
	return change.prefixAdd(), nil
}

// RemoveProtectedPrefix removes a protected prefix from the list.
func RemoveProtectedPrefix(rt *vaultruntime.Runtime, prefix string) (*ProtectedPrefixRemoveResult, error) {
	change, err := protectedPrefixes.remove(rt, prefix)
	if err != nil {
		return nil, err
	}
	return change.prefixRemove(), nil
}

// ListExclude returns the normalized list of exclude patterns.
func ListExclude(rt *vaultruntime.Runtime) (*ExcludeListResult, error) {
	configPath, exists, patterns, err := excludePatterns.list(rt)
	if err != nil {
		return nil, err
	}
	return &ExcludeListResult{
		ConfigPath: configPath,
		Exists:     exists,
		Exclude:    patterns,
	}, nil
}

// AddExclude adds an exclude pattern to the list (deduplicated).
func AddExclude(rt *vaultruntime.Runtime, pattern string) (*ExcludeAddResult, error) {
	change, err := excludePatterns.add(rt, pattern)
	if err != nil {
		return nil, err
	}
	return change.excludeAdd(), nil
}

// RemoveExclude removes an exclude pattern from the list.
func RemoveExclude(rt *vaultruntime.Runtime, pattern string) (*ExcludeRemoveResult, error) {
	change, err := excludePatterns.remove(rt, pattern)
	if err != nil {
		return nil, err
	}
	return change.excludeRemove(), nil
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

func directoriesResult(rt *vaultruntime.Runtime, mutation *MutationResult, pointers mutationPointers) (*DirectoriesResult, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return directoriesView(showDirectories(cfg), exists, *mutation, pointers), nil
}

func directoriesView(info DirectoriesInfo, exists bool, mutation MutationResult, pointers mutationPointers) *DirectoriesResult {
	result := &DirectoriesResult{
		ConfigPath:     mutation.ConfigPath,
		Exists:         exists,
		Configured:     info.Configured,
		Daily:          info.Daily,
		Type:           info.Type,
		Page:           info.Page,
		Template:       info.Template,
		Directories:    info,
		MutationResult: mutation,
	}
	applyMutationPointers(&result.Created, &result.Changed, mutation, pointers)
	return result
}

func captureResult(rt *vaultruntime.Runtime, mutation *MutationResult, pointers mutationPointers) (*CaptureResult, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return captureView(cfg, exists, *mutation, pointers), nil
}

func captureView(cfg *config.VaultConfig, exists bool, mutation MutationResult, pointers mutationPointers) *CaptureResult {
	capture := cfg.GetCaptureConfig()
	info := CaptureInfo{Destination: capture.Destination, Heading: capture.Heading}
	result := &CaptureResult{
		ConfigPath:     mutation.ConfigPath,
		Exists:         exists,
		Configured:     cfg.Capture != nil,
		Destination:    info.Destination,
		Heading:        info.Heading,
		Capture:        info,
		MutationResult: mutation,
	}
	applyMutationPointers(&result.Created, &result.Changed, mutation, pointers)
	return result
}

func deletionResult(rt *vaultruntime.Runtime, mutation *MutationResult, pointers mutationPointers) (*DeletionResult, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return deletionView(cfg, exists, *mutation, pointers), nil
}

func deletionView(cfg *config.VaultConfig, exists bool, mutation MutationResult, pointers mutationPointers) *DeletionResult {
	deletion := cfg.GetDeletionConfig()
	info := DeletionInfo{Behavior: deletion.Behavior, TrashDir: deletion.TrashDir}
	result := &DeletionResult{
		ConfigPath:     mutation.ConfigPath,
		Exists:         exists,
		Configured:     cfg.Deletion != nil,
		Behavior:       info.Behavior,
		TrashDir:       info.TrashDir,
		Deletion:       info,
		MutationResult: mutation,
	}
	applyMutationPointers(&result.Created, &result.Changed, mutation, pointers)
	return result
}

func applyMutationPointers(created, changed **bool, mutation MutationResult, pointers mutationPointers) {
	switch pointers {
	case pointersSet:
		c, ch := mutation.Created, mutation.Changed
		*created = &c
		*changed = &ch
	case pointersUnset:
		ch := mutation.Changed
		*changed = &ch
	}
}

func (c *listChange) prefixAdd() *ProtectedPrefixAddResult {
	return &ProtectedPrefixAddResult{
		ConfigPath:        c.ConfigPath,
		Created:           c.Created,
		Changed:           c.Changed,
		Prefix:            c.Item,
		ProtectedPrefixes: c.Values,
	}
}

func (c *listChange) prefixRemove() *ProtectedPrefixRemoveResult {
	return &ProtectedPrefixRemoveResult{
		ConfigPath:        c.ConfigPath,
		Changed:           c.Changed,
		Removed:           c.Item,
		ProtectedPrefixes: c.Values,
	}
}

func (c *listChange) excludeAdd() *ExcludeAddResult {
	return &ExcludeAddResult{
		ConfigPath: c.ConfigPath,
		Created:    c.Created,
		Changed:    c.Changed,
		Pattern:    c.Item,
		Exclude:    c.Values,
	}
}

func (c *listChange) excludeRemove() *ExcludeRemoveResult {
	return &ExcludeRemoveResult{
		ConfigPath: c.ConfigPath,
		Changed:    c.Changed,
		Removed:    c.Item,
		Exclude:    c.Values,
	}
}
