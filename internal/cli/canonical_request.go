package cli

import (
	"context"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func executeCanonicalCommand(commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	return executeCanonicalRequest(commandexec.Request{
		CommandID:  commandID,
		VaultPath:  vaultPath,
		ConfigPath: configPath,
		StatePath:  statePathFlag,
		Caller:     commandexec.CallerCLI,
		Args:       args,
	})
}

func executeCanonicalRequest(req commandexec.Request) commandexec.Result {
	if req.ConfigPath == "" {
		req.ConfigPath = configPath
	}
	if req.StatePath == "" {
		req.StatePath = statePathFlag
	}
	if req.Caller == "" {
		req.Caller = commandexec.CallerCLI
	}
	return app.CommandInvoker().Execute(context.Background(), req)
}

func canonicalDataMap(result commandexec.Result) map[string]interface{} {
	data, _ := result.Data.(map[string]interface{})
	return data
}

func boolValue(raw interface{}) bool {
	value, ok := raw.(bool)
	return ok && value
}
