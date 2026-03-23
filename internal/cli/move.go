package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
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

var moveCmd = &cobra.Command{
	Use:   "move <source> <destination>",
	Short: "Move or rename an object within the vault",
	Long: `Move or rename a file/object within the vault.

Both source and destination must be within the vault. This command:
- Validates paths are within the vault (security constraint)
- Updates all references to the moved file if --update-refs is set
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Destination must be a directory (ending with /).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn move people/loki people/loki-archived      # Rename
  rvn move inbox/task.md projects/website/task.md # Move to subdirectory
  rvn move drafts/person.md people/freya.md --update-refs

Bulk examples:
  rvn query "object:project .status==archived" --ids | rvn move --stdin archive/projects/
  rvn query "object:project .status==archived" --ids | rvn move --stdin archive/projects/ --confirm`,
	Args: cobra.MaximumNArgs(2),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveDefault,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --stdin mode for bulk operations
		if moveStdin {
			return runMoveBulk(args, vaultPath)
		}

		// Single object mode - requires source and destination
		if len(args) < 2 {
			return handleErrorMsg(ErrMissingArgument, "requires source and destination arguments", "Usage: rvn move <source> <destination>")
		}

		return moveSingleObject(vaultPath, args[0], args[1])
	},
}

// runMoveBulk handles bulk move operations from stdin.
func runMoveBulk(args []string, vaultPath string) error {
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no destination provided", "Usage: rvn move --stdin <destination-directory/>")
	}
	destination := args[0]

	if !strings.HasSuffix(destination, "/") {
		return handleErrorMsg(ErrInvalidInput,
			"destination must be a directory (end with /)",
			"Example: rvn move --stdin archive/projects/")
	}

	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	argsMap := map[string]interface{}{
		"stdin":       true,
		"object_ids":  stringsToAny(append(ids, embedded...)),
		"destination": destination,
		"update-refs": moveUpdateRefs,
	}
	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "move",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      argsMap,
		Confirm:   moveConfirm,
	})
	return handleCanonicalMoveResult(vaultPath, sourceDestinationArgs("", destination), result, true)
}

// moveSingleObject handles single move operation (non-bulk mode).
func moveSingleObject(vaultPath, source, destination string) error {
	argsMap := sourceDestinationArgs(source, destination)
	argsMap["update-refs"] = moveUpdateRefs
	if moveSkipTypeCheck {
		argsMap["skip-type-check"] = true
	}

	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "move",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      argsMap,
	})
	return handleCanonicalMoveResult(vaultPath, argsMap, result, false)
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
	rootCmd.AddCommand(moveCmd)
}

func handleCanonicalMoveResult(vaultPath string, args map[string]interface{}, result commandexec.Result, bulk bool) error {
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
	if needsConfirm, _ := data["needs_confirm"].(bool); needsConfirm {
		if !moveForce {
			for _, warning := range result.Warnings {
				fmt.Printf("⚠ Warning: %s\n", warning.Message)
				if warning.Ref != "" {
					fmt.Printf("  %s\n\n", warning.Ref)
				}
			}
			if !promptForConfirm("Proceed anyway?") {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		retryArgs := cloneArgsMap(args)
		retryArgs["skip-type-check"] = true
		retryResult := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "move",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args:      retryArgs,
		})
		return handleCanonicalMoveResult(vaultPath, retryArgs, retryResult, false)
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
