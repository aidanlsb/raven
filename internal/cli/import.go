package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/importsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var importCmd = newCanonicalLeafCommand("import", canonicalLeafOptions{
	VaultPath: getVaultPath,
	Prepare:   prepareImportStdin,
	Invoke:    invokeImportWithStdin,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		return renderCanonicalImportResult(result)
	},
})

type importResult = importsvc.ResultItem

func prepareImportStdin(cmd *cobra.Command, args []string) ([]string, bool, error) {
	return args, false, nil
}

func invokeImportWithStdin(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	file, _ := cmd.Flags().GetString("file")
	var stdinData []byte
	if strings.TrimSpace(file) == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return commandexec.Failure(ErrInvalidInput, err.Error(), nil, "Expected a JSON array of objects or a single JSON object")
		}
		stdinData = data
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Stdin:     stdinData,
	})
}

// outputImportResults outputs the import results in human-readable or JSON format.
func outputImportResults(results []importResult, warnings []Warning) error {
	// Count outcomes
	var created, updated, skipped, errored int
	for _, r := range results {
		switch r.Action {
		case "created", "create":
			created++
		case "updated", "update":
			updated++
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}

	if isJSONOutput() {
		data := map[string]interface{}{
			"total":   len(results),
			"created": created,
			"updated": updated,
			"skipped": skipped,
			"errors":  errored,
			"items":   results,
		}
		if len(warnings) > 0 {
			return outputSuccessWithWarnings(data, warnings, nil)
		}
		return outputSuccess(data, nil)
	}

	// Human-readable output
	// Detect preview mode by checking if any action is "create" or "update" (future tense)
	isDryRun := false
	for _, r := range results {
		if r.Action == "create" || r.Action == "update" {
			isDryRun = true
			break
		}
	}

	if isDryRun {
		fmt.Println(ui.Bold.Render("Dry run — no changes made:"))
	}

	for _, r := range results {
		switch r.Action {
		case "created":
			fmt.Println(ui.Checkf("Created %s", ui.FilePath(r.File)))
		case "create":
			fmt.Printf("  %s %s\n", ui.Bold.Render("create"), ui.FilePath(r.File))
		case "updated":
			fmt.Println(ui.Checkf("Updated %s", ui.FilePath(r.File)))
		case "update":
			fmt.Printf("  %s %s\n", ui.Bold.Render("update"), ui.FilePath(r.File))
		case "skipped":
			fmt.Printf("  %s %s: %s\n", ui.Warning("skip"), r.ID, r.Reason)
		case "error":
			if r.Code != "" {
				fmt.Printf("  %s %s [%s]: %s\n", ui.Warning("error"), r.ID, r.Code, r.Reason)
			} else {
				fmt.Printf("  %s %s: %s\n", ui.Warning("error"), r.ID, r.Reason)
			}
		}
	}

	// Summary line
	var parts []string
	if created > 0 {
		parts = append(parts, fmt.Sprintf("%d created", created))
	}
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", updated))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if errored > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errored))
	}
	if len(parts) > 0 {
		fmt.Printf("\n%s\n", strings.Join(parts, ", "))
	}

	for _, w := range warnings {
		fmt.Printf("  %s\n", ui.Warning(w.Message))
	}

	return nil
}

func renderCanonicalImportResult(result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.ImportResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	return outputImportResults(data.Items, result.Warnings)
}

func init() {
	rootCmd.AddCommand(importCmd)
}
