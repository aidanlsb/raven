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
)

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
	object := paths.NormalizeDirRoot(dirs.Object)

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

func compactCaptureConfig(cfg *config.CaptureConfig) *config.CaptureConfig {
	if cfg == nil {
		return nil
	}
	if cfg.Destination == "" && cfg.Heading == "" {
		return nil
	}
	return cfg
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

func captureConfig(cfg *config.VaultConfig) *config.CaptureConfig {
	return cfg.Capture
}

func setCaptureConfig(cfg *config.VaultConfig, next *config.CaptureConfig) {
	cfg.Capture = next
}

func deletionConfig(cfg *config.VaultConfig) *config.DeletionConfig {
	return cfg.Deletion
}

func setDeletionConfig(cfg *config.VaultConfig, next *config.DeletionConfig) {
	cfg.Deletion = next
}

func setDirectoriesConfig(cfg *config.VaultConfig, next *config.DirectoriesConfig) {
	cfg.Directories = next
}
