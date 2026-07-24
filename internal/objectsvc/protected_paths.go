package objectsvc

import (
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutationguard"
)

func ValidateContentMutationFilePath(vaultPath string, vaultCfg *config.VaultConfig, filePath string) error {
	return mutationguard.ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath)
}

func ValidateContentMutationRelPath(vaultCfg *config.VaultConfig, relPath string) error {
	return mutationguard.ValidateContentMutationRelPath(vaultCfg, relPath)
}
