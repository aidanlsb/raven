package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/template"
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

		templateDir := vaultCfg.GetTemplateDirectory()
		files, err := listTemplateFiles(vaultPath, templateDir)
		if err != nil {
			return handleError(ErrFileReadError, err, "Check directories.template and filesystem permissions")
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"template_dir": templateDir,
				"templates":    files,
			}, &Meta{Count: len(files), QueryTimeMs: elapsed})
			return nil
		}

		if len(files) == 0 {
			fmt.Printf("No template files found under %s\n", templateDir)
			return nil
		}

		fmt.Printf("Template files (%s):\n", templateDir)
		for _, f := range files {
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
		templateDir := vaultCfg.GetTemplateDirectory()

		fileRef, err := template.ResolveFileRef(args[0], templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir))
		}

		fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
		}

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return handleError(ErrFileWriteError, err, "Unable to create template directory")
		}

		status := "created"
		if existing, readErr := os.ReadFile(fullPath); readErr == nil {
			if string(existing) == templateWriteContent {
				status = "unchanged"
			} else {
				status = "updated"
			}
		} else if !os.IsNotExist(readErr) {
			return handleError(ErrFileReadError, readErr, "")
		}

		if status != "unchanged" {
			if err := atomicfile.WriteFile(fullPath, []byte(templateWriteContent), 0o644); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
			maybeReindex(vaultPath, fullPath, vaultCfg)
		}

		elapsed := time.Since(start).Milliseconds()
		result := map[string]interface{}{
			"path":         fileRef,
			"status":       status,
			"template_dir": templateDir,
		}
		if isJSONOutput() {
			outputSuccess(result, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Template %s: %s\n", status, fileRef)
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
		templateDir := vaultCfg.GetTemplateDirectory()

		fileRef, err := template.ResolveFileRef(args[0], templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir))
		}

		fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
		}
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("template file not found: %s", fileRef), "")
		} else if err != nil {
			return handleError(ErrFileReadError, err, "")
		}

		templateIDs, err := schemaTemplateRefsForFile(vaultPath, fileRef, templateDir)
		if err != nil {
			return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
		}
		if len(templateIDs) > 0 && !templateDeleteForce {
			return handleErrorMsg(
				ErrValidationFailed,
				fmt.Sprintf("template file %q is referenced by schema templates: %s", fileRef, strings.Join(templateIDs, ", ")),
				"Remove those template definitions first with `rvn schema template remove <template_id>` or use --force",
			)
		}

		trashRef, err := moveTemplateToTrash(vaultPath, fileRef)
		if err != nil {
			return handleError(ErrFileWriteError, err, "Unable to move template file to .trash")
		}

		var warnings []Warning
		db, err := openDatabaseWithConfig(vaultPath, vaultCfg)
		if err != nil {
			warnings = append(warnings, Warning{
				Code:    WarnIndexUpdateFailed,
				Message: fmt.Sprintf("failed to open index for cleanup: %v", err),
				Ref:     "Run 'rvn reindex' to rebuild the index",
			})
		} else {
			if err := db.RemoveFile(fileRef); err != nil {
				warnings = append(warnings, Warning{
					Code:    WarnIndexUpdateFailed,
					Message: fmt.Sprintf("failed to remove file from index: %v", err),
					Ref:     "Run 'rvn reindex' to rebuild the index",
				})
			}
			_ = db.Close()
		}

		elapsed := time.Since(start).Milliseconds()
		result := map[string]interface{}{
			"deleted":      fileRef,
			"trash_path":   trashRef,
			"forced":       templateDeleteForce,
			"template_ids": templateIDs,
		}
		if isJSONOutput() {
			if len(warnings) > 0 {
				outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
			} else {
				outputSuccess(result, &Meta{QueryTimeMs: elapsed})
			}
			return nil
		}

		fmt.Printf("Deleted template %s -> %s\n", fileRef, trashRef)
		for _, w := range warnings {
			fmt.Printf("warning: %s\n", w.Message)
		}
		return nil
	},
}

type templateFileInfo struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

func listTemplateFiles(vaultPath, templateDir string) ([]templateFileInfo, error) {
	root := filepath.Join(vaultPath, filepath.FromSlash(templateDir))
	if err := paths.ValidateWithinVault(vaultPath, root); err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []templateFileInfo{}, nil
	} else if err != nil {
		return nil, err
	}

	files := make([]templateFileInfo, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}
		files = append(files, templateFileInfo{
			Path:      filepath.ToSlash(rel),
			SizeBytes: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func schemaTemplateRefsForFile(vaultPath, fileRef, templateDir string) ([]string, error) {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return nil, err
	}

	var refs []string
	target := filepath.ToSlash(fileRef)
	for templateID, def := range sch.Templates {
		if def == nil {
			continue
		}
		candidate := filepath.ToSlash(strings.TrimSpace(def.File))
		resolved, err := template.ResolveFileRef(candidate, templateDir)
		if err == nil {
			candidate = filepath.ToSlash(resolved)
		}
		if candidate == target {
			refs = append(refs, templateID)
		}
	}

	sort.Strings(refs)
	return refs, nil
}

func moveTemplateToTrash(vaultPath, fileRef string) (string, error) {
	sourceAbs := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	trashRef, err := uniqueTrashRef(vaultPath, filepath.ToSlash(filepath.Join(".trash", filepath.FromSlash(fileRef))))
	if err != nil {
		return "", err
	}
	destAbs := filepath.Join(vaultPath, filepath.FromSlash(trashRef))

	if err := paths.ValidateWithinVault(vaultPath, destAbs); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(sourceAbs, destAbs); err != nil {
		return "", err
	}
	return trashRef, nil
}

func uniqueTrashRef(vaultPath, initial string) (string, error) {
	candidate := initial
	ext := filepath.Ext(initial)
	base := strings.TrimSuffix(initial, ext)

	for i := 0; i < 1000; i++ {
		candidateAbs := filepath.Join(vaultPath, filepath.FromSlash(candidate))
		if _, err := os.Stat(candidateAbs); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d%s", base, time.Now().UTC().UnixNano(), ext)
	}

	return "", fmt.Errorf("failed to generate unique trash path for %s", initial)
}

func init() {
	templateWriteCmd.Flags().StringVar(&templateWriteContent, "content", "", "Template file content (full file body)")
	templateDeleteCmd.Flags().BoolVar(&templateDeleteForce, "force", false, "Delete even if schema templates still reference this file")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateWriteCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	rootCmd.AddCommand(templateCmd)
}
