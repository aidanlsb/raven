package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/editsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	editConfirm   bool
	editEditsJSON string
)

type editSpec = editsvc.EditSpec

type editResult = editsvc.EditResult

type editCommandError struct {
	code       string
	message    string
	suggestion string
	details    interface{}
}

func (e *editCommandError) Error() string {
	return e.message
}

var editCmd = &cobra.Command{
	Use:   "edit <reference> [old_str] [new_str]",
	Short: commands.Registry["edit"].Description,
	Long:  commands.Registry["edit"].LongDesc,
	Args:  cobra.RangeArgs(1, 3),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		reference, edits, batchMode, err := parseEditArgs(args, editEditsJSON)
		if err != nil {
			return renderEditError(err)
		}

		vaultPath := getVaultPath()

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		resolvedRef, err := ResolveReference(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			return handleResolveError(err, reference)
		}

		filePath := resolvedRef.FilePath
		relPath, _ := filepath.Rel(vaultPath, filePath)

		content, err := os.ReadFile(filePath)
		if err != nil {
			return handleError("READ_ERROR", err, "")
		}

		newContent, results, err := applyEditsInMemory(string(content), relPath, edits)
		if err != nil {
			return renderEditError(err)
		}
		if len(results) == 0 {
			return handleErrorMsg(ErrInvalidInput, "no edits provided", "Provide at least one edit")
		}

		if !editConfirm {
			if jsonOutput {
				if batchMode {
					editsPreview := make([]map[string]interface{}, 0, len(results))
					for _, result := range results {
						editsPreview = append(editsPreview, map[string]interface{}{
							"index":   result.Index,
							"line":    result.Line,
							"old_str": result.OldStr,
							"new_str": result.NewStr,
							"preview": map[string]string{
								"before": result.Before,
								"after":  result.After,
							},
						})
					}

					outputJSON(Response{
						OK: true,
						Data: map[string]interface{}{
							"status": "preview",
							"path":   relPath,
							"count":  len(editsPreview),
							"edits":  editsPreview,
						},
						Meta: &Meta{},
					})
					return nil
				}

				result := results[0]
				outputJSON(Response{
					OK: true,
					Data: map[string]interface{}{
						"status": "preview",
						"path":   relPath,
						"line":   result.Line,
						"preview": map[string]string{
							"before": result.Before,
							"after":  result.After,
						},
					},
					Meta: &Meta{},
				})
				return nil
			}

			if batchMode {
				fmt.Printf("%s %s\n\n", ui.SectionHeader("Preview edits"), ui.FilePath(relPath))
				for _, result := range results {
					fmt.Println(ui.Muted.Render(fmt.Sprintf("EDIT %d (line %d):", result.Index, result.Line)))
					fmt.Println(ui.Muted.Render("BEFORE:"))
					fmt.Println(indent(result.Before, "  "))
					fmt.Println()
					fmt.Println(ui.Bold.Render("AFTER:"))
					fmt.Println(indent(result.After, "  "))
					fmt.Println()
				}
			} else {
				result := results[0]
				fmt.Printf("%s %s\n\n", ui.SectionHeader("Preview edit"), ui.FilePath(fmt.Sprintf("%s:%d", relPath, result.Line)))
				fmt.Println(ui.Muted.Render("BEFORE:"))
				fmt.Println(indent(result.Before, "  "))
				fmt.Println()
				fmt.Println(ui.Bold.Render("AFTER:"))
				fmt.Println(indent(result.After, "  "))
				fmt.Println()
			}
			fmt.Println(ui.Hint("Run with --confirm to apply this edit"))
			return nil
		}

		if err := atomicfile.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
			return handleError("WRITE_ERROR", err, "")
		}

		maybeReindex(vaultPath, filePath, vaultCfg)

		if jsonOutput {
			if batchMode {
				applied := make([]map[string]interface{}, 0, len(results))
				for _, result := range results {
					applied = append(applied, map[string]interface{}{
						"index":   result.Index,
						"line":    result.Line,
						"old_str": result.OldStr,
						"new_str": result.NewStr,
						"context": result.Context,
					})
				}

				outputSuccess(map[string]interface{}{
					"status": "applied",
					"path":   relPath,
					"count":  len(applied),
					"edits":  applied,
				}, nil)
				return nil
			}

			result := results[0]
			outputSuccess(map[string]interface{}{
				"status":  "applied",
				"path":    relPath,
				"line":    result.Line,
				"old_str": result.OldStr,
				"new_str": result.NewStr,
				"context": result.Context,
			}, nil)
			return nil
		}

		if batchMode {
			fmt.Println(ui.Checkf("Applied %d edits in %s", len(results), ui.FilePath(relPath)))
			fmt.Println()
			for _, result := range results {
				fmt.Println(ui.Muted.Render(fmt.Sprintf("EDIT %d (line %d):", result.Index, result.Line)))
				fmt.Println(indent(result.Context, "  "))
				fmt.Println()
			}
			return nil
		}

		result := results[0]
		fmt.Println(ui.Checkf("Applied edit in %s", ui.FilePath(fmt.Sprintf("%s:%d", relPath, result.Line))))
		fmt.Println()
		fmt.Println(ui.Muted.Render("Context:"))
		fmt.Println(indent(result.Context, "  "))
		return nil
	},
}

