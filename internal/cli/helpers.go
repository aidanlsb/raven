package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
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

// buildParseOptions creates parser.ParseOptions from vault config.
// Returns nil if no directory configuration is present.
func buildParseOptions(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil || !vaultCfg.HasDirectoriesConfig() {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}

// openDatabaseWithConfig opens the database and sets the daily directory.
// Caller is responsible for calling db.Close().
func openDatabaseWithConfig(vaultPath string, vaultCfg *config.VaultConfig) (*index.Database, error) {
	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, err
	}
	if vaultCfg != nil {
		db.SetDailyDirectory(vaultCfg.DailyDirectory)
	}
	return db, nil
}

// maybeReindex reindexes a file if auto-reindex is enabled in the vault config.
// Errors are logged but not returned (best-effort reindexing).
func maybeReindex(vaultPath, filePath string, vaultCfg *config.VaultConfig) {
	if vaultCfg == nil || !vaultCfg.IsAutoReindexEnabled() {
		return
	}
	if err := reindexFile(vaultPath, filePath, vaultCfg); err != nil {
		if !isJSONOutput() {
			fmt.Printf("  (reindex failed: %v)\n", err)
		}
	}
}
