package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
)

type vaultRow struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"is_default"`
	IsActive  bool   `json:"is_active"`
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

func resolveCurrentVault(cfg *config.Config, state *config.State) (*configsvc.CurrentVaultInfo, error) {
	return configsvc.ResolveCurrentVault(cfg, state)
}

func runVaultList(cmd *cobra.Command, args []string) error {
	result := executeCanonicalCommand("vault_list", "", nil)
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	return renderVaultList(cmd, result)
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

var vaultListCmd = newCanonicalLeafCommand("vault_list", canonicalLeafOptions{
	RenderHuman: renderVaultList,
})

var vaultCurrentCmd = newCanonicalLeafCommand("vault_current", canonicalLeafOptions{
	RenderHuman: renderVaultCurrent,
})

var vaultUseCmd = newCanonicalLeafCommand("vault_use", canonicalLeafOptions{
	RenderHuman: renderVaultUse,
})

var vaultClearCmd = newCanonicalLeafCommand("vault_clear", canonicalLeafOptions{
	RenderHuman: renderVaultClear,
})

var vaultPinCmd = newCanonicalLeafCommand("vault_pin", canonicalLeafOptions{
	RenderHuman: renderVaultPin,
})

var vaultAddCmd = newCanonicalLeafCommand("vault_add", canonicalLeafOptions{
	RenderHuman: renderVaultAdd,
})

var vaultRemoveCmd = newCanonicalLeafCommand("vault_remove", canonicalLeafOptions{
	RenderHuman: renderVaultRemove,
})

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

	rootCmd.AddCommand(vaultCmd)
}

func renderVaultList(_ *cobra.Command, result commandexec.Result) error {
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

func renderVaultCurrent(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("current: %s\n", stringValue(data["name"]))
	fmt.Printf("path:    %s\n", stringValue(data["path"]))
	fmt.Printf("source:  %s\n", stringValue(data["source"]))
	if boolValue(data["active_missing"]) {
		fmt.Printf("warning: active vault '%s' is missing; using default\n", stringValue(data["active_vault"]))
	}
	return nil
}

func renderVaultUse(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("Active vault set to '%s' -> %s\n", stringValue(data["active_vault"]), stringValue(data["path"]))
	fmt.Printf("state: %s\n", stringValue(data["state_path"]))
	return nil
}

func renderVaultClear(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if stringValue(data["previous"]) == "" {
		fmt.Println("Active vault already clear.")
	} else {
		fmt.Printf("Cleared active vault '%s'.\n", stringValue(data["previous"]))
	}
	fmt.Printf("state: %s\n", stringValue(data["state_path"]))
	return nil
}

func renderVaultPin(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("Default vault set to '%s' -> %s\n", stringValue(data["default_vault"]), stringValue(data["path"]))
	fmt.Printf("config: %s\n", stringValue(data["config_path"]))
	return nil
}

func renderVaultAdd(_ *cobra.Command, result commandexec.Result) error {
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
}

func renderVaultRemove(_ *cobra.Command, result commandexec.Result) error {
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
