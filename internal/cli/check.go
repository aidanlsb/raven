package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

// checkCmd is the validate-only parent. Repairs live in the `fix` and
// `create-missing` subcommands (see #91). All three are thin canonical leaves
// that delegate execution to commandimpl/checksvc; the CLI only builds args,
// prompts, and renders.
var checkCmd = newCanonicalLeafCommand("check", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	BuildArgs:    buildCheckValidateArgs,
	HandleError:  handleCheckLeafFailure,
	HandleResult: handleCheckValidateResult,
})

var checkFixCmd = newCanonicalLeafCommand("check_fix", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	BuildArgs:    buildCheckFixArgs,
	Invoke:       invokeCheckMutation,
	HandleError:  handleCheckLeafFailure,
	HandleResult: handleCheckFixResult,
})

var checkCreateMissingCmd = newCanonicalLeafCommand("check create-missing", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	BuildArgs:    buildCheckCreateMissingArgs,
	Invoke:       invokeCheckMutation,
	HandleError:  handleCheckLeafFailure,
	HandleResult: handleCheckCreateMissingResult,
})

// checkScopeArgs collects the canonical scope arguments shared by `check` and
// `check fix` from a command's own flag set, so no state is shared between the
// parent and its subcommands.
func checkScopeArgs(cmd *cobra.Command, args []string) map[string]interface{} {
	argsMap := map[string]interface{}{}
	if len(args) > 0 {
		argsMap["path"] = args[0]
	}
	if value, _ := cmd.Flags().GetString("type"); value != "" {
		argsMap["type"] = value
	}
	if value, _ := cmd.Flags().GetString("trait"); value != "" {
		argsMap["trait"] = value
	}
	if value, _ := cmd.Flags().GetString("issues"); value != "" {
		argsMap["issues"] = value
	}
	if value, _ := cmd.Flags().GetString("exclude"); value != "" {
		argsMap["exclude"] = value
	}
	if value, _ := cmd.Flags().GetBool("errors-only"); value {
		argsMap["errors-only"] = true
	}
	return argsMap
}

func buildCheckValidateArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	return checkScopeArgs(cmd, args), nil
}

func buildCheckFixArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	return checkScopeArgs(cmd, args), nil
}

func buildCheckCreateMissingArgs(_ *cobra.Command, _ []string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// invokeCheckMutation drives the mutating check subcommands. `check fix` honors
// --confirm directly. `check create-missing` in interactive (non-JSON) mode
// always runs a preview and then applies via the prompt flow, so --confirm is
// only meaningful for the non-interactive JSON path.
func invokeCheckMutation(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm, _ := cmd.Flags().GetBool("confirm")
	if commandID == "check create-missing" && !isJSONOutput() {
		confirm = false
	}
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   confirm,
	})
}

func handleCheckLeafFailure(result commandexec.Result) error {
	if result.Error == nil {
		return handleErrorMsg(ErrInternal, "check failed", "")
	}
	if result.Error.Details != nil {
		return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
	}
	return handleErrorMsg(result.Error.Code, result.Error.Message, result.Error.Suggestion)
}

func handleCheckValidateResult(cmd *cobra.Command, result commandexec.Result) error {
	return handleCheckResult(cmd, result, func() error {
		byFile, _ := cmd.Flags().GetBool("by-file")
		verbose, _ := cmd.Flags().GetBool("verbose")
		renderCanonicalCheckValidate(result, byFile, verbose)
		return nil
	})
}

func handleCheckFixResult(cmd *cobra.Command, result commandexec.Result) error {
	return handleCheckResult(cmd, result, func() error {
		renderCanonicalCheckFix(result)
		return nil
	})
}

func handleCheckCreateMissingResult(cmd *cobra.Command, result commandexec.Result) error {
	return handleCheckResult(cmd, result, func() error {
		return renderCanonicalCheckCreateMissing(getVaultPath(), result)
	})
}

func handleCheckResult(cmd *cobra.Command, result commandexec.Result, render func() error) error {
	strict, _ := cmd.Flags().GetBool("strict")
	if jsonOutput {
		if err := outputJSON(result); err != nil {
			return err
		}
		if checkShouldExit(result, strict) {
			os.Exit(1)
		}
		return nil
	}

	printCheckScopeHeader(getVaultPath(), checkScopeFromResult(result))
	if err := render(); err != nil {
		return err
	}
	if checkShouldExit(result, strict) {
		os.Exit(1)
	}
	return nil
}

func checkShouldExit(result commandexec.Result, strict bool) bool {
	data := canonicalDataMap(result)
	errorCount := intValue(data["error_count"])
	warningCount := intValue(data["warning_count"])
	if errorCount == 0 && warningCount == 0 {
		if decoded, ok := decodeCanonicalCheckJSON(result); ok {
			errorCount = decoded.ErrorCount
			warningCount = decoded.WarnCount
		}
	}
	return errorCount > 0 || (strict && warningCount > 0)
}

func init() {
	checkCmd.AddCommand(checkFixCmd)
	checkCmd.AddCommand(checkCreateMissingCmd)
	rootCmd.AddCommand(checkCmd)
}
