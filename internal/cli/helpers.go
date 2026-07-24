package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// loadVaultConfigSafe loads the vault config.
// Returns an error if raven.yaml exists but is invalid.
func loadVaultConfigSafe(vaultPath string) (*config.VaultConfig, error) {
	rt, err := vaultruntime.New(vaultPath, vaultruntime.Options{SkipSchema: true})
	if err != nil {
		return nil, fmt.Errorf("failed to load raven.yaml: %w", err)
	}
	defer rt.Close()
	cfg := rt.VaultCfg
	if cfg == nil {
		return &config.VaultConfig{}, nil
	}
	return cfg, nil
}

func loadSchemaSafe(vaultPath string) (*schema.Schema, error) {
	rt, err := vaultruntime.New(vaultPath, vaultruntime.Options{
		SkipConfig:    true,
		RequireSchema: true,
	})
	if err != nil {
		return nil, err
	}
	defer rt.Close()
	return rt.Schema, nil
}
