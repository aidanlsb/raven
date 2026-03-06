package rvnexec

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// Result captures raw process output plus parsed Raven JSON envelope metadata.
type Result struct {
	Output      []byte
	Envelope    map[string]interface{}
	HasEnvelope bool
	OK          *bool
}

// OutputString returns command output as a string.
func (r Result) OutputString() string {
	return string(r.Output)
}

// TrimmedOutput returns command output with surrounding whitespace removed.
func (r Result) TrimmedOutput() string {
	return strings.TrimSpace(string(r.Output))
}

// Run executes an rvn subprocess command and parses its JSON envelope when present.
func Run(executable string, args []string) (Result, error) {
	cmd := exec.Command(executable, args...)
	output, err := cmd.CombinedOutput()

	result := Result{
		Output: output,
	}

	var env map[string]interface{}
	if json.Unmarshal(output, &env) == nil {
		result.Envelope = env
		result.HasEnvelope = true
		if okVal, ok := env["ok"].(bool); ok {
			v := okVal
			result.OK = &v
		}
	}

	return result, err
}
