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
	errJSONStartupHandled = errors.New("json startup error already handled")

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
			return handleStartupError(ErrConfigInvalid, fmt.Sprintf("failed to load config: %v", err), "")
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		resolvedStatePath = config.ResolveStatePath(statePathFlag, resolvedConfigPath, cfg)
		ui.ConfigureTheme(cfg.UI.Accent)
		ui.ConfigureMarkdownCodeTheme(cfg.UI.CodeTheme)

		// Skip vault resolution for commands that don't need it
		switch cmd.Name() {
		case "init", "completion", "help", "version", "serve", "skill", "mcp":
			return nil
		case "vault", "config":
			if isRootChildCommand(cmd) {
				return nil
			}
		}
		// Also skip for completion/config/skill/mcp subcommands.
		// Most vault subcommands do not require resolved vault path, except `vault path` and `vault stats`.
		if cmd.Parent() != nil {
			parent := cmd.Parent()
			parentName := parent.Name()
			if parentName == "vault" && (cmd.Name() == "path" || cmd.Name() == "stats" || cmd.Name() == "config") {
				// `vault path` and `vault stats` should resolve exactly like vault-bound commands.
			} else if parentName == "completion" || parentName == "vault" || parentName == "skill" || parentName == "mcp" {
				return nil
			} else if parentName == "config" && isRootChildCommand(parent) {
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
				return handleStartupError(
					ErrVaultNotFound,
					fmt.Sprintf("vault '%s' not found", vaultName),
					"Run 'rvn vault list' to see configured vaults",
				)
			}
		} else {
			state, stateErr := config.LoadState(resolvedStatePath)
			if stateErr != nil {
				return handleStartupError(ErrConfigInvalid, fmt.Sprintf("failed to load state: %v", stateErr), "")
			}

			activeVaultName := strings.TrimSpace(state.ActiveVault)
			if activeVaultName != "" {
				resolvedVaultPath, err = cfg.GetVaultPath(activeVaultName)
				if err != nil {
					resolvedVaultPath, err = cfg.GetDefaultVaultPath()
					if err != nil {
						return handleStartupError(
							ErrVaultNotFound,
							fmt.Sprintf("active vault '%s' not found in config and no default vault configured", activeVaultName),
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
					return handleStartupError(
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
		if _, err := os.Stat(resolvedVaultPath); err != nil {
			if os.IsNotExist(err) {
				return handleStartupError(
					ErrVaultNotFound,
					fmt.Sprintf("vault not found: %s", resolvedVaultPath),
					fmt.Sprintf("Run 'rvn init %s' to create it", resolvedVaultPath),
				)
			}
			return handleStartupError(ErrInternal, fmt.Sprintf("failed to access vault: %v", err), "")
		}

		return nil
	},
}

// Execute runs the CLI.
func Execute() error {
	syncRegistryMetadata(rootCmd)
	prevSilenceErrors := rootCmd.SilenceErrors
	prevSilenceUsage := rootCmd.SilenceUsage
	if argsRequestJSON(os.Args[1:]) {
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
	}
	defer func() {
		rootCmd.SilenceErrors = prevSilenceErrors
		rootCmd.SilenceUsage = prevSilenceUsage
	}()

	err := rootCmd.Execute()
	if errors.Is(err, errJSONStartupHandled) {
		return nil
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

func isRootChildCommand(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Parent() != nil && cmd.Parent().Name() == "rvn"
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

func handleStartupError(code, message, suggestion string) error {
	if jsonOutput {
		outputError(code, message, nil, suggestion)
		return errJSONStartupHandled
	}
	if suggestion != "" {
		return fmt.Errorf("%s\n\n%s", message, suggestion)
	}
	return fmt.Errorf("%s", message)
}

func argsRequestJSON(args []string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "--json" {
			return true
		}
		if strings.HasPrefix(trimmed, "--json=") {
			return trimmed != "--json=false"
		}
	}
	return false
}
