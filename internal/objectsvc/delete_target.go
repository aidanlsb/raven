package objectsvc

import (
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vault"
)

type deleteTarget struct {
	ObjectID     string
	FilePath     string
	RelativePath string
	RavenObject  bool
}

func deleteTargetFromFilePath(vaultPath string, vaultCfg *config.VaultConfig, filePath, objectID string) (*deleteTarget, error) {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return nil, newError(ErrorUnexpected, "failed to resolve delete target path", "", nil, err)
	}
	relPath = paths.NormalizeVaultRelPath(relPath)

	ravenObject := paths.HasMDExtension(relPath)
	if !ravenObject {
		objectID = relPath
	} else if objectID == "" {
		objectID = vaultCfg.FilePathToObjectID(relPath)
	}

	return &deleteTarget{
		ObjectID:     objectID,
		FilePath:     filePath,
		RelativePath: relPath,
		RavenObject:  ravenObject,
	}, nil
}

func resolveBulkDeleteTarget(vaultPath string, vaultCfg *config.VaultConfig, reference string) (*deleteTarget, error) {
	// Non-Markdown files are accepted only as explicit vault-relative paths;
	// they are not Raven references.
	relReference := paths.NormalizeVaultRelPath(reference)
	literalPath := filepath.Join(vaultPath, filepath.FromSlash(relReference))
	if err := paths.ValidateWithinVault(vaultPath, literalPath); err == nil {
		if info, statErr := os.Stat(literalPath); statErr == nil && !info.IsDir() && !paths.HasMDExtension(relReference) {
			return deleteTargetFromFilePath(vaultPath, vaultCfg, literalPath, relReference)
		}
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, reference, vaultCfg)
	if err != nil {
		return nil, err
	}
	return deleteTargetFromFilePath(vaultPath, vaultCfg, filePath, "")
}
