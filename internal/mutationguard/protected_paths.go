// Package mutationguard enforces path policy for content mutations.
package mutationguard

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
)

// ValidateContentMutationFilePath validates a file path against the vault's
// content-mutation path policy.
func ValidateContentMutationFilePath(vaultPath string, vaultCfg *config.VaultConfig, filePath string) *svcerr.Error {
	if strings.TrimSpace(filePath) == "" {
		return svcerr.New(codes.ErrInvalidInput, "file path is required")
	}

	relPath := filePath
	if filepath.IsAbs(filePath) {
		if strings.TrimSpace(vaultPath) == "" {
			return nil
		}
		var err error
		relPath, err = filepath.Rel(vaultPath, filePath)
		if err != nil {
			return svcerr.Wrap(codes.ErrValidationFailed, "failed to resolve target path", err)
		}
	}

	return ValidateContentMutationRelPath(vaultCfg, relPath)
}

// ValidateContentMutationRelPath validates a vault-relative path against the
// vault's content-mutation path policy.
func ValidateContentMutationRelPath(vaultCfg *config.VaultConfig, relPath string) *svcerr.Error {
	normalized := paths.NormalizeVaultRelPath(relPath)
	if normalized == "" {
		return svcerr.New(codes.ErrInvalidInput, "path is required")
	}

	templateDir := ""
	var protectedPrefixes []string
	if vaultCfg != nil {
		templateDir = vaultCfg.GetTemplateDirectory()
		protectedPrefixes = vaultCfg.ProtectedPrefixes
	}

	if paths.IsProtectedRelPath(normalized, protectedPrefixes) {
		return svcerr.New(codes.ErrValidationFailed, "cannot modify protected or system-managed paths").
			WithSuggestion("Choose a path outside protected prefixes, or update them with 'rvn vault config protected-prefixes ...'").
			WithDetails(map[string]any{"path": normalized})
	}

	if vaultCfg != nil {
		excludeMatcher, err := ravenignore.NewMatcher(vaultCfg.GetExcludePatterns())
		if err != nil {
			return svcerr.Wrap(codes.ErrValidationFailed, "invalid exclude config", err).
				WithSuggestion("Fix raven.yaml exclude patterns and try again").
				WithDetails(map[string]any{"path": normalized})
		}
		if excludeMatcher.Match(normalized, false) {
			return svcerr.New(codes.ErrValidationFailed, "cannot modify excluded paths").
				WithSuggestion("Choose a managed path, or update exclusions with 'rvn vault config exclude ...'").
				WithDetails(map[string]any{"path": normalized})
		}
	}

	if templateDir != "" && strings.HasPrefix(normalized, templateDir) {
		return svcerr.New(codes.ErrValidationFailed, "cannot modify template files with content mutation commands").
			WithSuggestion("Use 'rvn template ...' or 'rvn schema template ...' for template changes").
			WithDetails(map[string]any{"path": normalized})
	}

	return nil
}
