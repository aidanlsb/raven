package cli

import (
	"fmt"
	"path/filepath"
	"time"

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
		start := time.Now()
		vaultPath := getVaultPath()

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		result, err := templatesvc.List(templatesvc.ListRequest{
			VaultPath:   vaultPath,
			TemplateDir: vaultCfg.GetTemplateDirectory(),
		})
		if err != nil {
			return mapTemplateSvcError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"template_dir": result.TemplateDir,
				"templates":    result.Templates,
			}, &Meta{Count: len(result.Templates), QueryTimeMs: elapsed})
			return nil
		}

		if len(result.Templates) == 0 {
			fmt.Printf("No template files found under %s\n", result.TemplateDir)
			return nil
		}

		fmt.Printf("Template files (%s):\n", result.TemplateDir)
		for _, f := range result.Templates {
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
		start := time.Now()
		vaultPath := getVaultPath()

		if !cmd.Flags().Changed("content") {
			return handleErrorMsg(ErrMissingArgument, "--content is required", "Provide template markdown with --content")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		result, err := templatesvc.Write(templatesvc.WriteRequest{
			VaultPath:   vaultPath,
			TemplateDir: vaultCfg.GetTemplateDirectory(),
			Path:        args[0],
			Content:     templateWriteContent,
		})
		if err != nil {
			return mapTemplateSvcError(err)
		}

		if result.Changed {
			maybeReindex(vaultPath, filepath.Join(vaultPath, filepath.FromSlash(result.Path)), vaultCfg)
		}

		elapsed := time.Since(start).Milliseconds()
		payload := map[string]interface{}{
			"path":         result.Path,
			"status":       result.Status,
			"template_dir": result.TemplateDir,
		}
		if isJSONOutput() {
			outputSuccess(payload, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Template %s: %s\n", result.Status, result.Path)
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
		start := time.Now()
		vaultPath := getVaultPath()

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		result, err := templatesvc.Delete(templatesvc.DeleteRequest{
			VaultPath:   vaultPath,
			TemplateDir: vaultCfg.GetTemplateDirectory(),
			Path:        args[0],
			Force:       templateDeleteForce,
		})
		if err != nil {
			return mapTemplateSvcError(err)
		}
		warnings := templateSvcWarnings(result.Warnings)

		elapsed := time.Since(start).Milliseconds()
		payload := map[string]interface{}{
			"deleted":      result.DeletedPath,
			"trash_path":   result.TrashPath,
			"forced":       result.Forced,
			"template_ids": result.TemplateIDs,
		}
		if isJSONOutput() {
			if len(warnings) > 0 {
				outputSuccessWithWarnings(payload, warnings, &Meta{QueryTimeMs: elapsed})
			} else {
				outputSuccess(payload, &Meta{QueryTimeMs: elapsed})
			}
			return nil
		}

		fmt.Printf("Deleted template %s -> %s\n", result.DeletedPath, result.TrashPath)
		for _, w := range warnings {
			fmt.Printf("warning: %s\n", w.Message)
		}
		return nil
	},
}

func mapTemplateSvcError(err error) error {
	svcErr, ok := templatesvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case templatesvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case templatesvc.CodeFileNotFound:
		return handleErrorMsg(ErrFileNotFound, svcErr.Message, svcErr.Suggestion)
	case templatesvc.CodeFileReadError:
		return handleError(ErrFileReadError, svcErr, svcErr.Suggestion)
	case templatesvc.CodeFileWriteError:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	case templatesvc.CodeFileOutsideVault:
		return handleError(ErrFileOutsideVault, svcErr, svcErr.Suggestion)
	case templatesvc.CodeSchemaInvalid:
		return handleError(ErrSchemaInvalid, svcErr, svcErr.Suggestion)
	case templatesvc.CodeValidationFailed:
		return handleErrorMsg(ErrValidationFailed, svcErr.Message, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func templateSvcWarnings(serviceWarnings []templatesvc.Warning) []Warning {
	if len(serviceWarnings) == 0 {
		return nil
	}
	warnings := make([]Warning, 0, len(serviceWarnings))
	for _, warning := range serviceWarnings {
		warnings = append(warnings, Warning{
			Code:    warning.Code,
			Message: warning.Message,
			Ref:     warning.Ref,
		})
	}
	return warnings
}

func init() {
	templateWriteCmd.Flags().StringVar(&templateWriteContent, "content", "", "Template file content (full file body)")
	templateDeleteCmd.Flags().BoolVar(&templateDeleteForce, "force", false, "Delete even if schema templates still reference this file")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateWriteCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	rootCmd.AddCommand(templateCmd)
}
