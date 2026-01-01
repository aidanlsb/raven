// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	vaultName     string // Named vault from config
	vaultPathFlag string // Explicit path (rare)
	configPath    string

	// Resolved values
	resolvedVaultPath string
	cfg               *config.Config
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "rvn",
	Short: "Raven - A personal knowledge system",
	Long: `Raven is a personal knowledge system with typed blocks, traits, and powerful querying.
Built for speed, with plain-text markdown files as the source of truth.

Named for Odin's ravens Huginn (thought) and Muninn (memory), 
who gathered knowledge from across the world.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip vault resolution for commands that don't need it
		switch cmd.Name() {
		case "init", "vaults", "completion", "help":
			return nil
		}
		// Also skip for completion subcommands (bash, zsh, fish, powershell)
		if cmd.Parent() != nil && cmd.Parent().Name() == "completion" {
			return nil
		}

		// Load config
		var err error
		if configPath != "" {
			cfg, err = config.LoadFrom(configPath)
		} else {
			cfg, err = config.Load()
		}
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if cfg == nil {
			cfg = &config.Config{}
		}

		// Resolve vault path: explicit path > named vault > default
		if vaultPathFlag != "" {
			// Explicit path takes priority
			resolvedVaultPath = vaultPathFlag
		} else if vaultName != "" {
			// Named vault from --vault flag
			resolvedVaultPath, err = cfg.GetVaultPath(vaultName)
			if err != nil {
				return fmt.Errorf("vault '%s' not found\n\nRun 'rvn vaults' to see configured vaults", vaultName)
			}
		} else {
			// Default vault
			resolvedVaultPath, err = cfg.GetDefaultVaultPath()
			if err != nil {
				return fmt.Errorf(`no vault specified

Either:
  1. Use --vault <name> (from config)
  2. Use --vault-path /path/to/vault
  3. Set default_vault in ~/.config/raven/config.toml
  4. Run 'rvn init /path/to/new/vault' to create one`)
			}
		}

		// Verify vault exists
		if _, err := os.Stat(resolvedVaultPath); os.IsNotExist(err) {
			return fmt.Errorf("vault not found: %s\n\nRun 'rvn init %s' to create it", resolvedVaultPath, resolvedVaultPath)
		}

		return nil
	},
}

// Execute runs the CLI.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&vaultName, "vault", "v", "", "Named vault from config")
	rootCmd.PersistentFlags().StringVar(&vaultPathFlag, "vault-path", "", "Explicit path to vault directory")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")
}

// getVaultPath returns the resolved vault path.
func getVaultPath() string {
	return resolvedVaultPath
}

// getConfig returns the loaded config.
func getConfig() *config.Config {
	return cfg
}
