package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	moveForce         bool
	moveUpdateRefs    bool
	moveSkipTypeCheck bool
	moveStdin         bool
	moveConfirm       bool
	moveDryRun        bool
)

var moveCmd = newCanonicalLeafCommand("move", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	Args:            cobra.MaximumNArgs(2),
	BuildArgs:       buildMoveArgs,
	Invoke:          invokeMove,
	RenderHuman:     renderMoveResult,
	SkipFlagBinding: true,
})

func buildMoveArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	if moveStdin {
		if len(args) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no destination provided", "Usage: rvn move --stdin <destination-directory/>")
		}
		destination := args[0]
		if !strings.HasSuffix(destination, "/") {
			return nil, handleErrorMsg(ErrInvalidInput,
				"destination must be a directory (end with /)",
				"Example: rvn move --stdin archive/projects/")
		}
		ids, sectionIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		if len(ids) == 0 && len(sectionIDs) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}
		return map[string]interface{}{
			"stdin":       true,
			"object_ids":  stringsToAny(append(ids, sectionIDs...)),
			"destination": destination,
			"update-refs": moveUpdateRefs,
		}, nil
	}

	if len(args) < 2 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires source and destination arguments", "Usage: rvn move <source> <destination>")
	}
	argsMap := sourceDestinationArgs(args[0], args[1])
	argsMap["update-refs"] = moveUpdateRefs
	if moveSkipTypeCheck {
		argsMap["skip-type-check"] = true
	}
	return argsMap, nil
}

func invokeMove(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Bulk move stays preview-first: changes apply only with --confirm.
	if boolValue(args["stdin"]) {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   moveConfirm,
		})
	}

	// Single-object move applies immediately; --dry-run previews instead.
	if moveDryRun {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Preview:   true,
		})
	}

	result := executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
	if !result.OK {
		return result
	}
	data := canonicalDataMap(result)
	if !boolValue(data["needs_confirm"]) {
		return result
	}

	// Type-directory mismatch. Non-interactive callers get the warning and must
	// re-run with --skip-type-check; interactive terminals prompt.
	if isJSONOutput() {
		return result
	}
	if !moveForce {
		for _, warning := range result.Warnings {
			fmt.Println(ui.Warningf("Warning: %s", warning.Message))
			if warning.Ref != "" {
				fmt.Printf("  %s\n\n", ui.FilePath(warning.Ref))
			}
		}
		if !promptForConfirm("Proceed anyway?") {
			return commandexec.Success(map[string]interface{}{"cancelled": true}, nil)
		}
	}

	retryArgs := cloneArgsMap(args)
	retryArgs["skip-type-check"] = true
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      retryArgs,
	})
}

func init() {
	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompts")
	moveCmd.Flags().BoolVar(&moveUpdateRefs, "update-refs", true, "Update references to moved file")
	moveCmd.Flags().BoolVar(&moveSkipTypeCheck, "skip-type-check", false, "Skip type-directory mismatch warning")
	moveCmd.Flags().BoolVar(&moveStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	moveCmd.Flags().BoolVar(&moveConfirm, "confirm", false, "Apply bulk move (without this flag, bulk shows preview only)")
	moveCmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "Preview a single-object move without applying it")
	moveCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveDefault,
	})
	rootCmd.AddCommand(moveCmd)
}

func renderMoveResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	if boolValue(data["cancelled"]) {
		fmt.Println(ui.Star("Cancelled."))
		return nil
	}
	if boolValue(data["bulk"]) || boolValue(data["stdin"]) {
		return renderCanonicalBulkResult(result)
	}
	source, _ := data["source"].(string)
	destination, _ := data["destination"].(string)
	if boolValue(data["preview"]) && !boolValue(data["needs_confirm"]) {
		fmt.Println(ui.Star(fmt.Sprintf("Would move %s → %s", ui.FilePath(source), ui.FilePath(destination))))
		if updatedRefs := stringSliceFromAny(data["updated_refs"]); len(updatedRefs) > 0 {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Would update %d references", len(updatedRefs))))
		}
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}
	fmt.Println(ui.Checkf("Moved %s → %s", ui.FilePath(source), ui.FilePath(destination)))
	if updatedRefs, ok := data["updated_refs"].([]string); ok && len(updatedRefs) > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Updated %d references", len(updatedRefs))))
		return nil
	}
	if updatedRefs := stringSliceFromAny(data["updated_refs"]); len(updatedRefs) > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Updated %d references", len(updatedRefs))))
	}
	return nil
}

func sourceDestinationArgs(source, destination string) map[string]interface{} {
	args := map[string]interface{}{}
	if strings.TrimSpace(source) != "" {
		args["source"] = source
	}
	if strings.TrimSpace(destination) != "" {
		args["destination"] = destination
	}
	return args
}

func cloneArgsMap(args map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}
