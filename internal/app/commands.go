package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandimpl"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/indexjournal"
)

var (
	commandInvokerOnce sync.Once
	commandInvoker     *commandexec.Invoker
)

// CommandInvoker returns the shared canonical command invoker.
func CommandInvoker() *commandexec.Invoker {
	commandInvokerOnce.Do(func() {
		registry := commandexec.NewHandlerRegistry()
		commandimpl.RegisterAll(registry)
		commandInvoker = commandexec.NewInvoker(registry, validateRequest).
			WithBeforeDispatch(beginIndexJournalOperation).
			WithResultAnnotator(annotateMutationPhase)
	})
	return commandInvoker
}

func beginIndexJournalOperation(_ context.Context, req commandexec.Request) (commandexec.Request, commandexec.Result, bool) {
	if req.Preview || !commands.UsesPostMutationIndex(req.CommandID) {
		return req, commandexec.Result{}, true
	}
	operationID, err := indexjournal.Begin(req.VaultPath)
	if err != nil {
		return req, commandexec.Failure(
			codes.ErrDatabase,
			"failed to establish index recovery guard",
			map[string]interface{}{"cause": err.Error()},
			"Restore write access to .raven and retry; no vault content was changed",
		), false
	}
	req.IndexJournalOperation = operationID
	return req, commandexec.Result{}, true
}

// annotateMutationPhase attaches the standard meta.mutation.phase signal to
// successful responses from mutating commands. It derives the phase from the
// normalized request's preview/apply resolution, so the signal stays consistent
// regardless of command class, flag vocabulary, or caller (CLI/MCP).
//
// A handler may set the phase explicitly for blocked or no-op states (e.g. a
// move awaiting confirmation writes nothing); that explicit phase is preserved.
func annotateMutationPhase(_ context.Context, req commandexec.Request, result commandexec.Result) commandexec.Result {
	if !result.OK {
		if err := indexjournal.Abandon(req.VaultPath, req.IndexJournalOperation); err != nil {
			result.Warnings = append(result.Warnings, commandexec.Warning{
				Code:    codes.WarnIndexUpdateFailed,
				Message: fmt.Sprintf("failed to release index recovery guard after mutation failure: %v", err),
				Ref:     "Run 'rvn reindex' before relying on index-backed results",
			})
		}
		return result
	}
	if result.Meta == nil || result.Meta.Mutation == nil {
		if commands.EmitsMutationPhase(req.CommandID) {
			phase := commandexec.MutationPhaseApplied
			if req.Preview {
				phase = commandexec.MutationPhasePreview
			}
			result = result.WithMutationPhase(phase)
		}
	}
	if !surfacesIndexDirtyWarning(req.CommandID) {
		return result
	}
	snapshot, err := indexjournal.Load(req.VaultPath)
	if err != nil {
		result.Warnings = append(result.Warnings, commandexec.Warning{
			Code:    codes.WarnDatabaseOutdated,
			Message: fmt.Sprintf("failed to inspect pending index work: %v", err),
			Ref:     "Run 'rvn reindex' before relying on index-backed results",
		})
		return result
	}
	if snapshot.Dirty() {
		result.Warnings = append(result.Warnings, commandexec.Warning{
			Code:    codes.WarnDatabaseOutdated,
			Message: "the derived index has pending updates from an interrupted or deferred mutation",
			Ref:     "Run 'rvn reindex' before relying on index-backed results",
		})
	}
	return result
}

func surfacesIndexDirtyWarning(commandID string) bool {
	switch commandID {
	case "query", "search", "read", "resolve", "backlinks", "outlinks", "open", "date", "check", "vault_stats":
		return true
	default:
		return false
	}
}

func validateRequest(_ context.Context, req commandexec.Request) (commandexec.Request, commandexec.Result, bool) {
	contract, ok := commands.BuildCommandContract(req.CommandID)
	if !ok {
		return req, commandexec.Failure(
			"COMMAND_NOT_FOUND",
			"unknown command: "+req.CommandID,
			map[string]interface{}{"command": req.CommandID},
			"Choose a registered command and retry",
		), false
	}

	spec := commands.BuildInvokeParamSpec(contract)

	normalized, issues := commands.ValidateArgumentsStrict(spec, req.Args)
	if len(issues) > 0 {
		return req, commandexec.Failure(
			"INVALID_ARGS",
			"argument validation failed",
			map[string]interface{}{
				"command":     req.CommandID,
				"issues":      issues,
				"args_schema": commands.CompactArgsSchema(contract),
				"schema_hash": contract.SchemaHash,
			},
			"Check command arguments and retry",
		), false
	}

	req.Args = normalized

	// Vault-scoped commands require a resolved vault path. CLI and MCP
	// transports resolve the vault before dispatch and surface their own
	// vault-resolution errors; this central check guards direct invoker
	// callers so every handler can assume req.VaultPath is non-empty and
	// no longer needs a duplicated defensive check. The check runs after
	// argument validation so INVALID_ARGS keeps priority over
	// VAULT_NOT_SPECIFIED — matching the pre-centralization handler order
	// where args were validated by the invoker before the handler's own
	// empty-path guard fired. Emit the stable VAULT_NOT_SPECIFIED code
	// because it is the documented contract for a missing vault (see
	// AGENTS.md and internal/codes) and matches the code the specialized
	// `vault_path` and read handlers already emitted for this condition;
	// INVALID_INPUT was a misleading defensive default in the removed
	// per-handler boilerplate.
	if commands.RequiresVault(req.CommandID) && strings.TrimSpace(req.VaultPath) == "" {
		return req, commandexec.Failure(
			codes.ErrVaultNotSpecified,
			"no vault path resolved",
			nil,
			"Use --vault-path, --vault, active_vault, or default_vault",
		), false
	}

	// `yes` is an apply flag used by commands that prompt in interactive
	// terminals (e.g. skill install); treat it like `confirm` for non-interactive
	// and MCP callers so preview/apply resolution stays consistent.
	req.Confirm = req.Confirm || normalizedBoolArg(normalized, "confirm") || normalizedBoolArg(normalized, "yes")
	switch {
	case normalizedBoolArg(normalized, "dry-run"):
		// An explicit dry run always previews and never applies, even if the
		// caller also passed confirm.
		req.Preview = true
		req.Confirm = false
	case req.Confirm:
		req.Preview = false
	case commands.ShouldPreviewByDefault(req.CommandID, normalized):
		req.Preview = true
	}
	return req, commandexec.Result{}, true
}

func normalizedBoolArg(args map[string]interface{}, name string) bool {
	value, ok := args[name].(bool)
	return ok && value
}
