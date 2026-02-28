package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/ui"
)

var hookCmd = &cobra.Command{
	Use:   "hook <name>",
	Short: "Run a named hook command from raven.yaml",
	Long: `Run a named hook command defined under hooks: in raven.yaml.

Execution is subject to hook policy gates:
- vault-local hooks_enabled: true
- global config [hooks] policy for the active vault
- --no-hooks / RVN_NO_HOOKS are not set`,
	Args: cobra.ExactArgs(1),
	RunE: runHook,
}

func runHook(cmd *cobra.Command, args []string) error {
	if hooksSuppressed() {
		return handleErrorMsg(ErrInvalidInput, "hooks are disabled by --no-hooks or RVN_NO_HOOKS", "remove --no-hooks or unset RVN_NO_HOOKS")
	}

	vaultPath := getVaultPath()
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	if !hooksEnabledForCurrentVault(cfg, vaultCfg, getVaultName()) {
		return handleErrorMsg(ErrInvalidInput, "hooks are not enabled for this vault", "set hooks_enabled: true in raven.yaml and enable [hooks] policy in global config")
	}

	hookName := strings.TrimSpace(args[0])
	if hookName == "" {
		return handleErrorMsg(ErrMissingArgument, "hook name is required", "Usage: rvn hook <name>")
	}

	if _, ok := vaultCfg.Hooks[hookName]; !ok {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("hook %q is not defined in raven.yaml", hookName), "")
	}

	depth := currentHookDepth()
	if depth >= maxHookDepthV1 {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("max hook depth reached (%d)", maxHookDepthV1), "")
	}

	configWarnings := validateTriggers(vaultCfg.Triggers)
	cliWarnings := make([]Warning, 0, len(configWarnings))
	for _, w := range configWarnings {
		cliWarnings = append(cliWarnings, Warning{
			Code:    WarnHookConfig,
			Message: w,
		})
	}

	result, runErr := runNamedHook(vaultPath, vaultCfg, hookName, "manual", depth)
	if runErr != nil {
		if isJSONOutput() {
			outputError(ErrInternal, fmt.Sprintf("hook %q failed", hookName), result, "")
			return nil
		}
		if result.Stdout != "" {
			fmt.Println(result.Stdout)
		}
		if result.Stderr != "" {
			fmt.Fprintln(os.Stderr, result.Stderr)
		}
		return fmt.Errorf("hook %q failed (exit=%d)", hookName, result.ExitCode)
	}

	if isJSONOutput() {
		if len(cliWarnings) > 0 {
			outputSuccessWithWarnings(result, cliWarnings, nil)
			return nil
		}
		outputSuccess(result, nil)
		return nil
	}

	if result.Stdout != "" {
		fmt.Println(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(os.Stderr, result.Stderr)
	}
	for _, w := range cliWarnings {
		fmt.Println(ui.Warning(w.Message))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
