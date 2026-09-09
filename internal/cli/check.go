package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
)

// checkCmd is the validate-only parent. Repairs live in the `fix` and
// `create-missing` subcommands (see #91). All three are thin canonical leaves
// that delegate execution to commandimpl/checksvc; the CLI only builds args,
// prompts, and renders.
var checkCmd = newExceptionLeafCommand("check", exceptionLeafOptions{
	HandleResult: handleCheckValidateResult,
})

var checkFixCmd = newExceptionLeafCommand("check_fix", exceptionLeafOptions{
	Invoke:       invokeCheckMutation,
	HandleResult: handleCheckFixResult,
})

var checkCreateMissingCmd = newExceptionLeafCommand("check create-missing", exceptionLeafOptions{
	Invoke:       invokeCheckMutation,
	HandleResult: handleCheckCreateMissingResult,
})

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
	switch data := result.Data.(type) {
	case commandpayload.CheckFixPreviewResult:
		return data.ErrorCount > 0 || (strict && data.WarningCount > 0)
	case commandpayload.CheckFixResult:
		return data.ErrorCount > 0 || (strict && data.WarningCount > 0)
	case commandpayload.CheckCreateMissingResult:
		return data.ErrorCount > 0 || (strict && data.WarningCount > 0)
	}
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
