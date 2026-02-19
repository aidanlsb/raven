package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
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

func loadVaultContext() (*vaultContext, error) {
	loadedCfg, resolvedConfigPath, err := loadGlobalConfigWithPath()
	if err != nil {
		return nil, err
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	resolvedStatePath := config.ResolveStatePath(statePathFlag, resolvedConfigPath, loadedCfg)
	state, err := config.LoadState(resolvedStatePath)
	if err != nil {
		return nil, err
	}

	return &vaultContext{
		cfg:        loadedCfg,
		state:      state,
		configPath: resolvedConfigPath,
		statePath:  resolvedStatePath,
	}, nil
}

func defaultVaultName(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.DefaultVault) != "" {
		return strings.TrimSpace(cfg.DefaultVault)
	}
	if strings.TrimSpace(cfg.Vault) != "" && len(cfg.Vaults) == 0 {
		return "default"
	}
	return ""
}

func vaultRows(cfg *config.Config, state *config.State) ([]vaultRow, string, string, bool) {
	vaults := cfg.ListVaults()
	defaultName := defaultVaultName(cfg)
	activeName := ""
	if state != nil {
		activeName = strings.TrimSpace(state.ActiveVault)
	}

	rows := make([]vaultRow, 0, len(vaults))
	names := make([]string, 0, len(vaults))
	for name := range vaults {
		names = append(names, name)
	}
	sort.Strings(names)

	activeMissing := activeName != ""
	for _, name := range names {
		rows = append(rows, vaultRow{
			Name:      name,
			Path:      vaults[name],
			IsDefault: name == defaultName,
			IsActive:  name == activeName,
		})
		if name == activeName {
			activeMissing = false
		}
	}

	return rows, defaultName, activeName, activeMissing
}

func resolveCurrentVault(cfg *config.Config, state *config.State) (*currentVaultInfo, error) {
	activeName := ""
	if state != nil {
		activeName = strings.TrimSpace(state.ActiveVault)
	}

	if activeName != "" {
		path, err := cfg.GetVaultPath(activeName)
		if err == nil {
			return &currentVaultInfo{
				Name:   activeName,
				Path:   path,
				Source: "active_vault",
			}, nil
		}
	}

	defaultPath, err := cfg.GetDefaultVaultPath()
	if err != nil {
		if activeName != "" {
			return nil, fmt.Errorf("active vault '%s' not found in config and no default vault configured", activeName)
		}
		return nil, err
	}

	source := "default_vault"
	activeMissing := false
	if activeName != "" {
		source = "default_vault_fallback"
		activeMissing = true
	}

	return &currentVaultInfo{
		Name:          defaultVaultName(cfg),
		Path:          defaultPath,
		Source:        source,
		ActiveMissing: activeMissing,
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
		ctx, err := loadVaultContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		current, err := resolveCurrentVault(ctx.cfg, ctx.state)
		if err != nil {
			return handleError(ErrVaultNotSpecified, err, "Use 'rvn vault use <name>' or set default_vault in config.toml")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":           current.Name,
				"path":           current.Path,
				"source":         current.Source,
				"active_missing": current.ActiveMissing,
				"config_path":    ctx.configPath,
				"state_path":     ctx.statePath,
			}, nil)
			return nil
		}

		fmt.Printf("current: %s\n", current.Name)
		fmt.Printf("path:    %s\n", current.Path)
		fmt.Printf("source:  %s\n", current.Source)
		if current.ActiveMissing {
			fmt.Printf("warning: active vault '%s' is missing; using default\n", strings.TrimSpace(ctx.state.ActiveVault))
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
		ctx, err := loadVaultContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		path, err := ctx.cfg.GetVaultPath(name)
		if err != nil {
			return handleError(ErrVaultNotFound, err, "Run 'rvn vault list' to see configured vaults")
		}

		ctx.state.ActiveVault = name
		if err := config.SaveState(ctx.statePath, ctx.state); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"active_vault": name,
				"path":         path,
				"state_path":   ctx.statePath,
			}, nil)
			return nil
		}

		fmt.Printf("Active vault set to '%s' -> %s\n", name, path)
		fmt.Printf("state: %s\n", ctx.statePath)
		return nil
	},
}

var vaultClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear active vault from state.toml",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadVaultContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		prev := strings.TrimSpace(ctx.state.ActiveVault)
		ctx.state.ActiveVault = ""
		if err := config.SaveState(ctx.statePath, ctx.state); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"cleared":    true,
				"previous":   prev,
				"state_path": ctx.statePath,
			}, nil)
			return nil
		}

		if prev == "" {
			fmt.Println("Active vault already clear.")
		} else {
			fmt.Printf("Cleared active vault '%s'.\n", prev)
		}
		fmt.Printf("state: %s\n", ctx.statePath)
		return nil
	},
}

var vaultPinCmd = &cobra.Command{
	Use:   "pin <name>",
	Short: "Set default_vault in config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		ctx, err := loadVaultContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		path, err := ctx.cfg.GetVaultPath(name)
		if err != nil {
			return handleError(ErrVaultNotFound, err, "Run 'rvn vault list' to see configured vaults")
		}

		ctx.cfg.DefaultVault = name
		if err := config.SaveTo(ctx.configPath, ctx.cfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"default_vault": name,
				"path":          path,
				"config_path":   ctx.configPath,
			}, nil)
			return nil
		}

		fmt.Printf("Default vault set to '%s' -> %s\n", name, path)
		fmt.Printf("config: %s\n", ctx.configPath)
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultListCmd)
	vaultCmd.AddCommand(vaultCurrentCmd)
	vaultCmd.AddCommand(vaultUseCmd)
	vaultCmd.AddCommand(vaultPinCmd)
	vaultCmd.AddCommand(vaultClearCmd)
	rootCmd.AddCommand(vaultCmd)
}
