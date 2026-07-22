package parser

import "github.com/aidanlsb/raven/internal/config"

// OptionsFromVaultConfig builds the canonical parser options for a vault.
// Daily note identity always follows the configured daily root. Object and page
// roots affect identity only when directory organization is explicitly enabled.
func OptionsFromVaultConfig(vaultCfg *config.VaultConfig) *ParseOptions {
	if vaultCfg == nil {
		return nil
	}

	opts := &ParseOptions{
		DailyRoot: vaultCfg.GetDailyDirectory(),
	}
	if vaultCfg.HasDirectoriesConfig() {
		opts.ObjectsRoot = vaultCfg.GetObjectsRoot()
		opts.PagesRoot = vaultCfg.GetPagesRoot()
	}
	return opts
}
