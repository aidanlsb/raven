package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var vaultConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage vault-level raven.yaml settings",
	Long: `Manage vault-level raven.yaml settings.

Use this command group for structured vault configuration instead of editing
raven.yaml directly.`,
	Args: cobra.NoArgs,
	RunE: canonicalGroupDefaultRunE("vault_config_show", getVaultPath, renderVaultConfigShow),
}

var vaultConfigShowCmd = newCanonicalLeafCommand("vault_config_show", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigShow,
})

var vaultConfigAutoReindexCmd = &cobra.Command{
	Use:   "auto-reindex",
	Short: "Manage auto_reindex in raven.yaml",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var vaultConfigAutoReindexSetCmd = newCanonicalLeafCommand("vault_config_auto_reindex_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigAutoReindexSet,
})

var vaultConfigAutoReindexUnsetCmd = newCanonicalLeafCommand("vault_config_auto_reindex_unset", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigAutoReindexUnset,
})

var vaultConfigProtectedPrefixesCmd = &cobra.Command{
	Use:   "protected-prefixes",
	Short: "Manage protected_prefixes in raven.yaml",
	Args:  cobra.NoArgs,
	RunE:  canonicalGroupDefaultRunE("vault_config_protected_prefixes_list", getVaultPath, renderVaultConfigProtectedPrefixesList),
}

var vaultConfigProtectedPrefixesListCmd = newCanonicalLeafCommand("vault_config_protected_prefixes_list", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigProtectedPrefixesList,
})

var vaultConfigProtectedPrefixesAddCmd = newCanonicalLeafCommand("vault_config_protected_prefixes_add", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigProtectedPrefixesAdd,
})

var vaultConfigProtectedPrefixesRemoveCmd = newCanonicalLeafCommand("vault_config_protected_prefixes_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigProtectedPrefixesRemove,
})

var vaultConfigDirectoriesCmd = &cobra.Command{
	Use:   "directories",
	Short: "Manage directories config in raven.yaml",
	Args:  cobra.NoArgs,
	RunE:  canonicalGroupDefaultRunE("vault_config_directories_get", getVaultPath, renderVaultConfigDirectoriesGet),
}

var vaultConfigDirectoriesGetCmd = newCanonicalLeafCommand("vault_config_directories_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigDirectoriesGet,
})

var vaultConfigDirectoriesSetCmd = newCanonicalLeafCommand("vault_config_directories_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigDirectoriesSet,
})

var vaultConfigDirectoriesUnsetCmd = newCanonicalLeafCommand("vault_config_directories_unset", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigDirectoriesUnset,
})

var vaultConfigCaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Manage capture config in raven.yaml",
	Args:  cobra.NoArgs,
	RunE:  canonicalGroupDefaultRunE("vault_config_capture_get", getVaultPath, renderVaultConfigCaptureGet),
}

var vaultConfigCaptureGetCmd = newCanonicalLeafCommand("vault_config_capture_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigCaptureGet,
})

var vaultConfigCaptureSetCmd = newCanonicalLeafCommand("vault_config_capture_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigCaptureSet,
})

var vaultConfigCaptureUnsetCmd = newCanonicalLeafCommand("vault_config_capture_unset", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigCaptureUnset,
})

var vaultConfigDeletionCmd = &cobra.Command{
	Use:   "deletion",
	Short: "Manage deletion config in raven.yaml",
	Args:  cobra.NoArgs,
	RunE:  canonicalGroupDefaultRunE("vault_config_deletion_get", getVaultPath, renderVaultConfigDeletionGet),
}

var vaultConfigDeletionGetCmd = newCanonicalLeafCommand("vault_config_deletion_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigDeletionGet,
})

var vaultConfigDeletionSetCmd = newCanonicalLeafCommand("vault_config_deletion_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigDeletionSet,
})

var vaultConfigDeletionUnsetCmd = newCanonicalLeafCommand("vault_config_deletion_unset", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultConfigDeletionUnset,
})

