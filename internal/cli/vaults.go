package cli

import (
	"fmt"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/spf13/cobra"
)

var vaultsCmd = &cobra.Command{
	Use:   "vaults",
	Short: "List configured vaults",
	Long: `Lists all vaults configured in ~/.config/raven/config.toml.

Example config:
  default_vault = "personal"

  [vaults]
  personal = "/Users/you/notes"
  work = "/Users/you/work-notes"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config directly (not using cfg since we skip PreRun)
		var loadedCfg *config.Config
		var err error

		if configPath != "" {
			loadedCfg, err = config.LoadFrom(configPath)
		} else {
			loadedCfg, err = config.Load()
		}
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		vaults := loadedCfg.ListVaults()

		if len(vaults) == 0 {
			fmt.Println("No vaults configured.")
			fmt.Println()
			fmt.Println("Add vaults to ~/.config/raven/config.toml:")
			fmt.Println()
			fmt.Println("  default_vault = \"personal\"")
			fmt.Println()
			fmt.Println("  [vaults]")
			fmt.Println("  personal = \"/path/to/your/notes\"")
			return nil
		}

		defaultName := loadedCfg.DefaultVault
		// Handle legacy single vault
		if defaultName == "" && loadedCfg.Vault != "" {
			defaultName = "default"
		}

		for name, path := range vaults {
			marker := "  "
			if name == defaultName {
				marker = "* "
			}
			fmt.Printf("%s%-12s → %s\n", marker, name, path)
		}

		if defaultName != "" {
			fmt.Println()
			fmt.Println("* = default vault")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(vaultsCmd)
}