func parseEditArgs(args []string, editsJSON string) (string, []editSpec, bool, error) {
	reference := args[0]
	raw := strings.TrimSpace(editsJSON)
	if raw == "" {
		if len(args) != 3 {
			return "", nil, false, &editCommandError{
				code:       ErrMissingArgument,
				message:    "requires <reference> <old_str> <new_str>",
				suggestion: `Usage: rvn edit <reference> <old_str> <new_str> or rvn edit <reference> --edits-json '{"edits":[...]}'`,
			}
		}
		return reference, []editSpec{{OldStr: args[1], NewStr: args[2]}}, false, nil
	}

	if len(args) != 1 {
		return "", nil, false, &editCommandError{
			code:       ErrInvalidInput,
			message:    "--edits-json mode requires exactly one reference argument",
			suggestion: `Usage: rvn edit <reference> --edits-json '{"edits":[...]}'`,
		}
	}

	edits, err := parseEditsJSON(raw)
	if err != nil {
		return "", nil, false, err
	}
	return reference, edits, true, nil
}

func parseEditsJSON(raw string) ([]editSpec, error) {
	return editsvc.ParseEditsJSON(raw)
}

func applyEditsInMemory(content, relPath string, edits []editSpec) (string, []editResult, error) {
	return editsvc.ApplyEditsInMemory(content, relPath, edits)
}

func renderEditError(err error) error {
	var cmdErr *editCommandError
	if errors.As(err, &cmdErr) {
		if cmdErr.details != nil {
			return handleErrorWithDetails(cmdErr.code, cmdErr.message, cmdErr.suggestion, cmdErr.details)
		}
		return handleErrorMsg(cmdErr.code, cmdErr.message, cmdErr.suggestion)
	}

	svcErr, ok := editsvc.AsError(err)
	if ok {
		if len(svcErr.Details) > 0 {
			return handleErrorWithDetails(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, svcErr.Details)
		}
		return handleErrorMsg(string(svcErr.Code), svcErr.Message, svcErr.Suggestion)
	}

	return handleError(ErrInternal, err, "")
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func init() {
	editCmd.Flags().BoolVar(&editConfirm, "confirm", false, "Apply the edit (default: preview only)")
	editCmd.Flags().StringVar(&editEditsJSON, "edits-json", "", "JSON object with ordered edits, e.g. '{\"edits\":[{\"old_str\":\"from\",\"new_str\":\"to\"}]}'")
	rootCmd.AddCommand(editCmd)
}
