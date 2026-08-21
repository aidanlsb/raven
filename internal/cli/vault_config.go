package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

// vaultConfigCmd is the "vault config" subtree. Its command hierarchy is
// generated from registry metadata (CLIPath) via buildRegistrySubtree, so
// adding a new vault_config_* registry entry only requires registering its
// human RenderHuman hook below — no new hand-written Cobra vars or AddCommand
// wiring.
var vaultConfigCmd = buildVaultConfigCommand()

const vaultConfigLong = `Manage vault-level raven.yaml settings.

Use this command group for structured vault configuration instead of editing
raven.yaml directly.`

func buildVaultConfigCommand() *cobra.Command {
	return buildRegistrySubtree(registrySubtreeSpec{
		Prefix:    []string{"vault", "config"},
		VaultPath: getVaultPath,
		Root: registryGroup{
			Use:           "config",
			Short:         "Manage vault-level raven.yaml settings",
			Long:          vaultConfigLong,
			DefaultLeafID: "vault_config_show",
			DefaultRender: renderVaultConfigShow,
		},
		Groups: map[string]registryGroup{
			"vault config auto-reindex": {
				Short: "Manage auto_reindex in raven.yaml",
			},
			"vault config protected-prefixes": {
				Short:         "Manage protected_prefixes in raven.yaml",
				DefaultLeafID: "vault_config_protected_prefixes_list",
				DefaultRender: renderVaultConfigProtectedPrefixesList,
			},
			"vault config exclude": {
				Short:         "Manage exclude patterns in raven.yaml",
				DefaultLeafID: "vault_config_exclude_list",
				DefaultRender: renderVaultConfigExcludeList,
			},
			"vault config directories": {
				Short:         "Manage directories config in raven.yaml",
				DefaultLeafID: "vault_config_directories_get",
				DefaultRender: renderVaultConfigDirectoriesGet,
			},
			"vault config capture": {
				Short:         "Manage capture config in raven.yaml",
				DefaultLeafID: "vault_config_capture_get",
				DefaultRender: renderVaultConfigCaptureGet,
			},
			"vault config deletion": {
				Short:         "Manage deletion config in raven.yaml",
				DefaultLeafID: "vault_config_deletion_get",
				DefaultRender: renderVaultConfigDeletionGet,
			},
		},
		Renders: map[string]func(*cobra.Command, commandexec.Result) error{
			"vault_config_show":                      renderVaultConfigShow,
			"vault_config_auto_reindex_set":          renderVaultConfigAutoReindexSet,
			"vault_config_auto_reindex_unset":        renderVaultConfigAutoReindexUnset,
			"vault_config_protected_prefixes_list":   renderVaultConfigProtectedPrefixesList,
			"vault_config_protected_prefixes_add":    renderVaultConfigProtectedPrefixesAdd,
			"vault_config_protected_prefixes_remove": renderVaultConfigProtectedPrefixesRemove,
			"vault_config_exclude_list":              renderVaultConfigExcludeList,
			"vault_config_exclude_add":               renderVaultConfigExcludeAdd,
			"vault_config_exclude_remove":            renderVaultConfigExcludeRemove,
			"vault_config_directories_get":           renderVaultConfigDirectoriesGet,
			"vault_config_directories_set":           renderVaultConfigDirectoriesSet,
			"vault_config_directories_unset":         renderVaultConfigDirectoriesUnset,
			"vault_config_capture_get":               renderVaultConfigCaptureGet,
			"vault_config_capture_set":               renderVaultConfigCaptureSet,
			"vault_config_capture_unset":             renderVaultConfigCaptureUnset,
			"vault_config_deletion_get":              renderVaultConfigDeletionGet,
			"vault_config_deletion_set":              renderVaultConfigDeletionSet,
			"vault_config_deletion_unset":            renderVaultConfigDeletionUnset,
		},
	})
}

