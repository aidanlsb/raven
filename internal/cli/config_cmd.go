package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global Raven config.toml settings",
	Long: `Manage global Raven config.toml settings.

Use this to initialize, inspect, and edit machine-level configuration.`,
	Args: cobra.NoArgs,
	RunE: canonicalGroupDefaultRunE("config_show", nil, renderConfigShow),
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
	data := canonicalDataMap(result)
	configFile := stringValue(data["config_path"])
	stateFile := stringValue(data["state_path"])
	if !boolValue(data["exists"]) {
		fmt.Printf("%s %s\n", ui.Warning("Config file does not exist:"), ui.FilePath(configFile))
		fmt.Println(ui.Hint("Run 'rvn config init' to create it."))
		return nil
	}

	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(configFile))
	fmt.Printf("%s %s\n", ui.Hint("state:"), ui.FilePath(stateFile))

	if v := strings.TrimSpace(stringValue(data["default_vault"])); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("default_vault:"), v)
	}
	if v := strings.TrimSpace(stringValue(data["state_file"])); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("state_file:"), ui.FilePath(v))
	}
	if v := strings.TrimSpace(stringValue(data["editor"])); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("editor:"), v)
	}
	if v := strings.TrimSpace(stringValue(data["editor_mode"])); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("editor_mode:"), v)
	}
	uiConfig, _ := data["ui"].(map[string]interface{})
	if v := strings.TrimSpace(stringValue(uiConfig["accent"])); v != "" {
		fmt.Printf("ui.accent: %s\n", v)
	}
	if v := strings.TrimSpace(stringValue(uiConfig["code_theme"])); v != "" {
		fmt.Printf("ui.code_theme: %s\n", v)
	}
	vaults := stringMap(data["vaults"])
	if len(vaults) == 0 {
		fmt.Printf("%s %s\n", ui.Hint("vaults:"), ui.Hint("(none)"))
		return nil
	}

	names := make([]string, 0, len(vaults))
	for name := range vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println(ui.SectionHeader("vaults"))
	for _, name := range names {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s = %s", name, ui.FilePath(vaults[name]))))
	}

	return nil
}

func renderConfigInit(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if !boolValue(data["created"]) {
		fmt.Println(ui.Starf("Config already exists: %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Checkf("Created config: %s", ui.FilePath(stringValue(data["config_path"]))))
	}
	return nil
}

func renderConfigSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Updated config: %s", ui.FilePath(stringValue(data["config_path"]))))
	fmt.Printf("%s %s\n", ui.Hint("changed:"), strings.Join(stringSliceFromAny(data["changed"]), ", "))
	return nil
}

func renderConfigUnset(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Updated config: %s", ui.FilePath(stringValue(data["config_path"]))))
	fmt.Printf("%s %s\n", ui.Hint("cleared:"), strings.Join(stringSliceFromAny(data["changed"]), ", "))
	return nil
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
