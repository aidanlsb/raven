package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
)

type vaultContext struct {
	cfg        *config.Config
	state      *config.State
	configPath string
	statePath  string
}

type vaultRow struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"is_default"`
	IsActive  bool   `json:"is_active"`
}

type currentVaultInfo struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Source        string `json:"source"`
	ActiveMissing bool   `json:"active_missing"`
}

var (
	vaultAddReplace         bool
	vaultAddPin             bool
	vaultRemoveClearDefault bool
	vaultRemoveClearActive  bool
)

func loadVaultContext() (*vaultContext, error) {
	serviceCtx, err := configsvc.LoadVaultContext(configsvc.ContextOptions{
		ConfigPathOverride: configPath,
		StatePathOverride:  statePathFlag,
	})
	if err != nil {
		return nil, err
	}

	return &vaultContext{
		cfg:        serviceCtx.Cfg,
		state:      serviceCtx.State,
		configPath: serviceCtx.ConfigPath,
		statePath:  serviceCtx.StatePath,
	}, nil
}

func vaultRows(cfg *config.Config, state *config.State) ([]vaultRow, string, string, bool) {
	serviceRows, defaultName, activeName, activeMissing := configsvc.VaultRows(cfg, state)
	rows := make([]vaultRow, 0, len(serviceRows))
	for _, r := range serviceRows {
		rows = append(rows, vaultRow{
			Name:      r.Name,
			Path:      r.Path,
			IsDefault: r.IsDefault,
			IsActive:  r.IsActive,
		})
	}
	return rows, defaultName, activeName, activeMissing
}

func resolveCurrentVault(cfg *config.Config, state *config.State) (*currentVaultInfo, error) {
	current, err := configsvc.ResolveCurrentVault(cfg, state)
	if err != nil {
		return nil, err
	}
	return &currentVaultInfo{
		Name:          current.Name,
		Path:          current.Path,
		Source:        current.Source,
		ActiveMissing: current.ActiveMissing,
	}, nil
}

func runVaultList(cmd *cobra.Command, args []string) error {
	ctx, err := loadVaultContext()
	if err != nil {
		return handleError(ErrConfigInvalid, err, "")
	}

	rows, defaultName, activeName, activeMissing := vaultRows(ctx.cfg, ctx.state)
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"config_path":    ctx.configPath,
			"state_path":     ctx.statePath,
			"default_vault":  defaultName,
			"active_vault":   activeName,
			"active_missing": activeMissing,
			"vaults":         rows,
		}, &Meta{Count: len(rows)})
		return nil
	}

	if len(rows) == 0 {
		fmt.Println("No vaults configured.")
		fmt.Printf("Config: %s\n", ctx.configPath)
		fmt.Println()
		fmt.Println("Add vaults to config.toml:")
		fmt.Println()
		fmt.Println("  default_vault = \"personal\"")
		fmt.Println()
		fmt.Println("  [vaults]")
		fmt.Println("  personal = \"/path/to/your/notes\"")
		return nil
	}

	for _, row := range rows {
		prefix := "  "
		if row.IsActive && row.IsDefault {
			prefix = ">*"
		} else if row.IsActive {
			prefix = "> "
		} else if row.IsDefault {
			prefix = " *"
		}
		fmt.Printf("%s %-12s -> %s\n", prefix, row.Name, row.Path)
	}

	fmt.Println()
	fmt.Println("> = active vault (state)")
	fmt.Println("* = default vault (config)")
	fmt.Printf("config: %s\n", ctx.configPath)
	fmt.Printf("state:  %s\n", ctx.statePath)
	if activeMissing {
		fmt.Printf("warning: active vault '%s' in state is not configured\n", activeName)
	}

	return nil
}

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage configured vaults and active selection",
	Long: `Manage configured vaults and active selection.

