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
	keepName      string // Named keep from config
	keepPathFlag  string // Explicit path (rare)
	configPath    string
	statePathFlag string

	// Resolved values
	resolvedKeepPath   string
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
		// Skip keep resolution for commands that don't need it
		switch cmd.Name() {
		case "init", "keep", "config", "completion", "help", "version", "serve", "skill", "mcp":
			return nil
		}
		// Also skip for completion/config/skill/mcp subcommands.
		// Most keep subcommands do not require resolved keep path, except `keep path`.
		if cmd.Parent() != nil {
			parentName := cmd.Parent().Name()
			if parentName == "keep" && cmd.Name() == "path" {
				// `keep path` should resolve exactly like keep-bound commands.
			} else if parentName == "completion" || parentName == "keep" || parentName == "config" || parentName == "skill" || parentName == "mcp" {
				return nil
			}
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

		// Resolve keep path: explicit path > named keep > active state > default
		if keepPathFlag != "" {
			// Explicit path takes priority
			resolvedKeepPath = keepPathFlag
		} else if keepName != "" {
			// Named keep from --keep flag
			resolvedKeepPath, err = cfg.GetKeepPath(keepName)
			if err != nil {
				return fmt.Errorf("keep '%s' not found\n\nRun 'rvn keep list' to see configured keeps", keepName)
			}
		} else {
			state, stateErr := config.LoadState(resolvedStatePath)
			if stateErr != nil {
				return fmt.Errorf("failed to load state: %w", stateErr)
			}

			activeKeepName := strings.TrimSpace(state.ActiveKeep)
			if activeKeepName != "" {
				resolvedKeepPath, err = cfg.GetKeepPath(activeKeepName)
				if err != nil {
					resolvedKeepPath, err = cfg.GetDefaultKeepPath()
					if err != nil {
						return fmt.Errorf("active keep '%s' not found in config and no default keep configured\n\nRun 'rvn keep use <name>' or set default_keep in config.toml", activeKeepName)
					}
					if !jsonOutput {
						fmt.Fprintf(os.Stderr, "warning: active keep '%s' not found in config, falling back to default\n", activeKeepName)
					}
				}
			} else {
				// Default keep
				resolvedKeepPath, err = cfg.GetDefaultKeepPath()
				if err != nil {
					return fmt.Errorf(`no keep specified

Either:
  1. Use --keep <name> (from config)
  2. Use --keep-path /path/to/keep
  3. Run 'rvn keep use <name>' to set active_keep in state.toml
  4. Set default_keep in ~/.config/raven/config.toml
  5. Run 'rvn init /path/to/new/keep' to create one`)
				}
			}
		}

		// Verify keep exists
		if _, err := os.Stat(resolvedKeepPath); os.IsNotExist(err) {
			return fmt.Errorf("keep not found: %s\n\nRun 'rvn init %s' to create it", resolvedKeepPath, resolvedKeepPath)
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
	rootCmd.PersistentFlags().StringVarP(&keepName, "keep", "v", "", "Named keep from config")
	rootCmd.PersistentFlags().StringVar(&keepPathFlag, "keep-path", "", "Explicit path to keep directory")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().StringVar(&statePathFlag, "state", "", "Path to state file (overrides state_file in config)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format (for agent/script use)")
}

// getKeepPath returns the resolved keep path.
func getKeepPath() string {
	return resolvedKeepPath
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

func loadGlobalConfigAllowMissingWithPath() (*config.Config, string, bool, error) {
	resolvedPath := config.ResolveConfigPath(configPath)

	// Default path loading already treats missing files as empty config.
	if strings.TrimSpace(configPath) == "" {
		loadedCfg, err := config.Load()
		if err != nil {
			return nil, "", false, err
		}
		if loadedCfg == nil {
			loadedCfg = &config.Config{}
		}
		_, statErr := os.Stat(resolvedPath)
		return loadedCfg, resolvedPath, statErr == nil, nil
	}

	_, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &config.Config{}, resolvedPath, false, nil
		}
		return nil, "", false, err
	}

	loadedCfg, err := config.LoadFrom(resolvedPath)
	if err != nil {
		return nil, "", false, err
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	return loadedCfg, resolvedPath, true, nil
}
