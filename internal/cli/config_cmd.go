package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
)

type globalConfigContext struct {
	cfg          *config.Config
	configPath   string
	statePath    string
	configExists bool
}

var (
	configSetEditor       string
	configSetEditorMode   string
	configSetStateFile    string
	configSetDefaultVault string
	configSetUIAccent     string
	configSetUICodeTheme  string

	configUnsetEditor       bool
	configUnsetEditorMode   bool
	configUnsetStateFile    bool
	configUnsetDefaultVault bool
	configUnsetUIAccent     bool
	configUnsetUICodeTheme  bool
)

func loadGlobalConfigContextAllowMissing() (*globalConfigContext, error) {
	loadedCfg, resolvedConfigPath, exists, err := loadGlobalConfigAllowMissingWithPath()
	if err != nil {
		return nil, err
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	return &globalConfigContext{
		cfg:          loadedCfg,
		configPath:   resolvedConfigPath,
		statePath:    config.ResolveStatePath(statePathFlag, resolvedConfigPath, loadedCfg),
		configExists: exists,
	}, nil
}

func configData(ctx *globalConfigContext) map[string]interface{} {
	vaults := make(map[string]string)
	for name, path := range ctx.cfg.Vaults {
		vaults[name] = path
	}

	return map[string]interface{}{
		"config_path":   ctx.configPath,
		"state_path":    ctx.statePath,
		"exists":        ctx.configExists,
		"default_vault": strings.TrimSpace(ctx.cfg.DefaultVault),
		"state_file":    strings.TrimSpace(ctx.cfg.StateFile),
		"vault":         strings.TrimSpace(ctx.cfg.Vault),
		"vaults":        vaults,
		"editor":        strings.TrimSpace(ctx.cfg.Editor),
		"editor_mode":   strings.TrimSpace(ctx.cfg.EditorMode),
		"ui": map[string]interface{}{
			"accent":     strings.TrimSpace(ctx.cfg.UI.Accent),
			"code_theme": strings.TrimSpace(ctx.cfg.UI.CodeTheme),
		},
		"hooks": map[string]interface{}{
			"default_enabled": ctx.cfg.Hooks.DefaultEnabled,
			"vaults":          ctx.cfg.Hooks.Vaults,
		},
	}
}

func normalizeEditorMode(raw string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "auto", "terminal", "gui":
		return mode, true
	default:
		return "", false
	}
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	ctx, err := loadGlobalConfigContextAllowMissing()
	if err != nil {
		return handleError(ErrConfigInvalid, err, "")
	}

	if isJSONOutput() {
		outputSuccess(configData(ctx), nil)
		return nil
	}

	if !ctx.configExists {
		fmt.Printf("Config file does not exist: %s\n", ctx.configPath)
		fmt.Println("Run 'rvn config init' to create it.")
		return nil
	}

	fmt.Printf("config: %s\n", ctx.configPath)
	fmt.Printf("state:  %s\n", ctx.statePath)

	if v := strings.TrimSpace(ctx.cfg.DefaultVault); v != "" {
		fmt.Printf("default_vault: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.cfg.StateFile); v != "" {
		fmt.Printf("state_file: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.cfg.Editor); v != "" {
		fmt.Printf("editor: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.cfg.EditorMode); v != "" {
		fmt.Printf("editor_mode: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.cfg.UI.Accent); v != "" {
		fmt.Printf("ui.accent: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.cfg.UI.CodeTheme); v != "" {
		fmt.Printf("ui.code_theme: %s\n", v)
	}
	if ctx.cfg.Hooks.DefaultEnabled != nil {
		fmt.Printf("hooks.default_enabled: %t\n", *ctx.cfg.Hooks.DefaultEnabled)
	}
	if len(ctx.cfg.Hooks.Vaults) > 0 {
		names := make([]string, 0, len(ctx.cfg.Hooks.Vaults))
		for name := range ctx.cfg.Hooks.Vaults {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Println("hooks.vaults:")
		for _, name := range names {
			fmt.Printf("  %s = %t\n", name, ctx.cfg.Hooks.Vaults[name])
		}
	}

	vaults := ctx.cfg.ListVaults()
	if len(vaults) == 0 {
		fmt.Println("vaults: (none)")
		return nil
	}

	names := make([]string, 0, len(vaults))
	for name := range vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println("vaults:")
	for _, name := range names {
		fmt.Printf("  %s = %s\n", name, vaults[name])
	}

	return nil
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global Raven config.toml settings",
	Long: `Manage global Raven config.toml settings.

Use this to initialize, inspect, and edit machine-level configuration.`,
	Args: cobra.NoArgs,
	RunE: runConfigShow,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default global config.toml if missing",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := config.ResolveConfigPath(configPath)
		_, statErr := os.Stat(targetPath)
		existed := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return handleError(ErrFileReadError, statErr, "")
		}

		createdPath, err := config.CreateDefaultAt(targetPath)
		if err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"config_path": createdPath,
				"created":     !existed,
			}, nil)
			return nil
		}

		if existed {
			fmt.Printf("Config already exists: %s\n", createdPath)
		} else {
			fmt.Printf("Created config: %s\n", createdPath)
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set one or more global config.toml fields",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadGlobalConfigContextAllowMissing()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		changed := make([]string, 0, 6)

		if cmd.Flags().Changed("editor") {
			value := strings.TrimSpace(configSetEditor)
			if value == "" {
				return handleErrorMsg(ErrInvalidInput, "editor cannot be empty; use 'rvn config unset --editor' to clear it", "")
			}
			ctx.cfg.Editor = value
			changed = append(changed, "editor")
		}

		if cmd.Flags().Changed("editor-mode") {
			value, ok := normalizeEditorMode(configSetEditorMode)
			if !ok {
				return handleErrorMsg(ErrInvalidInput, "editor-mode must be one of: auto, terminal, gui", "")
			}
			ctx.cfg.EditorMode = value
			changed = append(changed, "editor_mode")
		}

		if cmd.Flags().Changed("state-file") {
			value := strings.TrimSpace(configSetStateFile)
			if value == "" {
				return handleErrorMsg(ErrInvalidInput, "state-file cannot be empty; use 'rvn config unset --state-file' to clear it", "")
			}
			ctx.cfg.StateFile = value
			changed = append(changed, "state_file")
		}

		if cmd.Flags().Changed("default-vault") {
			value := strings.TrimSpace(configSetDefaultVault)
			if value == "" {
				return handleErrorMsg(ErrInvalidInput, "default-vault cannot be empty; use 'rvn config unset --default-vault' to clear it", "")
			}
			if _, err := ctx.cfg.GetVaultPath(value); err != nil {
				return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("default-vault '%s' is not configured", value), "Run 'rvn vault list' to see configured vaults")
			}
			ctx.cfg.DefaultVault = value
			changed = append(changed, "default_vault")
		}

		if cmd.Flags().Changed("ui-accent") {
			value := strings.TrimSpace(configSetUIAccent)
			if value == "" {
				return handleErrorMsg(ErrInvalidInput, "ui-accent cannot be empty; use 'rvn config unset --ui-accent' to clear it", "")
			}
			ctx.cfg.UI.Accent = value
			changed = append(changed, "ui.accent")
		}

		if cmd.Flags().Changed("ui-code-theme") {
			value := strings.TrimSpace(configSetUICodeTheme)
			if value == "" {
				return handleErrorMsg(ErrInvalidInput, "ui-code-theme cannot be empty; use 'rvn config unset --ui-code-theme' to clear it", "")
			}
			ctx.cfg.UI.CodeTheme = value
			changed = append(changed, "ui.code_theme")
		}

		if len(changed) == 0 {
			return handleErrorMsg(ErrMissingArgument, "no fields provided; set at least one --editor/--editor-mode/--state-file/--default-vault/--ui-accent/--ui-code-theme", "")
		}

		if err := config.SaveTo(ctx.configPath, ctx.cfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		ctx.configExists = true
		if isJSONOutput() {
			data := configData(ctx)
			data["changed"] = changed
			outputSuccess(data, nil)
			return nil
		}

		fmt.Printf("Updated config: %s\n", ctx.configPath)
		fmt.Printf("changed: %s\n", strings.Join(changed, ", "))
		return nil
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset",
	Short: "Clear one or more global config.toml fields",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadGlobalConfigContextAllowMissing()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}
		if !ctx.configExists {
			return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("config file not found: %s", ctx.configPath), "Run 'rvn config init' first")
		}

		changed := make([]string, 0, 6)
		if configUnsetEditor {
			ctx.cfg.Editor = ""
			changed = append(changed, "editor")
		}
		if configUnsetEditorMode {
			ctx.cfg.EditorMode = ""
			changed = append(changed, "editor_mode")
		}
		if configUnsetStateFile {
			ctx.cfg.StateFile = ""
			changed = append(changed, "state_file")
		}
		if configUnsetDefaultVault {
			ctx.cfg.DefaultVault = ""
			changed = append(changed, "default_vault")
		}
		if configUnsetUIAccent {
			ctx.cfg.UI.Accent = ""
			changed = append(changed, "ui.accent")
		}
		if configUnsetUICodeTheme {
			ctx.cfg.UI.CodeTheme = ""
			changed = append(changed, "ui.code_theme")
		}

		if len(changed) == 0 {
			return handleErrorMsg(ErrMissingArgument, "no fields selected; pass one or more unset flags", "")
		}

		if err := config.SaveTo(ctx.configPath, ctx.cfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			data := configData(ctx)
			data["changed"] = changed
			outputSuccess(data, nil)
			return nil
		}

		fmt.Printf("Updated config: %s\n", ctx.configPath)
		fmt.Printf("cleared: %s\n", strings.Join(changed, ", "))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current global config.toml values",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
	})

	configSetCmd.Flags().StringVar(&configSetEditor, "editor", "", "Set editor command")
	configSetCmd.Flags().StringVar(&configSetEditorMode, "editor-mode", "", "Set editor mode (auto|terminal|gui)")
	configSetCmd.Flags().StringVar(&configSetStateFile, "state-file", "", "Set state.toml path (absolute or relative to config directory)")
	configSetCmd.Flags().StringVar(&configSetDefaultVault, "default-vault", "", "Set default_vault to a configured vault name")
	configSetCmd.Flags().StringVar(&configSetUIAccent, "ui-accent", "", "Set UI accent color (ANSI 0-255 or #RRGGBB)")
	configSetCmd.Flags().StringVar(&configSetUICodeTheme, "ui-code-theme", "", "Set markdown code theme name")

	configUnsetCmd.Flags().BoolVar(&configUnsetEditor, "editor", false, "Clear editor")
	configUnsetCmd.Flags().BoolVar(&configUnsetEditorMode, "editor-mode", false, "Clear editor_mode")
	configUnsetCmd.Flags().BoolVar(&configUnsetStateFile, "state-file", false, "Clear state_file")
	configUnsetCmd.Flags().BoolVar(&configUnsetDefaultVault, "default-vault", false, "Clear default_vault")
	configUnsetCmd.Flags().BoolVar(&configUnsetUIAccent, "ui-accent", false, "Clear ui.accent")
	configUnsetCmd.Flags().BoolVar(&configUnsetUICodeTheme, "ui-code-theme", false, "Clear ui.code_theme")

	rootCmd.AddCommand(configCmd)
}
