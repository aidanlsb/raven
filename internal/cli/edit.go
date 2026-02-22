package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	editConfirm   bool
	editEditsJSON string
)

type editSpec struct {
	OldStr string `json:"old_str"`
	NewStr string `json:"new_str"`
}

type editBatchInput struct {
	Edits []editSpec `json:"edits"`
}

type editResult struct {
	Index   int
	Line    int
	OldStr  string
	NewStr  string
	Before  string
	After   string
	Context string
}

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

		// Load vault config
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		// Resolve the reference using unified resolver
		resolvedRef, err := ResolveReference(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			return handleResolveError(err, reference)
		}

		filePath := resolvedRef.FilePath
		relPath, _ := filepath.Rel(vaultPath, filePath)

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			return handleError("READ_ERROR", err, "")
		}

		contentStr := string(content)

		newContent, results, err := applyEditsInMemory(contentStr, relPath, edits)
		if err != nil {
			return renderEditError(err)
		}
		if len(results) == 0 {
			return handleErrorMsg(ErrInvalidInput, "no edits provided", "Provide at least one edit")
		}

		if !editConfirm {
			// Preview mode
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
				// Add suggestion as a separate print since Meta doesn't have suggestion
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

		// Apply the edit
		if err := atomicfile.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
			return handleError("WRITE_ERROR", err, "")
		}

		// Auto-reindex if configured
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
		return reference, []editSpec{
			{
				OldStr: args[1],
				NewStr: args[2],
			},
		}, false, nil
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
		return "", nil, false, &editCommandError{
			code:       ErrInvalidInput,
			message:    "invalid --edits-json payload",
			suggestion: `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`,
			details: map[string]string{
				"error": err.Error(),
			},
		}
	}
	return reference, edits, true, nil
}

func parseEditsJSON(raw string) ([]editSpec, error) {
	var input editBatchInput
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing content")
		}
		return nil, err
	}
	if len(input.Edits) == 0 {
		return nil, fmt.Errorf("edits must contain at least one item")
	}
	for i, edit := range input.Edits {
		if edit.OldStr == "" {
			return nil, fmt.Errorf("edits[%d].old_str must be non-empty", i)
		}
	}
	return input.Edits, nil
}

func applyEditsInMemory(content, relPath string, edits []editSpec) (string, []editResult, error) {
	updated := content
	results := make([]editResult, 0, len(edits))

	for i, edit := range edits {
		count := strings.Count(updated, edit.OldStr)
		editIndex := i + 1
		if count == 0 {
			return "", nil, &editCommandError{
				code:       "STRING_NOT_FOUND",
				message:    "old_str not found in file",
				suggestion: "Check the exact string including whitespace",
				details: map[string]string{
					"path":       relPath,
					"edit_index": fmt.Sprintf("%d", editIndex),
					"old_str":    edit.OldStr,
				},
			}
		}
		if count > 1 {
			return "", nil, &editCommandError{
				code:       "MULTIPLE_MATCHES",
				message:    fmt.Sprintf("old_str found %d times in file", count),
				suggestion: "Include more surrounding context to make the match unique",
				details: map[string]string{
					"path":       relPath,
					"edit_index": fmt.Sprintf("%d", editIndex),
					"count":      fmt.Sprintf("%d", count),
				},
			}
		}

		matchIndex := strings.Index(updated, edit.OldStr)
		lineNumber := strings.Count(updated[:matchIndex], "\n") + 1
		beforeContext := extractContext(updated, matchIndex, len(edit.OldStr))
		afterContext := extractContextAfterReplace(updated, edit.OldStr, edit.NewStr, matchIndex)

		updated = strings.Replace(updated, edit.OldStr, edit.NewStr, 1)
		results = append(results, editResult{
			Index:   editIndex,
			Line:    lineNumber,
			OldStr:  edit.OldStr,
			NewStr:  edit.NewStr,
			Before:  beforeContext,
			After:   afterContext,
			Context: afterContext,
		})
	}

	return updated, results, nil
}

func renderEditError(err error) error {
	var cmdErr *editCommandError
	if errors.As(err, &cmdErr) {
		if cmdErr.details != nil {
			return handleErrorWithDetails(cmdErr.code, cmdErr.message, cmdErr.suggestion, cmdErr.details)
		}
		return handleErrorMsg(cmdErr.code, cmdErr.message, cmdErr.suggestion)
	}
	return handleError(ErrInternal, err, "")
}

// extractContext extracts ~3 lines of context around a match
func extractContext(content string, matchIndex int, matchLen int) string {
	lines := strings.Split(content, "\n")

	// Find line containing the match
	charCount := 0
	startLine := 0
	for i, line := range lines {
		if charCount+len(line)+1 > matchIndex {
			startLine = i
			break
		}
		charCount += len(line) + 1 // +1 for newline
	}

	// Get 1 line before and 2 lines after
	contextStart := startLine
	if contextStart > 0 {
		contextStart--
	}
	contextEnd := startLine + 3
	if contextEnd > len(lines) {
		contextEnd = len(lines)
	}

	return strings.Join(lines[contextStart:contextEnd], "\n")
}

// extractContextAfterReplace shows context after the replacement
func extractContextAfterReplace(content, oldStr, newStr string, matchIndex int) string {
	newContent := strings.Replace(content, oldStr, newStr, 1)
	// Find approximate position in new content
	newMatchIndex := matchIndex
	if newMatchIndex > len(newContent) {
		newMatchIndex = len(newContent) - 1
	}
	if newMatchIndex < 0 {
		newMatchIndex = 0
	}
	return extractContext(newContent, newMatchIndex, len(newStr))
}

// indent adds a prefix to each line
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
