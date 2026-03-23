package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
)

// loadVaultConfigSafe loads the vault config.
// Returns an error if raven.yaml exists but is invalid.
func loadVaultConfigSafe(vaultPath string) (*config.VaultConfig, error) {
	cfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load raven.yaml: %w", err)
	}
	if cfg == nil {
		return &config.VaultConfig{}, nil
	}
	return cfg, nil
}

// openDatabaseWithConfig opens the database and sets the daily directory.
// Caller is responsible for calling db.Close().
func openDatabaseWithConfig(vaultPath string, vaultCfg *config.VaultConfig) (*index.Database, error) {
	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, err
	}
	if vaultCfg != nil {
		db.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	}
	return db, nil
}
