package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/configsvc"
)

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

func runConfigShow(cmd *cobra.Command, args []string) error {
	ctx, err := configsvc.ShowContext(configsvc.ContextOptions{
		ConfigPathOverride: configPath,
		StatePathOverride:  statePathFlag,
	})
	if err != nil {
		return handleConfigSvcError(err, "")
	}

	if isJSONOutput() {
		outputSuccess(ctx.Data(), nil)
		return nil
	}

	if !ctx.ConfigExists {
		fmt.Printf("Config file does not exist: %s\n", ctx.ConfigPath)
		fmt.Println("Run 'rvn config init' to create it.")
		return nil
	}

	fmt.Printf("config: %s\n", ctx.ConfigPath)
	fmt.Printf("state:  %s\n", ctx.StatePath)

	if v := strings.TrimSpace(ctx.Cfg.DefaultVault); v != "" {
		fmt.Printf("default_vault: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.Cfg.StateFile); v != "" {
		fmt.Printf("state_file: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.Cfg.Editor); v != "" {
		fmt.Printf("editor: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.Cfg.EditorMode); v != "" {
		fmt.Printf("editor_mode: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.Cfg.UI.Accent); v != "" {
		fmt.Printf("ui.accent: %s\n", v)
	}
	if v := strings.TrimSpace(ctx.Cfg.UI.CodeTheme); v != "" {
		fmt.Printf("ui.code_theme: %s\n", v)
	}
	vaults := ctx.Cfg.ListVaults()
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
		result, err := configsvc.Init(configsvc.InitRequest{
			ConfigPathOverride: configPath,
		})
		if err != nil {
			return handleConfigSvcError(err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"config_path": result.ConfigPath,
				"created":     result.Created,
			}, nil)
			return nil
		}

		if !result.Created {
			fmt.Printf("Config already exists: %s\n", result.ConfigPath)
		} else {
			fmt.Printf("Created config: %s\n", result.ConfigPath)
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set one or more global config.toml fields",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		req := configsvc.SetRequest{
			ContextOptions: configsvc.ContextOptions{
				ConfigPathOverride: configPath,
				StatePathOverride:  statePathFlag,
			},
		}
		if cmd.Flags().Changed("editor") {
			v := configSetEditor
			req.Editor = &v
		}
		if cmd.Flags().Changed("editor-mode") {
			v := configSetEditorMode
			req.EditorMode = &v
		}
		if cmd.Flags().Changed("state-file") {
			v := configSetStateFile
			req.StateFile = &v
		}
		if cmd.Flags().Changed("default-vault") {
			v := configSetDefaultVault
			req.DefaultVault = &v
		}
		if cmd.Flags().Changed("ui-accent") {
			v := configSetUIAccent
			req.UIAccent = &v
		}
		if cmd.Flags().Changed("ui-code-theme") {
			v := configSetUICodeTheme
			req.UICodeTheme = &v
		}

		result, err := configsvc.Set(req)
		if err != nil {
			if svcErr, ok := configsvc.AsError(err); ok && svcErr.Code == configsvc.CodeInvalidInput && strings.Contains(svcErr.Message, "not configured") {
				return handleConfigSvcError(err, "Run 'rvn vault list' to see configured vaults")
			}
			return handleConfigSvcError(err, "")
		}

		if isJSONOutput() {
			data := result.Context.Data()
			data["changed"] = result.Changed
			outputSuccess(data, nil)
			return nil
		}

		fmt.Printf("Updated config: %s\n", result.Context.ConfigPath)
		fmt.Printf("changed: %s\n", strings.Join(result.Changed, ", "))
		return nil
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset",
	Short: "Clear one or more global config.toml fields",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := configsvc.Unset(configsvc.UnsetRequest{
			ContextOptions: configsvc.ContextOptions{
				ConfigPathOverride: configPath,
				StatePathOverride:  statePathFlag,
			},
			Editor:       configUnsetEditor,
			EditorMode:   configUnsetEditorMode,
			StateFile:    configUnsetStateFile,
			DefaultVault: configUnsetDefaultVault,
			UIAccent:     configUnsetUIAccent,
			UICodeTheme:  configUnsetUICodeTheme,
		})
		if err != nil {
			return handleConfigSvcError(err, "Run 'rvn config init' first")
		}

		if isJSONOutput() {
			data := result.Context.Data()
			data["changed"] = result.Changed
			outputSuccess(data, nil)
			return nil
		}

		fmt.Printf("Updated config: %s\n", result.Context.ConfigPath)
		fmt.Printf("cleared: %s\n", strings.Join(result.Changed, ", "))
		return nil
	},
}

func handleConfigSvcError(err error, hint string) error {
	svcErr, ok := configsvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, hint)
	}

	code := ErrInternal
	switch svcErr.Code {
	case configsvc.CodeConfigInvalid:
		code = ErrConfigInvalid
	case configsvc.CodeVaultNotFound:
		code = ErrVaultNotFound
	case configsvc.CodeVaultNotSpecified:
		code = ErrVaultNotSpecified
	case configsvc.CodeFileNotFound:
		code = ErrFileNotFound
	case configsvc.CodeFileReadError:
		code = ErrFileReadError
	case configsvc.CodeFileWriteError:
		code = ErrFileWriteError
	case configsvc.CodeInvalidInput:
		code = ErrInvalidInput
	case configsvc.CodeMissingArgument:
		code = ErrMissingArgument
	case configsvc.CodeDuplicateName:
		code = ErrDuplicateName
	case configsvc.CodeConfirmationNeeded:
		code = ErrConfirmationRequired
	}

	return handleError(code, svcErr, hint)
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
