// Package cli implements the command-line interface.
package cli

import (
	"errors"
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
		var err error

		// Load global config and apply UI settings for every command, including
		// non-vault commands like `config show` and `version`.
		cfg, resolvedConfigPath, err = loadGlobalConfigWithPath()
		if err != nil {
			return newRootPreRunError(cmd, ErrConfigInvalid, fmt.Sprintf("failed to load config: %v", err), "")
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		resolvedStatePath = config.ResolveStatePath(statePathFlag, resolvedConfigPath, cfg)
		ui.ConfigureTheme(cfg.UI.Accent)
		ui.ConfigureMarkdownCodeTheme(cfg.UI.CodeTheme)

		// Skip vault resolution for commands that don't need it
		switch cmd.Name() {
		case "init", "vault", "config", "completion", "help", "version", "serve", "skill", "mcp":
			return nil
		}
		// Also skip for completion/config/skill/mcp subcommands.
		// Most vault subcommands do not require resolved vault path, except `vault path` and `vault stats`.
		if cmd.Parent() != nil {
			parentName := cmd.Parent().Name()
			if parentName == "vault" && (cmd.Name() == "path" || cmd.Name() == "stats") {
				// `vault path` and `vault stats` should resolve exactly like vault-bound commands.
			} else if parentName == "completion" || parentName == "vault" || parentName == "config" || parentName == "skill" || parentName == "mcp" {
				return nil
			}
		}

		// Resolve vault path: explicit path > named vault > active state > default
		if vaultPathFlag != "" {
			// Explicit path takes priority
			resolvedVaultPath = vaultPathFlag
		} else if vaultName != "" {
			// Named vault from --vault flag
			resolvedVaultPath, err = cfg.GetVaultPath(vaultName)
			if err != nil {
				return newRootPreRunError(
					cmd,
					ErrVaultNotFound,
					fmt.Sprintf("vault %q not found", vaultName),
					"Run 'rvn vault list' to see configured vaults",
				)
			}
		} else {
			state, stateErr := config.LoadState(resolvedStatePath)
			if stateErr != nil {
				return newRootPreRunError(cmd, ErrConfigInvalid, fmt.Sprintf("failed to load state: %v", stateErr), "")
			}

			activeVaultName := strings.TrimSpace(state.ActiveVault)
			if activeVaultName != "" {
				resolvedVaultPath, err = cfg.GetVaultPath(activeVaultName)
				if err != nil {
					resolvedVaultPath, err = cfg.GetDefaultVaultPath()
					if err != nil {
						return newRootPreRunError(
							cmd,
							ErrVaultNotFound,
							fmt.Sprintf("active vault %q not found in config and no default vault configured", activeVaultName),
							"Run 'rvn vault use <name>' or set default_vault in config.toml",
						)
					}
					if !jsonOutput {
						fmt.Fprintf(os.Stderr, "warning: active vault '%s' not found in config, falling back to default\n", activeVaultName)
					}
				}
			} else {
				// Default vault
				resolvedVaultPath, err = cfg.GetDefaultVaultPath()
				if err != nil {
					return newRootPreRunError(
						cmd,
						ErrVaultNotSpecified,
						"no vault specified",
						`Either:
  1. Use --vault <name> (from config)
  2. Use --vault-path /path/to/vault
  3. Run 'rvn vault use <name>' to set active_vault in state.toml
  4. Set default_vault in ~/.config/raven/config.toml
  5. Run 'rvn init /path/to/new/vault' to create one`,
					)
				}
			}
		}

		// Verify vault exists
		if _, err := os.Stat(resolvedVaultPath); os.IsNotExist(err) {
			return newRootPreRunError(
				cmd,
				ErrVaultNotFound,
				fmt.Sprintf("vault not found: %s", resolvedVaultPath),
				fmt.Sprintf("Run 'rvn init %s' to create it", resolvedVaultPath),
			)
		}

		return nil
	},
}

// Execute runs the CLI.
func Execute() error {
	syncRegistryMetadata(rootCmd)
	err := rootCmd.Execute()
	if err == nil {
		return nil
	}

	var preRunErr *rootPreRunError
	if errors.As(err, &preRunErr) {
		outputError(preRunErr.Code, preRunErr.Message, nil, preRunErr.Suggestion)
	}

	return err
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

func loadGlobalConfigWithPath() (*config.Config, string, error) {
	resolvedPath := config.ResolveConfigPath(configPath)

	var loadedCfg *config.Config
	var err error
	if strings.TrimSpace(configPath) != "" {
		if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
			return &config.Config{}, resolvedPath, nil
		}
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

type rootPreRunError struct {
	Code       string
	Message    string
	Suggestion string
}

func (e *rootPreRunError) Error() string {
	if e.Suggestion == "" {
		return e.Message
	}
	return e.Message + "\n\n" + e.Suggestion
}

func newRootPreRunError(cmd *cobra.Command, code, message, suggestion string) error {
	if !jsonOutput {
		if suggestion == "" {
			return fmt.Errorf("%s", message)
		}
		return fmt.Errorf("%s\n\n%s", message, suggestion)
	}

	silenceCobraForJSON(cmd)
	return &rootPreRunError{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
	}
}

func silenceCobraForJSON(cmd *cobra.Command) {
	root := cmd.Root()
	root.SilenceErrors = true
	root.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
}
