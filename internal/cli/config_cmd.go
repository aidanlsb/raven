package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
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
	result := executeCanonicalCommand("config_show", "", nil)
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	data := canonicalDataMap(result)
	configFile := stringValue(data["config_path"])
	stateFile := stringValue(data["state_path"])
	if !boolValue(data["exists"]) {
		fmt.Printf("Config file does not exist: %s\n", configFile)
		fmt.Println("Run 'rvn config init' to create it.")
		return nil
	}

	fmt.Printf("config: %s\n", configFile)
	fmt.Printf("state:  %s\n", stateFile)

	if v := strings.TrimSpace(stringValue(data["default_vault"])); v != "" {
		fmt.Printf("default_vault: %s\n", v)
	}
	if v := strings.TrimSpace(stringValue(data["state_file"])); v != "" {
		fmt.Printf("state_file: %s\n", v)
	}
	if v := strings.TrimSpace(stringValue(data["editor"])); v != "" {
		fmt.Printf("editor: %s\n", v)
	}
	if v := strings.TrimSpace(stringValue(data["editor_mode"])); v != "" {
		fmt.Printf("editor_mode: %s\n", v)
	}
	ui, _ := data["ui"].(map[string]interface{})
	if v := strings.TrimSpace(stringValue(ui["accent"])); v != "" {
		fmt.Printf("ui.accent: %s\n", v)
	}
	if v := strings.TrimSpace(stringValue(ui["code_theme"])); v != "" {
		fmt.Printf("ui.code_theme: %s\n", v)
	}
	vaults := stringMap(data["vaults"])
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
		result := executeCanonicalCommand("config_init", "", nil)
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data := canonicalDataMap(result)
		if !boolValue(data["created"]) {
			fmt.Printf("Config already exists: %s\n", stringValue(data["config_path"]))
		} else {
			fmt.Printf("Created config: %s\n", stringValue(data["config_path"]))
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set one or more global config.toml fields",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		argsMap := map[string]interface{}{}
		if cmd.Flags().Changed("editor") {
			argsMap["editor"] = configSetEditor
		}
		if cmd.Flags().Changed("editor-mode") {
			argsMap["editor-mode"] = configSetEditorMode
		}
		if cmd.Flags().Changed("state-file") {
			argsMap["state-file"] = configSetStateFile
		}
		if cmd.Flags().Changed("default-vault") {
			argsMap["default-vault"] = configSetDefaultVault
		}
		if cmd.Flags().Changed("ui-accent") {
			argsMap["ui-accent"] = configSetUIAccent
		}
		if cmd.Flags().Changed("ui-code-theme") {
			argsMap["ui-code-theme"] = configSetUICodeTheme
		}

		result := executeCanonicalCommand("config_set", "", argsMap)
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data := canonicalDataMap(result)
		fmt.Printf("Updated config: %s\n", stringValue(data["config_path"]))
		fmt.Printf("changed: %s\n", strings.Join(stringSliceFromAny(data["changed"]), ", "))
		return nil
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset",
	Short: "Clear one or more global config.toml fields",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("config_unset", "", map[string]interface{}{
			"editor":        configUnsetEditor,
			"editor-mode":   configUnsetEditorMode,
			"state-file":    configUnsetStateFile,
			"default-vault": configUnsetDefaultVault,
			"ui-accent":     configUnsetUIAccent,
			"ui-code-theme": configUnsetUICodeTheme,
		})
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data := canonicalDataMap(result)
		fmt.Printf("Updated config: %s\n", stringValue(data["config_path"]))
		fmt.Printf("cleared: %s\n", strings.Join(stringSliceFromAny(data["changed"]), ", "))
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

func stringMap(raw interface{}) map[string]string {
	switch value := raw.(type) {
	case map[string]string:
		out := make(map[string]string, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out
	case map[string]interface{}:
		out := make(map[string]string, len(value))
		for key, item := range value {
			text, ok := item.(string)
			if ok {
				out[key] = text
			}
		}
		return out
	default:
		return map[string]string{}
	}
}
