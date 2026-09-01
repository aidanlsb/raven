package cli

import (
	"context"
	"fmt"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func executeCanonicalCommand(commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	req := commandexec.Request{
		CommandID:  commandID,
		VaultPath:  vaultPath,
		ConfigPath: configPath,
		StatePath:  statePathFlag,
		Caller:     commandexec.CallerCLI,
		Args:       args,
	}
	
	// Honor confirm/dry-run from args map (populated by buildCanonicalArgsForMeta)
	if confirm, ok := args["confirm"].(bool); ok && confirm {
		req.Confirm = true
	}
	if dryRun, ok := args["dry-run"].(bool); ok && dryRun {
		req.Preview = true
	}
	
	return executeCanonicalRequest(req)
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

func commandResultData[T any](result commandexec.Result) (T, error) {
	data, ok := result.Data.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("unexpected command result payload %T", result.Data)
	}
	return data, nil
}

func boolValue(raw interface{}) bool {
	value, ok := raw.(bool)
	return ok && value
}

func stringsToAny(values []string) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
