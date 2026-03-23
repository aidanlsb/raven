// Package cli implements the command-line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aidanlsb/raven/internal/commandexec"
)

// Global JSON output flag
var jsonOutput bool

// Response is the standard JSON envelope for all CLI output.
type Response = commandexec.Result

// ErrorInfo contains structured error information.
type ErrorInfo = commandexec.ErrorInfo

// Warning represents a non-fatal warning.
type Warning = commandexec.Warning

// Meta contains metadata about the response.
type Meta = commandexec.Meta

// outputJSON outputs the response as JSON to stdout.
func outputJSON(resp Response) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}

// outputSuccess outputs a successful JSON response.
func outputSuccess(data interface{}, meta *Meta) {
	outputJSON(commandexec.Success(data, meta))
}

// outputSuccessWithWarnings outputs a successful JSON response with warnings.
func outputSuccessWithWarnings(data interface{}, warnings []Warning, meta *Meta) {
	outputJSON(commandexec.SuccessWithWarnings(data, warnings, meta))
}

// outputError outputs an error JSON response.
func outputError(code, message string, details interface{}, suggestion string) {
	outputJSON(commandexec.Failure(code, message, details, suggestion))
}

// outputErrorFromErr converts a Go error to a JSON error response.
func outputErrorFromErr(code string, err error, suggestion string) {
	outputError(code, err.Error(), nil, suggestion)
}

// isJSONOutput returns true if JSON output is enabled.
func isJSONOutput() bool {
	return jsonOutput
}

// handleError handles an error appropriately based on output mode.
// In JSON mode, outputs a JSON error. In text mode, returns the error for Cobra.
func handleError(code string, err error, suggestion string) error {
	if jsonOutput {
		outputErrorFromErr(code, err, suggestion)
		return nil // Don't let Cobra also print the error
	}
	return err
}

// handleErrorMsg handles an error message appropriately based on output mode.
func handleErrorMsg(code, message, suggestion string) error {
	if jsonOutput {
		outputError(code, message, nil, suggestion)
		return nil
	}
	return fmt.Errorf("%s", message)
}

// handleErrorWithDetails handles an error with structured details.
func handleErrorWithDetails(code, message, suggestion string, details interface{}) error {
	if jsonOutput {
		outputError(code, message, details, suggestion)
		return nil
	}
	return fmt.Errorf("%s", message)
}
