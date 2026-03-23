package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	addToFlag      string
	addHeadingFlag string
	addStdin       bool
	addConfirm     bool
)

var addCmd = &cobra.Command{
	Use:   "add <text>",
	Short: "Quick capture - append text to daily note or inbox",
	Long: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. Configure destination in raven.yaml.
Auto-reindex is ON by default; configure via auto_reindex in raven.yaml.

Bulk operations:
  Use --stdin to read object IDs from stdin and append text to each.
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn add "Call Odin about the Bifrost"
  rvn add "@due(tomorrow) Send the estimate"
  rvn add "Plan for tomorrow" --to tomorrow
  rvn add "Project idea" --to inbox.md
  rvn add "Fix parser edge case" --to project/raven --heading bugs-fixes
  rvn add "Capture under heading" --to project/raven --heading "### Bugs / Fixes"
  rvn add "Structured note" --to project/raven#bugs-fixes
  rvn add "Meeting notes" --to cursor       # Resolves to companies/cursor.md
  rvn add "Met with [[people/freya]]" --json

Bulk examples:
  rvn query "object:project .status==active" --ids | rvn add --stdin "Review scheduled for Q2"
  rvn query "object:project .status==active" --ids | rvn add --stdin "@reviewed(2026-01-07)" --confirm

Configuration (raven.yaml):
  capture:
    destination: daily      # "daily" or a file path
    heading: "## Captured"  # Optional heading to append under`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --stdin mode for bulk operations
		if addStdin {
			return runAddBulk(args, vaultPath)
		}

		// Single capture mode - requires text argument
		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "requires text argument", "Usage: rvn add <text>")
		}

		return addSingleCapture(vaultPath, args)
	},
}

// runAddBulk handles bulk add operations from stdin.
func runAddBulk(args []string, vaultPath string) error {
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no text to add", "Usage: rvn add --stdin <text>")
	}
	text := strings.Join(args, " ")

	fileIDs, embeddedIDs, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	ids := append(fileIDs, embeddedIDs...)
	if len(ids) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	argsMap := map[string]interface{}{
		"text":       formatCaptureLine(text),
		"stdin":      true,
		"object_ids": stringsToAny(ids),
	}
	if headingSpec := effectiveAddHeadingSpec(); headingSpec != "" {
		argsMap["heading"] = headingSpec
	}

	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "add",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      argsMap,
		Confirm:   addConfirm,
	})
	return handleCanonicalAddResult(result, true)
}

// addSingleCapture handles single capture mode (non-bulk).
func addSingleCapture(vaultPath string, args []string) error {
	text := strings.Join(args, " ")
	argsMap := map[string]interface{}{
		"text": formatCaptureLine(text),
	}
	if to := strings.TrimSpace(addToFlag); to != "" {
		argsMap["to"] = to
	}
	if headingSpec := effectiveAddHeadingSpec(); headingSpec != "" {
		argsMap["heading"] = headingSpec
	}

	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "add",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      argsMap,
	})
	return handleCanonicalAddResult(result, false)
}

func handleCanonicalAddResult(result commandexec.Result, bulk bool) error {
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	if bulk {
		return renderCanonicalBulkResult(result)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	relativePath, _ := data["file"].(string)
	fmt.Println(ui.Checkf("Added to %s", ui.FilePath(relativePath)))
	for _, warning := range result.Warnings {
		fmt.Printf("  ⚠ %s: %s\n", warning.Code, warning.Message)
		if warning.CreateCommand != "" {
			fmt.Printf("    → %s\n", warning.CreateCommand)
		}
	}
	return nil
}

func formatCaptureLine(text string) string {
	return text
}

func effectiveAddHeadingSpec() string {
	return strings.TrimSpace(addHeadingFlag)
}

func parseHeadingTextFromSpec(spec string) (string, bool) {
	trimmed := strings.TrimSpace(spec)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i >= len(trimmed) || trimmed[i] != ' ' {
		return "", false
	}
	headingText := strings.TrimSpace(trimmed[i:])
	if headingText == "" {
		return "", false
	}
	return headingText, true
}

func buildCreateObjectCommand(typeName, targetRaw string) string {
	title := filepath.Base(strings.TrimSpace(targetRaw))
	if title == "" || title == "." || title == "/" {
		title = "new-object"
	}
	return fmt.Sprintf("rvn new %s %q --json", typeName, title)
}

func init() {
	addCmd.Flags().StringVar(&addToFlag, "to", "", "Target file (path or reference like 'cursor')")
	addCmd.Flags().StringVar(&addHeadingFlag, "heading", "", "Target heading within destination (heading slug, object#heading ID, or markdown heading text)")
	addCmd.Flags().BoolVar(&addStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	addCmd.Flags().BoolVar(&addConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	if err := addCmd.RegisterFlagCompletionFunc("to", completeReferenceFlag(true)); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(addCmd)
}
