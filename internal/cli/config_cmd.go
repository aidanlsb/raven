package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func runConfigShow(cmd *cobra.Command, args []string) error {
	result := executeCanonicalCommand("config_show", "", nil)
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	return renderConfigShow(cmd, result)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global Raven config.toml settings",
	Long: `Manage global Raven config.toml settings.

Use this to initialize, inspect, and edit machine-level configuration.`,
	Args: cobra.NoArgs,
	RunE: runConfigShow,
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

func renderConfigInit(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if !boolValue(data["created"]) {
		fmt.Printf("Config already exists: %s\n", stringValue(data["config_path"]))
	} else {
		fmt.Printf("Created config: %s\n", stringValue(data["config_path"]))
	}
	return nil
}

func renderConfigSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("Updated config: %s\n", stringValue(data["config_path"]))
	fmt.Printf("changed: %s\n", strings.Join(stringSliceFromAny(data["changed"]), ", "))
	return nil
}

func renderConfigUnset(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("Updated config: %s\n", stringValue(data["config_path"]))
	fmt.Printf("cleared: %s\n", strings.Join(stringSliceFromAny(data["changed"]), ", "))
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
