package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/ui"
)

const (
	hookDepthEnv     = "RVN_HOOK_DEPTH"
	hookNoHooksEnv   = "RVN_NO_HOOKS"
	hookNameEnv      = "RVN_HOOK_NAME"
	hookTriggerEnv   = "RVN_HOOK_TRIGGER"
	maxHookDepthV1   = 1
	defaultShellPath = "/bin/sh"
)

type hookRunResult struct {
	Name       string `json:"name"`
	Trigger    string `json:"trigger"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
}

func maybeRunCommandHooks(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if hooksSuppressed() {
		return
	}
	vaultPath := strings.TrimSpace(getVaultPath())
	if vaultPath == "" {
		return
	}

	commandPath := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "rvn "))
	if commandPath == "" {
		return
	}
	commandID, meta, ok := commands.LookupMetaByPath(commandPath)
	if !ok || !meta.MutatesVault {
		return
	}
	if !mutationApplied(cmd, commandID) {
		return
	}

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		emitHookWarning(WarnHookConfig, fmt.Sprintf("failed to load raven.yaml for hooks: %v", err))
		return
	}
	if !hooksEnabledForCurrentVault(cfg, vaultCfg, getVaultName()) {
		return
	}

	depth := currentHookDepth()
	if depth >= maxHookDepthV1 {
		emitHookWarning(WarnHookExecution, fmt.Sprintf("skipping hook trigger after:%s: max hook depth reached (%d)", commandID, maxHookDepthV1))
		return
	}

	hookNames, warnings := resolveHooksForCommand(vaultCfg, commandID)
	for _, w := range warnings {
		emitHookWarning(WarnHookConfig, w)
	}
	if len(hookNames) == 0 {
		return
	}

	for _, hookName := range hookNames {
		result, runErr := runNamedHook(vaultPath, vaultCfg, hookName, "after:"+commandID, depth)
		if runErr != nil {
			emitHookWarning(WarnHookExecution, fmt.Sprintf("hook %q failed (%s): %v", hookName, result.Trigger, runErr))
		}
	}
}

func hooksEnabledForCurrentVault(globalCfg *config.Config, vaultCfg *config.VaultConfig, vaultName string) bool {
	if vaultCfg == nil || !vaultCfg.IsHooksEnabled() {
		return false
	}
	if globalCfg == nil {
		return false
	}
	return globalCfg.HooksEnabledForVault(vaultName)
}

func resolveHooksForCommand(vaultCfg *config.VaultConfig, commandID string) ([]string, []string) {
	if vaultCfg == nil || len(vaultCfg.Triggers) == 0 {
		return nil, nil
	}

	warnings := validateTriggers(vaultCfg.Triggers)

	ordered := make([]string, 0)
	seen := make(map[string]struct{})
	matchingKeys := []string{"after:" + commandID}
	if alt := strings.ReplaceAll(commandID, "_", " "); alt != commandID {
		matchingKeys = append(matchingKeys, "after:"+alt)
	}
	matchingKeys = append(matchingKeys, "after:*")

	for _, key := range matchingKeys {
		hooks := vaultCfg.Triggers[key]
		for _, hookName := range hooks {
			trimmed := strings.TrimSpace(hookName)
			if trimmed == "" {
				continue
			}
			if _, ok := vaultCfg.Hooks[trimmed]; !ok {
				warnings = append(warnings, fmt.Sprintf("trigger %q references unknown hook %q", key, trimmed))
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			ordered = append(ordered, trimmed)
		}
	}

	return ordered, warnings
}

func validateTriggers(triggers map[string]config.HookRefList) []string {
	if len(triggers) == 0 {
		return nil
	}

	keys := make([]string, 0, len(triggers))
	for key := range triggers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	warnings := make([]string, 0)
	for _, key := range keys {
		if !strings.HasPrefix(key, "after:") {
			warnings = append(warnings, fmt.Sprintf("invalid trigger %q: expected prefix \"after:\"", key))
			continue
		}
		commandRef := strings.TrimSpace(strings.TrimPrefix(key, "after:"))
		if commandRef == "*" {
			continue
		}
		commandID, ok := commands.ResolveCommandID(commandRef)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown trigger command %q in %q", commandRef, key))
			continue
		}
		meta, ok := commands.Registry[commandID]
		if ok && !meta.MutatesVault {
			warnings = append(warnings, fmt.Sprintf("trigger %q targets non-mutating command %q", key, commandID))
		}
	}
	return warnings
}

func mutationApplied(cmd *cobra.Command, commandID string) bool {
	switch commandID {
	case "add", "delete", "move", "set", "update":
		if boolFlag(cmd, "stdin") && !boolFlag(cmd, "confirm") {
			return false
		}
		return true
	case "import":
		return !boolFlag(cmd, "dry-run")
	case "workflow_runs_prune":
		return boolFlag(cmd, "confirm")
	default:
		return true
	}
}

func boolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return false
	}
	return value
}

func runNamedHook(vaultPath string, vaultCfg *config.VaultConfig, hookName, trigger string, depth int) (hookRunResult, error) {
	result := hookRunResult{
		Name:    hookName,
		Trigger: trigger,
	}

	if vaultCfg == nil {
		return result, fmt.Errorf("nil vault config")
	}
	command := strings.TrimSpace(vaultCfg.Hooks[hookName])
	if command == "" {
		return result, fmt.Errorf("hook %q is not defined", hookName)
	}
	result.Command = command

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = defaultShellPath
	}

	ctx, cancel := context.WithTimeout(context.Background(), vaultCfg.GetHooksTimeout())
	defer cancel()

	execCmd := exec.CommandContext(ctx, shell, "-c", command)
	execCmd.Dir = vaultPath
	execCmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=1", hookNoHooksEnv),
		fmt.Sprintf("%s=%d", hookDepthEnv, depth+1),
		fmt.Sprintf("%s=%s", hookNameEnv, hookName),
		fmt.Sprintf("%s=%s", hookTriggerEnv, trigger),
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	start := time.Now()
	err := execCmd.Run()
	result.DurationMs = time.Since(start).Milliseconds()
	result.Stdout = strings.TrimSpace(stdout.String())
	result.Stderr = strings.TrimSpace(stderr.String())

	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = -1
	}

	return result, err
}

func hooksSuppressed() bool {
	if noHooksFlag {
		return true
	}
	return envBoolTrue(hookNoHooksEnv)
}

func currentHookDepth() int {
	raw := strings.TrimSpace(os.Getenv(hookDepthEnv))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func envBoolTrue(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func emitHookWarning(code, message string) {
	msg := fmt.Sprintf("%s: %s", code, message)
	if isJSONOutput() {
		fmt.Fprintln(os.Stderr, msg)
		return
	}
	fmt.Fprintln(os.Stderr, ui.Warning(msg))
}
