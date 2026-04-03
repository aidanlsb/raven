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
				"command": req.CommandID,
				"issues":  issues,
			},
			"Check command arguments and retry",
		), false
	}

	req.Args = normalized
	return req, commandexec.Result{}, true
}
