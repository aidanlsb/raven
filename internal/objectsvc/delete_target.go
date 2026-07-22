package objectsvc

import (
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vault"
)

type deleteTarget struct {
	ObjectID     string
	FilePath     string
	RelativePath string
	IsAsset      bool
}

func deleteTargetFromFilePath(vaultPath string, vaultCfg *config.VaultConfig, filePath, objectID string) (*deleteTarget, error) {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return nil, newError(ErrorUnexpected, "failed to resolve delete target path", "", nil, err)
	}
	relPath = paths.NormalizeVaultRelPath(relPath)

	isAsset := !paths.HasMDExtension(relPath)
	if isAsset {
		// Asset IDs are stable vault-relative file paths. Do not apply object
		// directory-root mappings to them.
		objectID = relPath
	} else if objectID == "" {
		objectID = vaultCfg.FilePathToObjectID(relPath)
	}

	return &deleteTarget{
		ObjectID:     objectID,
		FilePath:     filePath,
		RelativePath: relPath,
		IsAsset:      isAsset,
	}, nil
}

func resolveBulkDeleteTarget(vaultPath string, vaultCfg *config.VaultConfig, reference string) (*deleteTarget, error) {
	// Object resolution only considers Markdown candidates. Asset IDs are
	// vault-relative paths, so resolve an existing non-Markdown file literally
	// before using the object resolver.
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

func removeDeleteTargetFromIndex(db *index.Database, target *deleteTarget) error {
	if target.IsAsset {
		return db.RemoveFile(target.RelativePath)
	}
	return db.RemoveDocument(target.ObjectID)
}
