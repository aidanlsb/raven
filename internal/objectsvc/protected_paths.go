package objectsvc

import (
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutationguard"
)

func ValidateContentMutationFilePath(vaultPath string, vaultCfg *config.VaultConfig, filePath string) error {
	if err := mutationguard.ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return err
	}
	return nil
}

func ValidateContentMutationRelPath(vaultCfg *config.VaultConfig, relPath string) error {
	if err := mutationguard.ValidateContentMutationRelPath(vaultCfg, relPath); err != nil {
		return err
	}
	return nil
}
