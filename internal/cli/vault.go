package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/ui"
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

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage configured vaults and active selection",
	Long: `Manage configured vaults and active selection.

The active vault is stored in state.toml.
The default vault is stored in config.toml and used as fallback.`,
	Args: cobra.NoArgs,
	RunE: canonicalGroupDefaultRunE("vault_list", nil, renderVaultList),
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
	vaultCmd.AddCommand(vaultConfigCmd)

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
		fmt.Println(ui.Star("No vaults configured."))
		fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(configFile))
		fmt.Println()
		fmt.Println(ui.SectionHeader("Add vaults to config.toml"))
		fmt.Println()
		fmt.Println(ui.Bullet(ui.Hint(`default_vault = "personal"`)))
		fmt.Println()
		fmt.Println(ui.Bullet(ui.Hint("[vaults]")))
		fmt.Println(ui.Bullet(ui.Hint(`personal = "/path/to/your/notes"`)))
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
		fmt.Printf("%s %s -> %s\n", prefix, ui.Bold.Render(row.Name), ui.FilePath(row.Path))
	}

	fmt.Println()
	fmt.Println(ui.Hint("> = active vault (state)"))
	fmt.Println(ui.Hint("* = default vault (config)"))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(configFile))
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(stateFile))
	if activeMissing {
		fmt.Println(ui.Warningf("active vault '%s' in state is not configured", activeName))
	}

	return nil
}

func renderVaultCurrent(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("current:"), ui.Bold.Render(stringValue(data["name"])))
	fmt.Printf("%s %s\n", ui.Hint("path:"), ui.FilePath(stringValue(data["path"])))
	fmt.Printf("%s %s\n", ui.Hint("source:"), stringValue(data["source"]))
	if boolValue(data["active_missing"]) {
		fmt.Println(ui.Warningf("active vault '%s' is missing; using default", stringValue(data["active_vault"])))
	}
	return nil
}

func renderVaultUse(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Active vault set to '%s' -> %s", stringValue(data["active_vault"]), ui.FilePath(stringValue(data["path"]))))
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(stringValue(data["state_path"])))
	return nil
}

func renderVaultClear(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if stringValue(data["previous"]) == "" {
		fmt.Println(ui.Star("Active vault already clear."))
	} else {
		fmt.Println(ui.Checkf("Cleared active vault '%s'.", stringValue(data["previous"])))
	}
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(stringValue(data["state_path"])))
	return nil
}

func renderVaultPin(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Default vault set to '%s' -> %s", stringValue(data["default_vault"]), ui.FilePath(stringValue(data["path"]))))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	return nil
}

func renderVaultAdd(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["replaced"]) {
		fmt.Println(ui.Checkf("Updated vault '%s' -> %s", stringValue(data["name"]), ui.FilePath(stringValue(data["path"]))))
	} else {
		fmt.Println(ui.Checkf("Added vault '%s' -> %s", stringValue(data["name"]), ui.FilePath(stringValue(data["path"]))))
	}
	if boolValue(data["pinned"]) {
		fmt.Println(ui.Hint(fmt.Sprintf("Default vault set to '%s'.", stringValue(data["name"]))))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	return nil
}

func renderVaultRemove(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Removed vault '%s' (%s)", stringValue(data["name"]), ui.FilePath(stringValue(data["removed_path"]))))
	if boolValue(data["default_cleared"]) {
		fmt.Println(ui.Hint("Cleared default vault."))
	}
	if boolValue(data["active_cleared"]) {
		fmt.Println(ui.Hint("Cleared active vault."))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	if boolValue(data["active_cleared"]) {
		fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(stringValue(data["state_path"])))
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
