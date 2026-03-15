package toolexec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/rvnexec"
	"github.com/aidanlsb/raven/internal/toolargs"
)

// Execute runs a Raven tool by name and returns the parsed Raven JSON envelope.
// The executable should point to an rvn binary.
func Execute(vaultPath, executable, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, &Error{
			Code: CodeExecutableRequired,
			Tool: toolName,
			Err:  fmt.Errorf("rvn executable is required"),
		}
	}

	cmdArgs := toolargs.BuildCLIArgs(toolName, args)
	if len(cmdArgs) == 0 {
		return nil, &Error{
			Code: CodeUnknownTool,
			Tool: toolName,
			Err:  fmt.Errorf("unknown tool: %s", toolName),
		}
	}

	if strings.TrimSpace(vaultPath) != "" {
		cmdArgs = append([]string{"--vault-path", vaultPath}, cmdArgs...)
	}

	result, err := rvnexec.Run(executable, cmdArgs)
	if err != nil {
		if result.HasEnvelope && result.OK != nil {
			return nil, &Error{
				Code:   CodeToolReturnedError,
				Tool:   toolName,
				Output: result.OutputString(),
				Err:    fmt.Errorf("tool '%s' returned error: %s", toolName, result.OutputString()),
			}
		}

		out := result.TrimmedOutput()
		if out == "" {
			return nil, &Error{
				Code: CodeExecutionFailed,
				Tool: toolName,
				Err:  fmt.Errorf("tool '%s' failed: %w", toolName, err),
			}
		}
		return nil, &Error{
			Code:   CodeExecutionFailed,
			Tool:   toolName,
			Output: out,
			Err:    fmt.Errorf("tool '%s' failed: %s", toolName, out),
		}
	}

	envelope, parseErr := parseEnvelope(result.OutputString())
	if parseErr != nil {
		return nil, &Error{
			Code:   CodeInvalidJSON,
			Tool:   toolName,
			Output: result.OutputString(),
			Err:    fmt.Errorf("tool '%s' returned invalid JSON: %w", toolName, parseErr),
		}
	}

	if okValue, present := envelope["ok"]; present {
		if okFlag, ok := okValue.(bool); ok && !okFlag {
			return nil, &Error{
				Code:   CodeToolReturnedError,
				Tool:   toolName,
				Output: result.OutputString(),
				Err:    fmt.Errorf("tool '%s' returned error: %s", toolName, result.OutputString()),
			}
		}
	}

	return envelope, nil
}

func parseEnvelope(raw string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty response")
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return nil, err
	}
	return envelope, nil
}