func init() {
	vaultConfigAutoReindexCmd.AddCommand(vaultConfigAutoReindexSetCmd)
	vaultConfigAutoReindexCmd.AddCommand(vaultConfigAutoReindexUnsetCmd)

	vaultConfigProtectedPrefixesCmd.AddCommand(vaultConfigProtectedPrefixesListCmd)
	vaultConfigProtectedPrefixesCmd.AddCommand(vaultConfigProtectedPrefixesAddCmd)
	vaultConfigProtectedPrefixesCmd.AddCommand(vaultConfigProtectedPrefixesRemoveCmd)

	vaultConfigDirectoriesCmd.AddCommand(vaultConfigDirectoriesGetCmd)
	vaultConfigDirectoriesCmd.AddCommand(vaultConfigDirectoriesSetCmd)
	vaultConfigDirectoriesCmd.AddCommand(vaultConfigDirectoriesUnsetCmd)

	vaultConfigCaptureCmd.AddCommand(vaultConfigCaptureGetCmd)
	vaultConfigCaptureCmd.AddCommand(vaultConfigCaptureSetCmd)
	vaultConfigCaptureCmd.AddCommand(vaultConfigCaptureUnsetCmd)

	vaultConfigDeletionCmd.AddCommand(vaultConfigDeletionGetCmd)
	vaultConfigDeletionCmd.AddCommand(vaultConfigDeletionSetCmd)
	vaultConfigDeletionCmd.AddCommand(vaultConfigDeletionUnsetCmd)

	vaultConfigCmd.AddCommand(vaultConfigShowCmd)
	vaultConfigCmd.AddCommand(vaultConfigAutoReindexCmd)
	vaultConfigCmd.AddCommand(vaultConfigProtectedPrefixesCmd)
	vaultConfigCmd.AddCommand(vaultConfigDirectoriesCmd)
	vaultConfigCmd.AddCommand(vaultConfigCaptureCmd)
	vaultConfigCmd.AddCommand(vaultConfigDeletionCmd)
}

func renderVaultConfigShow(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	if !boolValue(data["exists"]) {
		fmt.Println(ui.Hint("raven.yaml does not exist yet; showing effective defaults."))
	}

	autoReindexSource := "default"
	if boolValue(data["auto_reindex_explicit"]) {
		autoReindexSource = "explicit"
	}
	fmt.Printf("%s %t (%s)\n", ui.Hint("auto_reindex:"), boolValue(data["auto_reindex"]), autoReindexSource)
	if dailyTemplate := stringValue(data["daily_template"]); dailyTemplate != "" {
		fmt.Printf("%s %s\n", ui.Hint("daily_template:"), dailyTemplate)
	}

	directories, _ := data["directories"].(map[string]interface{})
	fmt.Println(ui.SectionHeader("directories"))
	fmt.Printf("%s %s\n", ui.Hint("daily:"), stringValue(directories["daily"]))
	if v := stringValue(directories["type"]); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("type:"), v)
	}
	if v := stringValue(directories["page"]); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("page:"), v)
	}
	fmt.Printf("%s %s\n", ui.Hint("template:"), stringValue(directories["template"]))

	capture, _ := data["capture"].(map[string]interface{})
	fmt.Println(ui.SectionHeader("capture"))
	fmt.Printf("%s %s\n", ui.Hint("destination:"), stringValue(capture["destination"]))
	if v := stringValue(capture["heading"]); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("heading:"), v)
	}

	deletion, _ := data["deletion"].(map[string]interface{})
	fmt.Println(ui.SectionHeader("deletion"))
	fmt.Printf("%s %s\n", ui.Hint("behavior:"), stringValue(deletion["behavior"]))
	fmt.Printf("%s %s\n", ui.Hint("trash_dir:"), stringValue(deletion["trash_dir"]))
	fmt.Printf("%s %v\n", ui.Hint("queries:"), data["queries_count"])

	prefixes := stringSliceFromAny(data["protected_prefixes"])
	if len(prefixes) == 0 {
		fmt.Printf("%s %s\n", ui.Hint("protected_prefixes:"), ui.Hint("(none)"))
		return nil
	}

	fmt.Println(ui.SectionHeader("protected_prefixes"))
	for _, prefix := range prefixes {
		fmt.Println(ui.Bullet(prefix))
	}
	return nil
}

func renderVaultConfigAutoReindexSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Starf("auto_reindex already %t", boolValue(data["auto_reindex"])))
	}
	fmt.Printf("%s %t\n", ui.Hint("auto_reindex:"), boolValue(data["auto_reindex"]))
	return nil
}

func renderVaultConfigAutoReindexUnset(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Cleared explicit auto_reindex in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("auto_reindex already using the default behavior."))
	}
	fmt.Printf("%s %t (default)\n", ui.Hint("auto_reindex:"), boolValue(data["auto_reindex"]))
	return nil
}

func renderVaultConfigProtectedPrefixesList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	prefixes := stringSliceFromAny(data["protected_prefixes"])
	if len(prefixes) == 0 {
		fmt.Println(ui.Star("No configured protected prefixes."))
		return nil
	}
	for _, prefix := range prefixes {
		fmt.Println(ui.Bullet(prefix))
	}
	return nil
}

func renderVaultConfigProtectedPrefixesAdd(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Added protected prefix '%s'", stringValue(data["prefix"])))
	} else {
		fmt.Println(ui.Starf("Protected prefix '%s' already configured", stringValue(data["prefix"])))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	return nil
}

func renderVaultConfigProtectedPrefixesRemove(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Removed protected prefix '%s'", stringValue(data["removed"])))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	return nil
}

func renderVaultConfigDirectoriesGet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	if !boolValue(data["configured"]) {
		fmt.Println(ui.Hint("directories block not explicitly configured; showing effective values."))
	}
	fmt.Printf("%s %s\n", ui.Hint("daily:"), stringValue(data["daily"]))
	if v := stringValue(data["type"]); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("type:"), v)
	}
	if v := stringValue(data["page"]); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("page:"), v)
	}
	fmt.Printf("%s %s\n", ui.Hint("template:"), stringValue(data["template"]))
	return nil
}

func renderVaultConfigDirectoriesSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Updated directories in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("Directories config unchanged."))
	}
	return renderVaultConfigDirectoriesGet(nil, result)
}

func renderVaultConfigDirectoriesUnset(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Cleared directories fields in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("Directories config unchanged."))
	}
	return renderVaultConfigDirectoriesGet(nil, result)
}

func renderVaultConfigCaptureGet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	if !boolValue(data["configured"]) {
		fmt.Println(ui.Hint("capture block not explicitly configured; showing effective values."))
	}
	fmt.Printf("%s %s\n", ui.Hint("destination:"), stringValue(data["destination"]))
	if v := stringValue(data["heading"]); v != "" {
		fmt.Printf("%s %s\n", ui.Hint("heading:"), v)
	}
	return nil
}

func renderVaultConfigCaptureSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Updated capture config in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("Capture config unchanged."))
	}
	return renderVaultConfigCaptureGet(nil, result)
}

func renderVaultConfigCaptureUnset(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Cleared capture fields in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("Capture config unchanged."))
	}
	return renderVaultConfigCaptureGet(nil, result)
}

func renderVaultConfigDeletionGet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(stringValue(data["config_path"])))
	if !boolValue(data["configured"]) {
		fmt.Println(ui.Hint("deletion block not explicitly configured; showing effective values."))
	}
	fmt.Printf("%s %s\n", ui.Hint("behavior:"), stringValue(data["behavior"]))
	fmt.Printf("%s %s\n", ui.Hint("trash_dir:"), stringValue(data["trash_dir"]))
	return nil
}

func renderVaultConfigDeletionSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Updated deletion config in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("Deletion config unchanged."))
	}
	return renderVaultConfigDeletionGet(nil, result)
}

func renderVaultConfigDeletionUnset(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if boolValue(data["changed"]) {
		fmt.Println(ui.Checkf("Cleared deletion fields in %s", ui.FilePath(stringValue(data["config_path"]))))
	} else {
		fmt.Println(ui.Star("Deletion config unchanged."))
	}
	return renderVaultConfigDeletionGet(nil, result)
}
