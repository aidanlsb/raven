package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
)

type vaultRow struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"is_default"`
	IsActive  bool   `json:"is_active"`
}

var (
	vaultAddReplace         bool
	vaultAddPin             bool
	vaultRemoveClearDefault bool
	vaultRemoveClearActive  bool
)

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

func resolveCurrentVault(cfg *config.Config, state *config.State) (*configsvc.CurrentVaultInfo, error) {
	return configsvc.ResolveCurrentVault(cfg, state)
}

func runVaultList(cmd *cobra.Command, args []string) error {
	result := executeCanonicalCommand("vault_list", "", nil)
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
	rows := vaultRowsFromAny(data["vaults"])
	activeName := stringValue(data["active_vault"])
	activeMissing := boolValue(data["active_missing"])
	configFile := stringValue(data["config_path"])
	stateFile := stringValue(data["state_path"])

	if len(rows) == 0 {
		fmt.Println("No vaults configured.")
		fmt.Printf("Config: %s\n", configFile)
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
	fmt.Printf("config: %s\n", configFile)
	fmt.Printf("state:  %s\n", stateFile)
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
		result := executeCanonicalCommand("vault_current", "", nil)
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
		fmt.Printf("current: %s\n", stringValue(data["name"]))
		fmt.Printf("path:    %s\n", stringValue(data["path"]))
		fmt.Printf("source:  %s\n", stringValue(data["source"]))
		if boolValue(data["active_missing"]) {
			fmt.Printf("warning: active vault '%s' is missing; using default\n", stringValue(data["active_vault"]))
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
		result := executeCanonicalCommand("vault_use", "", map[string]interface{}{"name": name})
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
		fmt.Printf("Active vault set to '%s' -> %s\n", stringValue(data["active_vault"]), stringValue(data["path"]))
		fmt.Printf("state: %s\n", stringValue(data["state_path"]))
		return nil
	},
}

var vaultClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear active vault from state.toml",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("vault_clear", "", nil)
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
		if stringValue(data["previous"]) == "" {
			fmt.Println("Active vault already clear.")
		} else {
			fmt.Printf("Cleared active vault '%s'.\n", stringValue(data["previous"]))
		}
		fmt.Printf("state: %s\n", stringValue(data["state_path"]))
		return nil
	},
}

var vaultPinCmd = &cobra.Command{
	Use:   "pin <name>",
	Short: "Set default_vault in config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		result := executeCanonicalCommand("vault_pin", "", map[string]interface{}{"name": name})
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
		fmt.Printf("Default vault set to '%s' -> %s\n", stringValue(data["default_vault"]), stringValue(data["path"]))
		fmt.Printf("config: %s\n", stringValue(data["config_path"]))
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
		result := executeCanonicalCommand("vault_add", "", map[string]interface{}{
			"name":    name,
			"path":    rawPath,
			"replace": vaultAddReplace,
			"pin":     vaultAddPin,
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
		if boolValue(data["replaced"]) {
			fmt.Printf("Updated vault '%s' -> %s\n", stringValue(data["name"]), stringValue(data["path"]))
		} else {
			fmt.Printf("Added vault '%s' -> %s\n", stringValue(data["name"]), stringValue(data["path"]))
		}
		if boolValue(data["pinned"]) {
			fmt.Printf("Default vault set to '%s'.\n", stringValue(data["name"]))
		}
		fmt.Printf("config: %s\n", stringValue(data["config_path"]))
		return nil
	},
}

var vaultRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a vault from config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		result := executeCanonicalCommand("vault_remove", "", map[string]interface{}{
			"name":          name,
			"clear-default": vaultRemoveClearDefault,
			"clear-active":  vaultRemoveClearActive,
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
		fmt.Printf("Removed vault '%s' (%s)\n", stringValue(data["name"]), stringValue(data["removed_path"]))
		if boolValue(data["default_cleared"]) {
			fmt.Println("Cleared default vault.")
		}
		if boolValue(data["active_cleared"]) {
			fmt.Println("Cleared active vault.")
		}
		fmt.Printf("config: %s\n", stringValue(data["config_path"]))
		if boolValue(data["active_cleared"]) {
			fmt.Printf("state:  %s\n", stringValue(data["state_path"]))
		}
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultListCmd)
	vaultCmd.AddCommand(vaultCurrentCmd)
	vaultCmd.AddCommand(vaultPathCmd)
	vaultCmd.AddCommand(vaultStatsCmd)
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

func vaultRowsFromAny(raw interface{}) []vaultRow {
	switch value := raw.(type) {
	case []vaultRow:
		return append([]vaultRow(nil), value...)
	case []configsvc.VaultRow:
		rows := make([]vaultRow, 0, len(value))
		for _, row := range value {
			rows = append(rows, vaultRow{
				Name:      row.Name,
				Path:      row.Path,
				IsDefault: row.IsDefault,
				IsActive:  row.IsActive,
			})
		}
		return rows
	case []interface{}:
		rows := make([]vaultRow, 0, len(value))
		for _, item := range value {
			rowMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			rows = append(rows, vaultRow{
				Name:      stringValue(rowMap["name"]),
				Path:      stringValue(rowMap["path"]),
				IsDefault: boolValue(rowMap["is_default"]),
				IsActive:  boolValue(rowMap["is_active"]),
			})
		}
		return rows
	default:
		return nil
	}
}
