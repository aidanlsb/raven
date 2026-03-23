package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/templatesvc"
)

var (
	templateWriteContent string
	templateDeleteForce  bool
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

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List template files",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		result := executeCanonicalCommand("template_list", vaultPath, nil)
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		templateDir, _ := data["template_dir"].(string)
		templates, err := decodeSchemaValue[[]templatesvc.TemplateFileInfo](data["templates"])
		if err != nil {
			return err
		}

		if len(templates) == 0 {
			fmt.Printf("No template files found under %s\n", templateDir)
			return nil
		}

		fmt.Printf("Template files (%s):\n", templateDir)
		for _, f := range templates {
			fmt.Printf("  - %s (%d bytes)\n", f.Path, f.SizeBytes)
		}
		return nil
	},
}

var templateWriteCmd = &cobra.Command{
	Use:   "write <path>",
	Short: "Create or update a template file",
	Long: `Create or update a template file under directories.template.

This command replaces the full file body with --content.
Use it for both initial template creation and iterative updates.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		if !cmd.Flags().Changed("content") {
			return handleErrorMsg(ErrMissingArgument, "--content is required", "Provide template markdown with --content")
		}

		result := executeCanonicalCommand("template_write", vaultPath, map[string]interface{}{
			"path":    args[0],
			"content": templateWriteContent,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		fmt.Printf("Template %s: %s\n", data["status"], data["path"])
		return nil
	},
}

var templateDeleteCmd = &cobra.Command{
	Use:   "delete <path>",
	Short: "Delete a template file (moves to .trash)",
	Long: `Delete a template file under directories.template.

By default, this command blocks deletion when schema templates still reference
the file path. Use --force to bypass that check.

The file is moved to .trash/ for recovery.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		result := executeCanonicalCommand("template_delete", vaultPath, map[string]interface{}{
			"path":  args[0],
			"force": templateDeleteForce,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		fmt.Printf("Deleted template %s -> %s\n", data["deleted"], data["trash_path"])
		for _, w := range result.Warnings {
			fmt.Printf("warning: %s\n", w.Message)
		}
		return nil
	},
}

func init() {
	templateWriteCmd.Flags().StringVar(&templateWriteContent, "content", "", "Template file content (full file body)")
	templateDeleteCmd.Flags().BoolVar(&templateDeleteForce, "force", false, "Delete even if schema templates still reference this file")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateWriteCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	rootCmd.AddCommand(templateCmd)
}
