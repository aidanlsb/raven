package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global Raven config.toml settings",
	Long: `Manage global Raven config.toml settings.

Use this to initialize, inspect, and edit machine-level configuration.`,
	Args: cobra.NoArgs,
	RunE: canonicalGroupDefaultRunE("config_show", renderConfigShow),
}

var configInitCmd = newCanonicalLeafCommand("config_init", canonicalLeafOptions{
	RenderHuman: renderConfigInit,
})

var configSetCmd = newCanonicalLeafCommand("config_set", canonicalLeafOptions{
	RenderHuman: renderConfigSet,
})

var configUnsetCmd = newCanonicalLeafCommand("config_unset", canonicalLeafOptions{
	RenderHuman: renderConfigUnset,
})

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(newCanonicalLeafCommand("config_show", canonicalLeafOptions{
		RenderHuman: renderConfigShow,
	}))

	rootCmd.AddCommand(configCmd)
}

func renderConfigShow(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.ShowResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	if !data.Exists {
		fmt.Printf("%s %s\n", ui.Warning("Config file does not exist:"), ui.FilePath(data.ConfigPath))
		fmt.Println(ui.Hint("Run 'rvn config init' to create it."))
	}

	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(data.StatePath))
	fmt.Printf("%s %s\n", ui.Hint("default_vault:"), configDisplayValue(data.DefaultVault))
	fmt.Printf("%s %s\n", ui.Hint("editor:"), configDisplayValue(data.Editor))
	fmt.Printf("%s %s\n", ui.Hint("editor_mode:"), configDisplayValue(data.EditorMode))
	fmt.Printf("%s %s\n", ui.Hint("ui.markdown_style:"), configDisplayValue(data.UI.MarkdownStyle))
	if len(data.Vaults) == 0 {
		fmt.Printf("%s %s\n", ui.Hint("vaults:"), ui.Hint("(none)"))
		return nil
	}

	names := make([]string, 0, len(data.Vaults))
	for name := range data.Vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println(ui.SectionHeader("vaults"))
	for _, name := range names {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s = %s", name, ui.FilePath(data.Vaults[name]))))
	}

	return nil
}

func configDisplayValue(raw interface{}) string {
	value := strings.TrimSpace(stringValue(raw))
	if value == "" {
		return ui.Hint("(none)")
	}
	return value
}

func renderConfigInit(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.InitResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	if !data.Created {
		fmt.Println(ui.Starf("Config already exists: %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Checkf("Created config: %s", ui.FilePath(data.ConfigPath)))
	}
	return nil
}

func renderConfigSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.MutateResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.Checkf("Updated config: %s", ui.FilePath(data.ConfigPath)))
	fmt.Printf("%s %s\n", ui.Hint("changed:"), strings.Join(data.Changed, ", "))
	return nil
}

func renderConfigUnset(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[configsvc.MutateResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.Checkf("Updated config: %s", ui.FilePath(data.ConfigPath)))
	fmt.Printf("%s %s\n", ui.Hint("cleared:"), strings.Join(data.Changed, ", "))
	return nil
}
