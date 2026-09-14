// Package cli implements the command-line interface.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

// Global JSON output flag
var jsonOutput bool

var errJSONResponseFailure = errors.New("json failure response written")

// Response is the standard JSON envelope for all CLI output.
type Response = commandexec.Result

// ErrorInfo contains structured error information.
type ErrorInfo = commandexec.ErrorInfo

// Warning represents a non-fatal warning.
type Warning = commandexec.Warning

// Meta contains metadata about the response.
type Meta = commandexec.Meta

// outputJSON outputs the response as JSON to stdout.
func outputJSON(resp Response) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to write JSON output: %w", err)
	}
	if !resp.OK {
		return errJSONResponseFailure
	}
	return nil
}

// outputSuccess outputs a successful JSON response.
func outputSuccess(data interface{}, meta *Meta) error {
	return outputJSON(commandexec.Success(data, meta))
}

// outputSuccessWithWarnings outputs a successful JSON response with warnings.
func outputSuccessWithWarnings(data interface{}, warnings []Warning, meta *Meta) error {
	return outputJSON(commandexec.SuccessWithWarnings(data, warnings, meta))
}

// outputError outputs an error JSON response.
func outputError(code codes.ErrorCode, message string, details interface{}, suggestion string) error {
	return outputJSON(commandexec.Failure(code, message, details, suggestion))
}

// outputErrorFromErr converts a Go error to a JSON error response.
func outputErrorFromErr(code codes.ErrorCode, err error, suggestion string) error {
	return outputError(code, err.Error(), nil, suggestion)
}

// isJSONOutput returns true if JSON output is enabled.
func isJSONOutput() bool {
	return jsonOutput
}

// handleError handles an error appropriately based on output mode.
// In JSON mode, outputs a JSON error. In text mode, returns the error for Cobra.
func handleError(code codes.ErrorCode, err error, suggestion string) error {
	if jsonOutput {
		if encErr := outputErrorFromErr(code, err, suggestion); encErr != nil {
			return encErr
		}
		return nil // Don't let Cobra also print the error
	}
	return err
}

// handleErrorMsg handles an error message appropriately based on output mode.
func handleErrorMsg(code codes.ErrorCode, message, suggestion string) error {
	if jsonOutput {
		if encErr := outputError(code, message, nil, suggestion); encErr != nil {
			return encErr
		}
		return nil
	}
	return fmt.Errorf("%s", message)
}

// handleErrorWithDetails handles an error with structured details.
func handleErrorWithDetails(code codes.ErrorCode, message, suggestion string, details interface{}) error {
	if jsonOutput {
		if encErr := outputError(code, message, details, suggestion); encErr != nil {
			return encErr
		}
		return nil
	}
	return fmt.Errorf("%s", message)
}

// emitJSONErrorEnvelope writes a JSON failure envelope when --json was requested
// and Cobra failed before RunE could write one. Flag-parse errors are the usual
// case: Cobra never reaches the command, and SilenceErrors has already turned
// off the human printer, so a bare exit 1 used to look like a successful no-op.
func emitJSONErrorEnvelope(jsonRequested bool, err error) error {
	if err == nil || !jsonRequested {
		return err
	}
	if errors.Is(err, errJSONResponseFailure) || errors.Is(err, ErrPickCancelled) || errors.Is(err, flag.ErrHelp) {
		return err
	}
	return outputError(codes.ErrInvalidInput, err.Error(), nil, flagParseSuggestion(err))
}

func flagParseSuggestion(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "unknown shorthand flag") || strings.Contains(msg, "unknown flag:") {
		return `If an argument starts with a dash, put it after -- so it is not parsed as a flag. Keep flags such as --to and --json before --. Example: rvn add --to today --json -- "- Review the rollout"`
	}
	return ""
}
