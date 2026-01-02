// Package cli implements the command-line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Global JSON output flag
var jsonOutput bool

// Response is the standard JSON envelope for all CLI output.
type Response struct {
	OK       bool        `json:"ok"`
	Data     interface{} `json:"data,omitempty"`
	Error    *ErrorInfo  `json:"error,omitempty"`
	Warnings []Warning   `json:"warnings,omitempty"`
	Meta     *Meta       `json:"meta,omitempty"`
}

// ErrorInfo contains structured error information.
type ErrorInfo struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Details    interface{} `json:"details,omitempty"`
	Suggestion string      `json:"suggestion,omitempty"`
}

// Warning represents a non-fatal warning.
type Warning struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	Ref           string `json:"ref,omitempty"`
	SuggestedType string `json:"suggested_type,omitempty"`
	CreateCommand string `json:"create_command,omitempty"`
}

// Meta contains metadata about the response.
type Meta struct {
	Count       int   `json:"count,omitempty"`
	QueryTimeMs int64 `json:"query_time_ms,omitempty"`
}

// outputJSON outputs the response as JSON to stdout.
func outputJSON(resp Response) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

// outputSuccess outputs a successful JSON response.
func outputSuccess(data interface{}, meta *Meta) {
	outputJSON(Response{
		OK:   true,
		Data: data,
		Meta: meta,
	})
}

// outputSuccessWithWarnings outputs a successful JSON response with warnings.
func outputSuccessWithWarnings(data interface{}, warnings []Warning, meta *Meta) {
	outputJSON(Response{
		OK:       true,
		Data:     data,
		Warnings: warnings,
		Meta:     meta,
	})
}

// outputError outputs an error JSON response.
func outputError(code, message string, details interface{}, suggestion string) {
	outputJSON(Response{
		OK: false,
		Error: &ErrorInfo{
			Code:       code,
			Message:    message,
			Details:    details,
			Suggestion: suggestion,
		},
	})
}

// outputErrorFromErr converts a Go error to a JSON error response.
func outputErrorFromErr(code string, err error, suggestion string) {
	outputError(code, err.Error(), nil, suggestion)
}

// isJSONOutput returns true if JSON output is enabled.
func isJSONOutput() bool {
	return jsonOutput
}

// withTiming wraps a function and returns execution time in milliseconds.
func withTiming(fn func() error) (int64, error) {
	start := time.Now()
	err := fn()
	elapsed := time.Since(start).Milliseconds()
	return elapsed, err
}

// printOrJSON outputs text if not in JSON mode, otherwise does nothing.
// Use this for human-readable output that should be suppressed in JSON mode.
func printOrJSON(format string, args ...interface{}) {
	if !jsonOutput {
		fmt.Printf(format, args...)
	}
}

// printlnOrJSON outputs a line if not in JSON mode.
func printlnOrJSON(a ...interface{}) {
	if !jsonOutput {
		fmt.Println(a...)
	}
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
