package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/templatesvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage template files in directories.template",
	Long: `Manage template files under directories.template.

Use this command group for template file lifecycle operations:
- write/update template content
- list available template files
- delete template files safely`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var templateListCmd = newCanonicalLeafCommand("template_list", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderTemplateList,
})

var templateWriteCmd = newCanonicalLeafCommand("template_write", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildTemplateWriteArgs,
	RenderHuman: renderTemplateWrite,
})

var templateDeleteCmd = newCanonicalLeafCommand("template_delete", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderTemplateDelete,
})

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateWriteCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	rootCmd.AddCommand(templateCmd)
}

func buildTemplateWriteArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	if !cmd.Flags().Changed("content") {
		return nil, handleErrorMsg(ErrMissingArgument, "--content is required", "Provide template markdown with --content")
	}
	meta, _ := commands.EffectiveMeta("template_write")
	return buildCanonicalArgsForMeta(meta, cmd, args)
}

func renderTemplateList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	templateDir, _ := data["template_dir"].(string)
	templates, err := decodeSchemaValue[[]templatesvc.TemplateFileInfo](data["templates"])
	if err != nil {
		return err
	}

	if len(templates) == 0 {
		fmt.Println(ui.Starf("No template files found under %s", ui.FilePath(templateDir)))
		return nil
	}

	fmt.Printf("Template files (%s):\n", ui.FilePath(templateDir))
	for _, f := range templates {
		fmt.Printf("  - %s (%d bytes)\n", f.Path, f.SizeBytes)
	}
	return nil
}

func renderTemplateWrite(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Template %s: %s", data["status"], ui.FilePath(stringValue(data["path"]))))
	return nil
}

func renderTemplateDelete(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Deleted template %s -> %s", ui.FilePath(stringValue(data["deleted"])), ui.FilePath(stringValue(data["trash_path"]))))
	for _, w := range result.Warnings {
		fmt.Printf("warning: %s\n", w.Message)
	}
	return nil
}