The active vault is stored in state.toml.
The default vault is stored in config.toml and used as fallback.`,
	Args: cobra.NoArgs,
	RunE: runVaultList,
}

var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured vaults",
	Args:  cobra.NoArgs,
	RunE:  runVaultList,
}

var vaultCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the current resolved vault",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := configsvc.CurrentVault(configsvc.ContextOptions{
			ConfigPathOverride: configPath,
			StatePathOverride:  statePathFlag,
		})
		if err != nil {
			return handleConfigSvcError(err, "Use 'rvn vault use <name>' or set default_vault in config.toml")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":           result.Current.Name,
				"path":           result.Current.Path,
				"source":         result.Current.Source,
				"active_missing": result.Current.ActiveMissing,
				"config_path":    result.ConfigPath,
				"state_path":     result.StatePath,
			}, nil)
			return nil
		}

		fmt.Printf("current: %s\n", result.Current.Name)
		fmt.Printf("path:    %s\n", result.Current.Path)
		fmt.Printf("source:  %s\n", result.Current.Source)
		if result.Current.ActiveMissing {
			fmt.Printf("warning: active vault '%s' is missing; using default\n", result.ActiveVault)
		}
		return nil
	},
}

var vaultUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active vault in state.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		result, err := configsvc.UseVault(configsvc.ContextOptions{
			ConfigPathOverride: configPath,
			StatePathOverride:  statePathFlag,
		}, name)
		if err != nil {
			return handleConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"active_vault": result.ActiveVault,
				"path":         result.Path,
				"state_path":   result.StatePath,
			}, nil)
			return nil
		}

		fmt.Printf("Active vault set to '%s' -> %s\n", result.ActiveVault, result.Path)
		fmt.Printf("state: %s\n", result.StatePath)
		return nil
	},
}

var vaultClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear active vault from state.toml",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := configsvc.ClearActiveVault(configsvc.ContextOptions{
			ConfigPathOverride: configPath,
			StatePathOverride:  statePathFlag,
		})
		if err != nil {
			return handleConfigSvcError(err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"cleared":    result.Cleared,
				"previous":   result.Previous,
				"state_path": result.StatePath,
			}, nil)
			return nil
		}

		if result.Previous == "" {
			fmt.Println("Active vault already clear.")
		} else {
			fmt.Printf("Cleared active vault '%s'.\n", result.Previous)
		}
		fmt.Printf("state: %s\n", result.StatePath)
		return nil
	},
}

var vaultPinCmd = &cobra.Command{
	Use:   "pin <name>",
	Short: "Set default_vault in config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		result, err := configsvc.PinVault(configsvc.ContextOptions{
			ConfigPathOverride: configPath,
			StatePathOverride:  statePathFlag,
		}, name)
		if err != nil {
			return handleConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"default_vault": result.DefaultVault,
				"path":          result.Path,
				"config_path":   result.ConfigPath,
			}, nil)
			return nil
		}

		fmt.Printf("Default vault set to '%s' -> %s\n", result.DefaultVault, result.Path)
		fmt.Printf("config: %s\n", result.ConfigPath)
		return nil
	},
}

var vaultAddCmd = &cobra.Command{
	Use:   "add <name> <path>",
	Short: "Add a vault to config.toml",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		rawPath := strings.TrimSpace(args[1])
		result, err := configsvc.AddVault(configsvc.VaultAddRequest{
			ContextOptions: configsvc.ContextOptions{
				ConfigPathOverride: configPath,
				StatePathOverride:  statePathFlag,
			},
			Name:    name,
			RawPath: rawPath,
			Replace: vaultAddReplace,
			Pin:     vaultAddPin,
		})
		if err != nil {
			if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeFileNotFound {
				return handleConfigSvcError(err, "Run 'rvn init "+rawPath+"' to create it first")
			}
			if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeDuplicateName {
				return handleConfigSvcError(err, "Use --replace to update the path")
			}
			return handleConfigSvcError(err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":          result.Name,
				"path":          result.Path,
				"config_path":   result.ConfigPath,
				"replaced":      result.Replaced,
				"previous_path": result.PreviousPath,
				"pinned":        result.Pinned,
				"default_vault": result.DefaultVault,
			}, nil)
			return nil
		}

		if result.Replaced {
			fmt.Printf("Updated vault '%s' -> %s\n", result.Name, result.Path)
		} else {
			fmt.Printf("Added vault '%s' -> %s\n", result.Name, result.Path)
		}
		if result.Pinned {
			fmt.Printf("Default vault set to '%s'.\n", result.Name)
		}
		fmt.Printf("config: %s\n", result.ConfigPath)
		return nil
	},
}

var vaultRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a vault from config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		result, err := configsvc.RemoveVault(configsvc.VaultRemoveRequest{
			ContextOptions: configsvc.ContextOptions{
				ConfigPathOverride: configPath,
				StatePathOverride:  statePathFlag,
			},
			Name:         name,
			ClearDefault: vaultRemoveClearDefault,
			ClearActive:  vaultRemoveClearActive,
		})
		if err != nil {
			if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeConfirmationNeeded {
				if strings.Contains(svcErr.Message, "default vault") {
					return handleConfigSvcError(err, "Use --clear-default to clear default_vault as part of removal, or pin another vault first")
				}
				if strings.Contains(svcErr.Message, "active vault") {
					return handleConfigSvcError(err, "Use --clear-active to clear active_vault as part of removal, or switch active vault first")
				}
			}
			return handleConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":            result.Name,
				"removed_path":    result.RemovedPath,
				"removed_legacy":  result.RemovedLegacy,
				"default_cleared": result.DefaultCleared,
				"active_cleared":  result.ActiveCleared,
				"config_path":     result.ConfigPath,
				"state_path":      result.StatePath,
			}, nil)
			return nil
		}

		fmt.Printf("Removed vault '%s' (%s)\n", result.Name, result.RemovedPath)
		if result.DefaultCleared {
			fmt.Println("Cleared default vault.")
		}
		if result.ActiveCleared {
			fmt.Println("Cleared active vault.")
		}
		fmt.Printf("config: %s\n", result.ConfigPath)
		if result.ActiveCleared {
			fmt.Printf("state:  %s\n", result.StatePath)
		}
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultListCmd)
	vaultCmd.AddCommand(vaultCurrentCmd)
	vaultCmd.AddCommand(vaultPathCmd)
	vaultCmd.AddCommand(vaultUseCmd)
	vaultCmd.AddCommand(vaultPinCmd)
	vaultCmd.AddCommand(vaultClearCmd)
	vaultCmd.AddCommand(vaultAddCmd)
	vaultCmd.AddCommand(vaultRemoveCmd)

	vaultAddCmd.Flags().BoolVar(&vaultAddReplace, "replace", false, "Replace existing vault path if name already exists")
	vaultAddCmd.Flags().BoolVar(&vaultAddPin, "pin", false, "Also set this vault as default_vault")
	vaultRemoveCmd.Flags().BoolVar(&vaultRemoveClearDefault, "clear-default", false, "Clear default_vault when removing the default")
	vaultRemoveCmd.Flags().BoolVar(&vaultRemoveClearActive, "clear-active", false, "Clear active_vault when removing the active vault")

	rootCmd.AddCommand(vaultCmd)
}
