package app

import (
	"context"
	"sync"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandimpl"
	"github.com/aidanlsb/raven/internal/commands"
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
		commandInvoker = commandexec.NewInvoker(registry, validateRequest)
	})
	return commandInvoker
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
