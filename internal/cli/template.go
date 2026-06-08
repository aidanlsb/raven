package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/shellquote"
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
	edit, _ := cmd.Flags().GetBool("edit")
	hasContent := cmd.Flags().Changed("content")
	if edit && hasContent {
		return nil, handleErrorMsg(ErrInvalidInput, "--edit and --content cannot be used together", "Use --edit for interactive editing or --content for scripted writes")
	}
	if !edit && !hasContent {
		return nil, handleErrorMsg(ErrMissingArgument, "--content or --edit is required", "Provide template markdown with --content or open an editor with --edit")
	}
	meta, _ := commands.EffectiveMeta("template_write")
	argsMap, err := buildCanonicalArgsForMeta(meta, cmd, args)
	if err != nil {
		return nil, err
	}
	if edit {
		content, err := editTemplateContent(stringValue(argsMap["path"]))
		if err != nil {
			return nil, err
		}
		argsMap["content"] = content
	}
	return argsMap, nil
}

func editTemplateContent(pathArg string) (string, error) {
	cfg := getConfig()
	editor := ""
	if cfg != nil {
		editor = strings.TrimSpace(cfg.GetEditor())
	}
	if editor == "" {
		return "", handleErrorMsg(ErrMissingArgument, "no editor configured", "Set 'editor' in config.toml or $EDITOR, then rerun with --edit")
	}

	vaultPath := getVaultPath()
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return "", handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}
	readResult, err := templatesvc.Read(templatesvc.ReadRequest{
		VaultPath:   vaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        pathArg,
	})
	if err != nil {
		return "", handleTemplateCLIError(err)
	}

	tmpDir, err := os.MkdirTemp("", "rvn-template-edit-*")
	if err != nil {
		return "", handleError(ErrFileWriteError, err, "Check temporary directory permissions and try again")
	}
	defer os.RemoveAll(tmpDir)

	ext := filepath.Ext(readResult.Path)
	if ext == "" {
		ext = ".md"
	}
	tmpPath := filepath.Join(tmpDir, "template"+ext)
	if err := os.WriteFile(tmpPath, []byte(readResult.Content), 0o600); err != nil {
		return "", handleError(ErrFileWriteError, err, "Check temporary directory permissions and try again")
	}

	if err := runBlockingEditor(editor, tmpPath); err != nil {
		return "", handleError(ErrInternal, err, "Make sure your editor command exits after saving, for example 'code --wait'")
	}

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", handleError(ErrFileReadError, err, "Check temporary directory permissions and try again")
	}
	return string(content), nil
}

func runBlockingEditor(editor, filePath string) error {
	var cmd *exec.Cmd
	if strings.Contains(editor, " ") {
		cmd = exec.Command("sh", "-c", editor+" "+shellquote.Quote(filePath))
	} else {
		cmd = exec.Command(editor, filePath)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func handleTemplateCLIError(err error) error {
	svcErr, ok := templatesvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}
	return handleErrorMsg(svcErr.Code, svcErr.Message, svcErr.Suggestion)
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

	fmt.Println(ui.SectionHeader(fmt.Sprintf("Template files (%s)", ui.FilePath(templateDir))))
	for _, f := range templates {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.FilePath(f.Path), ui.Hint(fmt.Sprintf("(%d bytes)", f.SizeBytes)))))
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
		fmt.Println(ui.Warning(w.Message))
	}
	return nil
}