func renderVaultConfigShow(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigShowResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if !data.Exists {
		fmt.Println(ui.Hint("raven.yaml does not exist yet; showing effective defaults."))
	}

	autoReindexSource := "default"
	if data.AutoReindexExplicit {
		autoReindexSource = "explicit"
	}
	fmt.Printf("%s %t (%s)\n", ui.Hint("auto_reindex:"), data.AutoReindex, autoReindexSource)
	if data.DailyTemplate != "" {
		fmt.Printf("%s %s\n", ui.Hint("daily_template:"), data.DailyTemplate)
	}

	fmt.Println(ui.SectionHeader("directories"))
	fmt.Printf("%s %s\n", ui.Hint("daily:"), data.Directories.Daily)
	if data.Directories.Type != "" {
		fmt.Printf("%s %s\n", ui.Hint("type:"), data.Directories.Type)
	}
	if data.Directories.Page != "" {
		fmt.Printf("%s %s\n", ui.Hint("page:"), data.Directories.Page)
	}
	fmt.Printf("%s %s\n", ui.Hint("template:"), data.Directories.Template)

	fmt.Println(ui.SectionHeader("capture"))
	fmt.Printf("%s %s\n", ui.Hint("destination:"), data.Capture.Destination)
	if data.Capture.Heading != "" {
		fmt.Printf("%s %s\n", ui.Hint("heading:"), data.Capture.Heading)
	}

	fmt.Println(ui.SectionHeader("deletion"))
	fmt.Printf("%s %s\n", ui.Hint("behavior:"), data.Deletion.Behavior)
	fmt.Printf("%s %s\n", ui.Hint("trash_dir:"), data.Deletion.TrashDir)
	fmt.Printf("%s %v\n", ui.Hint("queries:"), data.QueriesCount)

	if len(data.ProtectedPrefixes) == 0 {
		fmt.Printf("%s %s\n", ui.Hint("protected_prefixes:"), ui.Hint("(none)"))
	} else {
		fmt.Println(ui.SectionHeader("protected_prefixes"))
		for _, prefix := range data.ProtectedPrefixes {
			fmt.Println(ui.Bullet(prefix))
		}
	}

	if len(data.Exclude) == 0 {
		fmt.Printf("%s %s\n", ui.Hint("exclude:"), ui.Hint("(none)"))
	} else {
		fmt.Println(ui.SectionHeader("exclude"))
		for _, pattern := range data.Exclude {
			fmt.Println(ui.Bullet(pattern))
		}
	}
	return nil
}

func renderVaultConfigAutoReindexSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigAutoReindexResult](result)
	if err != nil {
		return err
	}
	if data.Changed {
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Starf("auto_reindex already %t", data.AutoReindex))
	}
	fmt.Printf("%s %t\n", ui.Hint("auto_reindex:"), data.AutoReindex)
	return nil
}

func renderVaultConfigAutoReindexUnset(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigAutoReindexResult](result)
	if err != nil {
		return err
	}
	if data.Changed {
		fmt.Println(ui.Checkf("Cleared explicit auto_reindex in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("auto_reindex already using the default behavior."))
	}
	fmt.Printf("%s %t (default)\n", ui.Hint("auto_reindex:"), data.AutoReindex)
	return nil
}

func renderVaultConfigProtectedPrefixesList(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigProtectedPrefixesListResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if len(data.ProtectedPrefixes) == 0 {
		fmt.Println(ui.Star("No configured protected prefixes."))
		return nil
	}
	for _, prefix := range data.ProtectedPrefixes {
		fmt.Println(ui.Bullet(prefix))
	}
	return nil
}

func renderVaultConfigProtectedPrefixesAdd(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigProtectedPrefixAddResult](result)
	if err != nil {
		return err
	}
	if data.Changed {
		fmt.Println(ui.Checkf("Added protected prefix '%s'", data.Prefix))
	} else {
		fmt.Println(ui.Starf("Protected prefix '%s' already configured", data.Prefix))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	return nil
}

func renderVaultConfigProtectedPrefixesRemove(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigProtectedPrefixRemoveResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Removed protected prefix '%s'", data.Removed))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	return nil
}

func renderVaultConfigExcludeList(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigExcludeListResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if len(data.Exclude) == 0 {
		fmt.Println(ui.Star("No configured exclude patterns."))
		return nil
	}
	for _, pattern := range data.Exclude {
		fmt.Println(ui.Bullet(pattern))
	}
	return nil
}

func renderVaultConfigExcludeAdd(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigExcludeAddResult](result)
	if err != nil {
		return err
	}
	if data.Changed {
		fmt.Println(ui.Checkf("Added exclude pattern '%s'", data.Pattern))
	} else {
		fmt.Println(ui.Starf("Exclude pattern '%s' already configured", data.Pattern))
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	return nil
}

func renderVaultConfigExcludeRemove(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigExcludeRemoveResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Removed exclude pattern '%s'", data.Removed))
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	return nil
}

func renderVaultConfigDirectoriesGet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigDirectoriesResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if !data.Configured {
		fmt.Println(ui.Hint("directories block not explicitly configured; showing effective values."))
	}
	fmt.Printf("%s %s\n", ui.Hint("daily:"), data.Daily)
	if data.Type != "" {
		fmt.Printf("%s %s\n", ui.Hint("type:"), data.Type)
	}
	if data.Page != "" {
		fmt.Printf("%s %s\n", ui.Hint("page:"), data.Page)
	}
	fmt.Printf("%s %s\n", ui.Hint("template:"), data.Template)
	return nil
}

func renderVaultConfigDirectoriesSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigDirectoriesResult](result)
	if err != nil {
		return err
	}
	if data.Changed != nil && *data.Changed {
		fmt.Println(ui.Checkf("Updated directories in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("Directories config unchanged."))
	}
	return renderVaultConfigDirectoriesGet(nil, result)
}

func renderVaultConfigDirectoriesUnset(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigDirectoriesResult](result)
	if err != nil {
		return err
	}
	if data.Changed != nil && *data.Changed {
		fmt.Println(ui.Checkf("Cleared directories fields in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("Directories config unchanged."))
	}
	return renderVaultConfigDirectoriesGet(nil, result)
}

func renderVaultConfigCaptureGet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigCaptureResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if !data.Configured {
		fmt.Println(ui.Hint("capture block not explicitly configured; showing effective values."))
	}
	fmt.Printf("%s %s\n", ui.Hint("destination:"), data.Destination)
	if data.Heading != "" {
		fmt.Printf("%s %s\n", ui.Hint("heading:"), data.Heading)
	}
	return nil
}

func renderVaultConfigCaptureSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigCaptureResult](result)
	if err != nil {
		return err
	}
	if data.Changed != nil && *data.Changed {
		fmt.Println(ui.Checkf("Updated capture config in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("Capture config unchanged."))
	}
	return renderVaultConfigCaptureGet(nil, result)
}

func renderVaultConfigCaptureUnset(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigCaptureResult](result)
	if err != nil {
		return err
	}
	if data.Changed != nil && *data.Changed {
		fmt.Println(ui.Checkf("Cleared capture fields in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("Capture config unchanged."))
	}
	return renderVaultConfigCaptureGet(nil, result)
}

func renderVaultConfigDeletionGet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigDeletionResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(data.ConfigPath))
	if !data.Configured {
		fmt.Println(ui.Hint("deletion block not explicitly configured; showing effective values."))
	}
	fmt.Printf("%s %s\n", ui.Hint("behavior:"), data.Behavior)
	fmt.Printf("%s %s\n", ui.Hint("trash_dir:"), data.TrashDir)
	return nil
}

func renderVaultConfigDeletionSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigDeletionResult](result)
	if err != nil {
		return err
	}
	if data.Changed != nil && *data.Changed {
		fmt.Println(ui.Checkf("Updated deletion config in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("Deletion config unchanged."))
	}
	return renderVaultConfigDeletionGet(nil, result)
}

func renderVaultConfigDeletionUnset(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.VaultConfigDeletionResult](result)
	if err != nil {
		return err
	}
	if data.Changed != nil && *data.Changed {
		fmt.Println(ui.Checkf("Cleared deletion fields in %s", ui.FilePath(data.ConfigPath)))
	} else {
		fmt.Println(ui.Star("Deletion config unchanged."))
	}
	return renderVaultConfigDeletionGet(nil, result)
}
