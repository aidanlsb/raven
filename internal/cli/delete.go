package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	deleteForce   bool
	deleteStdin   bool
	deleteConfirm bool
)

var deleteCmd = newCanonicalLeafCommand("delete", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	Args:            cobra.MaximumNArgs(1),
	BuildArgs:       buildDeleteArgs,
	Invoke:          invokeDelete,
	RenderHuman:     renderDeleteResult,
	SkipFlagBinding: true,
})

func buildDeleteArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	if deleteStdin {
		ids, embedded, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		if len(ids) == 0 && len(embedded) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}
		return map[string]interface{}{
			"stdin":      true,
			"object_ids": stringsToAny(append(ids, embedded...)),
		}, nil
	}

	if len(args) == 0 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires object-id argument", "Usage: rvn delete <object-id>")
	}
	return map[string]interface{}{
		"object_id": args[0],
	}, nil
}

func invokeDelete(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	if boolValue(args["stdin"]) {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   deleteConfirm,
		})
	}

	if isJSONOutput() {
		if !deleteConfirm {
			return executeCanonicalRequest(commandexec.Request{
				CommandID: commandID,
				VaultPath: vaultPath,
				Args:      args,
				Preview:   true,
			})
		}
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   true,
		})
	}

	if deleteForce || deleteConfirm {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   true,
		})
	}

	if !deleteForce {
		preview := executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Preview:   true,
		})
		if !preview.OK {
			return preview
		}
		if !renderDeletePreviewPrompt(preview) {
			return commandexec.Success(map[string]interface{}{"cancelled": true}, nil)
		}
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   true,
	})
}

func renderDeleteResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	if boolValue(data["cancelled"]) {
		fmt.Println("Cancelled.")
		return nil
	}
	if boolValue(data["bulk"]) || boolValue(data["stdin"]) {
		return renderCanonicalBulkResult(result)
	}
	behavior, _ := data["behavior"].(string)
	if behavior == "trash" {
		if trashPath, ok := data["trash_path"].(string); ok && strings.TrimSpace(trashPath) != "" {
			fmt.Println(ui.Checkf("Moved to %s", ui.FilePath(trashPath)))
			return nil
		}
	}
	if deleted, ok := data["deleted"].(string); ok {
		fmt.Println(ui.Checkf("Deleted %s", ui.FilePath(deleted)))
		return nil
	}
	return handleErrorMsg(ErrInternal, "command execution failed", "")
}

func renderDeletePreviewPrompt(result commandexec.Result) bool {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return false
	}
	objectID, _ := data["object_id"].(string)
	fmt.Printf("Delete %s?\n", objectID)

	backlinks := deletePreviewBacklinks(data["backlinks"])
	if len(backlinks) > 0 {
		fmt.Printf("  ⚠ Warning: Referenced by %d objects:\n", len(backlinks))
		for _, bl := range backlinks {
			line := 0
			if bl.Line != nil {
				line = *bl.Line
			}
			fmt.Printf("    - %s (line %d)\n", bl.SourceID, line)
		}
	}

	behavior, _ := data["behavior"].(string)
	fmt.Printf("\nBehavior: %s", behavior)
	if behavior == "trash" {
		if trashDir, ok := data["trash_dir"].(string); ok && strings.TrimSpace(trashDir) != "" {
			fmt.Printf(" (to %s/)\n", trashDir)
		} else {
			fmt.Println()
		}
	} else {
		fmt.Println()
	}

	return promptForConfirm("Confirm?")
}

func deletePreviewBacklinks(raw interface{}) []model.Reference {
	var backlinks []model.Reference
	_ = decodeResultData(raw, &backlinks)
	return backlinks
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	deleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	deleteCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(deleteCmd)
}
