package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

func ValidateContentMutationFilePath(vaultPath string, vaultCfg *config.VaultConfig, filePath string) error {
	if strings.TrimSpace(filePath) == "" {
		return newError(ErrorInvalidInput, "file path is required", "", nil, nil)
	}

	relPath := filePath
	if filepath.IsAbs(filePath) {
		if strings.TrimSpace(vaultPath) == "" {
			return nil
		}
		var err error
		relPath, err = filepath.Rel(vaultPath, filePath)
		if err != nil {
			return newError(ErrorValidationFailed, "failed to resolve target path", "", nil, err)
		}
	}

	return ValidateContentMutationRelPath(vaultCfg, relPath)
}

func ValidateContentMutationRelPath(vaultCfg *config.VaultConfig, relPath string) error {
	normalized := paths.NormalizeVaultRelPath(relPath)
	if normalized == "" {
		return newError(ErrorInvalidInput, "path is required", "", nil, nil)
	}

	templateDir := ""
	protectedPrefixes := []string(nil)
	if vaultCfg != nil {
		templateDir = vaultCfg.GetTemplateDirectory()
		protectedPrefixes = vaultCfg.ProtectedPrefixes
	}

	if paths.IsProtectedRelPath(normalized, protectedPrefixes) {
		return newError(
			ErrorValidationFailed,
			"cannot modify protected or system-managed paths",
			"Choose a path outside protected prefixes, or update them with 'rvn vault config protected-prefixes ...'",
			map[string]interface{}{"path": normalized},
			nil,
		)
	}

	if templateDir != "" && strings.HasPrefix(normalized, templateDir) {
		return newError(
			ErrorValidationFailed,
			"cannot modify template files with content mutation commands",
			"Use 'rvn template ...' or 'rvn schema template ...' for template changes",
			map[string]interface{}{"path": normalized},
			nil,
		)
	}

	return nil
}
