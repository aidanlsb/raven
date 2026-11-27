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

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage configured vaults and active selection",
	Long: `Manage configured vaults and active selection.

The active vault is stored in state.toml.
The default vault is stored in config.toml and used when no active vault is set.
If the active vault name is not configured, CLI vault resolution fails until it
is switched or cleared.`,
	Args: cobra.NoArgs,
	RunE: canonicalGroupDefaultRunE("vault_list", renderVaultList),
}

var vaultListCmd = newCanonicalLeafCommand("vault_list", canonicalLeafOptions{
	RenderHuman: renderVaultList,
})

var vaultUseCmd = newCanonicalLeafCommand("vault_use", canonicalLeafOptions{
	RenderHuman: renderVaultUse,
})

var vaultFocusCmd = newCanonicalLeafCommand("vault_focus", canonicalLeafOptions{
	RenderHuman: renderVaultFocus,
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
	vaultCmd.AddCommand(vaultStatsCmd)
	vaultCmd.AddCommand(vaultUseCmd)
	vaultCmd.AddCommand(vaultFocusCmd)
	vaultCmd.AddCommand(vaultPinCmd)
	vaultCmd.AddCommand(vaultClearCmd)
	vaultCmd.AddCommand(vaultAddCmd)
	vaultCmd.AddCommand(vaultRemoveCmd)
	vaultCmd.AddCommand(vaultConfigCmd)

	rootCmd.AddCommand(vaultCmd)
}

func renderVaultList(_ *cobra.Command, result commandexec.Result) error {
	if pathOnly, ok := result.Data.(configsvc.VaultPathResult); ok {
		fmt.Println(pathOnly.Path)
		return nil
	}

	data, err := commandResultData[configsvc.VaultListResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}

	rows := data.Vaults
	current := data.Current
	activeName := data.ActiveVault
	activeMissing := data.ActiveMissing
	configFile := data.ConfigPath
	stateFile := data.StatePath

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
	if current != nil {
		fmt.Printf(
			"%s %s -> %s (%s)\n",
			ui.Hint("current:"),
			ui.Bold.Render(current.Name),
			ui.FilePath(current.Path),
			current.Source,
		)
	} else {
		fmt.Printf("%s %s\n", ui.Hint("current:"), ui.Hint("none"))
	}
	fmt.Println(ui.Hint("> = active vault (state)"))
	fmt.Println(ui.Hint("* = default vault (config)"))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(configFile))
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(stateFile))
	if activeMissing {
		fmt.Println(ui.Warningf("active vault '%s' in state is not configured", activeName))
	}

	return nil
}

func renderVaultUse(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.VaultUseResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.Checkf("Active vault set to '%s' -> %s", data.ActiveVault, ui.FilePath(data.Path)))
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(data.StatePath))
	return nil
}

func renderVaultFocus(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.VaultFocusPayload](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	if data.Cleared {
		fmt.Println(ui.Check("MCP session focus clear request validated."))
	} else if data.Name != "" {
		fmt.Println(ui.Checkf("MCP session focus target validated: '%s' -> %s", data.Name, ui.FilePath(data.Path)))
	} else {
		fmt.Println(ui.Checkf("MCP session focus target validated: %s", ui.FilePath(data.Path)))
	}
	fmt.Println(ui.Hint("CLI validation does not change a running MCP server; invoke vault_focus through raven_invoke in that session."))
	return nil
}

func renderVaultClear(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.VaultClearResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	if data.Previous == "" {
		fmt.Println(ui.Star("Active vault already clear."))
	} else {
		fmt.Println(ui.Checkf("Cleared active vault '%s'.", data.Previous))
	}
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(data.StatePath))
	return nil
}

func renderVaultPin(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.VaultPinResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.Checkf("Default vault set to '%s' -> %s", data.DefaultVault, ui.FilePath(data.Path)))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	return nil
}

func renderVaultAdd(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.VaultAddResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	if data.Replaced {
		fmt.Println(ui.Checkf("Updated vault '%s' -> %s", data.Name, ui.FilePath(data.Path)))
	} else {
		fmt.Println(ui.Checkf("Added vault '%s' -> %s", data.Name, ui.FilePath(data.Path)))
	}
	if data.Pinned {
		fmt.Println(ui.Hint(fmt.Sprintf("Default vault set to '%s'.", data.Name)))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	return nil
}

func renderVaultRemove(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.VaultRemoveResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.Checkf("Removed vault '%s' (%s)", data.Name, ui.FilePath(data.RemovedPath)))
	if data.DefaultCleared {
		fmt.Println(ui.Hint("Cleared default vault."))
	}
	if data.ActiveCleared {
		fmt.Println(ui.Hint("Cleared active vault."))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if data.ActiveCleared {
		fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(data.StatePath))
	}
	return nil
}
