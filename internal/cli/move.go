package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	moveForce         bool
	moveUpdateRefs    bool
	moveSkipTypeCheck bool
	moveStdin         bool
	moveConfirm       bool
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
		ids, embedded, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		if len(ids) == 0 && len(embedded) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}
		return map[string]interface{}{
			"stdin":       true,
			"object_ids":  stringsToAny(append(ids, embedded...)),
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
	if boolValue(args["stdin"]) {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			VaultPath: vaultPath,
			Args:      args,
			Confirm:   moveConfirm,
		})
	}

	result := executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
	if isJSONOutput() || !result.OK {
		return result
	}
	data := canonicalDataMap(result)
	if !boolValue(data["needs_confirm"]) {
		return result
	}
	if !moveForce {
		for _, warning := range result.Warnings {
			fmt.Printf("⚠ Warning: %s\n", warning.Message)
			if warning.Ref != "" {
				fmt.Printf("  %s\n\n", warning.Ref)
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

func updateReference(vaultPath string, vaultCfg *config.VaultConfig, sourceID, oldRef, newRef string) error {
	return objectsvc.UpdateReference(vaultPath, vaultCfg, sourceID, oldRef, newRef)
}

func updateReferenceAtLine(vaultPath string, vaultCfg *config.VaultConfig, sourceID string, line int, oldRef, newRef string) error {
	return objectsvc.UpdateReferenceAtLine(vaultPath, vaultCfg, sourceID, line, oldRef, newRef)
}

func replaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot string) string {
	return objectsvc.ReplaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot)
}

func chooseReplacementRefBase(oldBase, sourceID, destID string, aliasSlugToID map[string]string, res *resolver.Resolver) string {
	return objectsvc.ChooseReplacementRefBase(oldBase, sourceID, destID, aliasSlugToID, res)
}

func init() {
	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompts")
	moveCmd.Flags().BoolVar(&moveUpdateRefs, "update-refs", true, "Update references to moved file")
	moveCmd.Flags().BoolVar(&moveSkipTypeCheck, "skip-type-check", false, "Skip type-directory mismatch warning")
	moveCmd.Flags().BoolVar(&moveStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	moveCmd.Flags().BoolVar(&moveConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
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
		fmt.Println("Cancelled.")
		return nil
	}
	if boolValue(data["bulk"]) || boolValue(data["stdin"]) {
		return renderCanonicalBulkResult(result)
	}
	source, _ := data["source"].(string)
	destination, _ := data["destination"].(string)
	fmt.Println(ui.Checkf("Moved %s → %s", ui.FilePath(source), ui.FilePath(destination)))
	if updatedRefs, ok := data["updated_refs"].([]string); ok && len(updatedRefs) > 0 {
		fmt.Printf("  Updated %d references\n", len(updatedRefs))
		return nil
	}
	if updatedRefs := stringSliceFromAny(data["updated_refs"]); len(updatedRefs) > 0 {
		fmt.Printf("  Updated %d references\n", len(updatedRefs))
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
