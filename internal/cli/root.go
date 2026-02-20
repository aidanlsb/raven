// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	// Global flags
	vaultName     string // Named vault from config
	vaultPathFlag string // Explicit path (rare)
	configPath    string
	statePathFlag string

	// Resolved values
	resolvedVaultPath  string
	resolvedConfigPath string
	resolvedStatePath  string
	cfg                *config.Config
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
		case "init", "vault", "completion", "help", "version", "serve", "skill":
			return nil
		}
		// Also skip for completion/vault subcommands.
		if cmd.Parent() != nil && (cmd.Parent().Name() == "completion" || cmd.Parent().Name() == "vault" || cmd.Parent().Name() == "skill") {
			return nil
		}

		// Load config
		var err error
		cfg, resolvedConfigPath, err = loadGlobalConfigWithPath()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		resolvedStatePath = config.ResolveStatePath(statePathFlag, resolvedConfigPath, cfg)
		ui.ConfigureTheme(cfg.UI.Accent)
		ui.ConfigureMarkdownCodeTheme(cfg.UI.CodeTheme)

		// Resolve vault path: explicit path > named vault > active state > default
		if vaultPathFlag != "" {
			// Explicit path takes priority
			resolvedVaultPath = vaultPathFlag
		} else if vaultName != "" {
			// Named vault from --vault flag
			resolvedVaultPath, err = cfg.GetVaultPath(vaultName)
			if err != nil {
				return fmt.Errorf("vault '%s' not found\n\nRun 'rvn vault list' to see configured vaults", vaultName)
			}
		} else {
			state, stateErr := config.LoadState(resolvedStatePath)
			if stateErr != nil {
				return fmt.Errorf("failed to load state: %w", stateErr)
			}

			activeVaultName := strings.TrimSpace(state.ActiveVault)
			if activeVaultName != "" {
				resolvedVaultPath, err = cfg.GetVaultPath(activeVaultName)
				if err != nil {
					resolvedVaultPath, err = cfg.GetDefaultVaultPath()
					if err != nil {
						return fmt.Errorf("active vault '%s' not found in config and no default vault configured\n\nRun 'rvn vault use <name>' or set default_vault in config.toml", activeVaultName)
					}
					if !jsonOutput {
						fmt.Fprintf(os.Stderr, "warning: active vault '%s' not found in config, falling back to default\n", activeVaultName)
					}
				}
			} else {
				// Default vault
				resolvedVaultPath, err = cfg.GetDefaultVaultPath()
				if err != nil {
					return fmt.Errorf(`no vault specified

Either:
  1. Use --vault <name> (from config)
  2. Use --vault-path /path/to/vault
  3. Run 'rvn vault use <name>' to set active_vault in state.toml
  4. Set default_vault in ~/.config/raven/config.toml
  5. Run 'rvn init /path/to/new/vault' to create one`)
				}
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
	syncRegistryMetadata(rootCmd)
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&vaultName, "vault", "v", "", "Named vault from config")
	rootCmd.PersistentFlags().StringVar(&vaultPathFlag, "vault-path", "", "Explicit path to vault directory")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().StringVar(&statePathFlag, "state", "", "Path to state file (overrides state_file in config)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format (for agent/script use)")
}

// getVaultPath returns the resolved vault path.
func getVaultPath() string {
	return resolvedVaultPath
}

// getConfig returns the loaded config.
func getConfig() *config.Config {
	return cfg
}

// getConfigPath returns the resolved global config path.
func getConfigPath() string {
	return resolvedConfigPath
}

// getStatePath returns the resolved global state path.
func getStatePath() string {
	return resolvedStatePath
}

func loadGlobalConfigWithPath() (*config.Config, string, error) {
	resolvedPath := config.ResolveConfigPath(configPath)

	var loadedCfg *config.Config
	var err error
	if strings.TrimSpace(configPath) != "" {
		loadedCfg, err = config.LoadFrom(configPath)
	} else {
		loadedCfg, err = config.Load()
	}
	if err != nil {
		return nil, "", err
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	return loadedCfg, resolvedPath, nil
}
