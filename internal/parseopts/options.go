// Package parseopts translates vault configuration into canonical parser
// options.
package parseopts

import (
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
)

// FromVaultConfig builds the canonical parser options for a vault.
// Daily note identity always follows the configured daily root. Object and page
// roots affect identity only when directory organization is explicitly enabled.
func FromVaultConfig(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil {
		return nil
	}

	opts := &parser.ParseOptions{
		DailyRoot: vaultCfg.GetDailyDirectory(),
	}
	if vaultCfg.HasDirectoriesConfig() {
		opts.ObjectsRoot = vaultCfg.GetObjectsRoot()
		opts.PagesRoot = vaultCfg.GetPagesRoot()
	}
	return opts
}
