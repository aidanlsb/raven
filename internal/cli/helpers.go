package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
)

// loadKeepConfigSafe loads the keep config.
// Returns an error if raven.yaml exists but is invalid.
func loadKeepConfigSafe(keepPath string) (*config.KeepConfig, error) {
	cfg, err := config.LoadKeepConfig(keepPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load raven.yaml: %w", err)
	}
	if cfg == nil {
		return &config.KeepConfig{}, nil
	}
	return cfg, nil
}

// buildParseOptions creates parser.ParseOptions from keep config.
// Returns nil if no directory configuration is present.
func buildParseOptions(keepCfg *config.KeepConfig) *parser.ParseOptions {
	if keepCfg == nil || !keepCfg.HasDirectoriesConfig() {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: keepCfg.GetObjectsRoot(),
		PagesRoot:   keepCfg.GetPagesRoot(),
	}
}

// openDatabaseWithConfig opens the database and sets the daily directory.
// Caller is responsible for calling db.Close().
func openDatabaseWithConfig(keepPath string, keepCfg *config.KeepConfig) (*index.Database, error) {
	db, err := index.Open(keepPath)
	if err != nil {
		return nil, err
	}
	if keepCfg != nil {
		db.SetDailyDirectory(keepCfg.GetDailyDirectory())
	}
	return db, nil
}

// maybeReindex reindexes a file if auto-reindex is enabled in the keep config.
// Errors are logged but not returned (best-effort reindexing).
func maybeReindex(keepPath, filePath string, keepCfg *config.KeepConfig) {
	if keepCfg == nil || !keepCfg.IsAutoReindexEnabled() {
		return
	}
	if err := reindexFile(keepPath, filePath, keepCfg); err != nil {
		if !isJSONOutput() {
			fmt.Printf("  (reindex failed: %v)\n", err)
		}
	}
}
