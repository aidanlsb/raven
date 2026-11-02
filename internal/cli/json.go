// Package cli implements the command-line interface.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

// outputJSON outputs the response as compact JSON to stdout (no indentation).
// Compact encoding matches MCP and `rvn api` so agent-facing JSON is consistent.
func outputJSON(resp Response) error {
	enc := json.NewEncoder(os.Stdout)
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
	if err == nil {
		return formatHumanError("", suggestion)
	}
	return formatHumanError(err.Error(), suggestion)
}

// handleErrorMsg handles an error message appropriately based on output mode.
func handleErrorMsg(code codes.ErrorCode, message, suggestion string) error {
	if jsonOutput {
		if encErr := outputError(code, message, nil, suggestion); encErr != nil {
			return encErr
		}
		return nil
	}
	return formatHumanError(message, suggestion)
}

// handleErrorWithDetails handles an error with structured details.
func handleErrorWithDetails(code codes.ErrorCode, message, suggestion string, details interface{}) error {
	if jsonOutput {
		if encErr := outputError(code, message, details, suggestion); encErr != nil {
			return encErr
		}
		return nil
	}
	return formatHumanError(message, suggestion)
}

func formatHumanError(message, suggestion string) error {
	message = strings.TrimSpace(message)
	suggestion = strings.TrimSpace(suggestion)
	if message == "" {
		if suggestion == "" {
			return fmt.Errorf("error")
		}
		return fmt.Errorf("%s", suggestion)
	}
	if suggestion == "" {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s\n%s", message, suggestion)
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
	message := err.Error()
	var typed *flagParseError
	if errors.As(err, &typed) && typed != nil {
		message = typed.message
	}
	return outputError(codes.ErrInvalidInput, message, nil, flagParseSuggestion(err))
}

// flagParseError is a cobra flag-parse failure with a human/JSON suggestion.
type flagParseError struct {
	message    string
	suggestion string
	cause      error
}

func (e *flagParseError) Error() string {
	if e == nil {
		return ""
	}
	if e.suggestion == "" || jsonRequestedFromArgs() {
		return e.message
	}
	return e.message + "\n" + e.suggestion
}

func (e *flagParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func jsonRequestedFromArgs() bool {
	return argsRequestJSON(os.Args[1:])
}

func flagUsageError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	var existing *flagParseError
	if errors.As(err, &existing) {
		return err
	}
	return &flagParseError{
		message:    err.Error(),
		suggestion: flagParseSuggestionFor(cmd, err),
		cause:      err,
	}
}

func flagParseSuggestion(err error) string {
	if err == nil {
		return ""
	}
	var typed *flagParseError
	if errors.As(err, &typed) {
		return typed.suggestion
	}
	return flagParseSuggestionFor(nil, err)
}

func flagParseSuggestionFor(cmd *cobra.Command, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	helpLine := "Run 'rvn --help'"
	if cmd != nil {
		helpLine = fmt.Sprintf("Run '%s --help'", cmd.CommandPath())
	}

	if isDashPrefixedValueError(msg) {
		return `If an argument starts with a dash, put it after -- so it is not parsed as a flag. Keep flags such as --to and --json before --. Example: rvn add --to today --json -- "- Review the rollout"`
	}

	if strings.Contains(msg, "unknown shorthand flag") || strings.Contains(msg, "unknown flag:") {
		if suggestion := unknownFlagSuggestion(cmd, msg); suggestion != "" {
			return suggestion + "\n" + helpLine
		}
		return helpLine
	}
	return helpLine
}

func isDashPrefixedValueError(msg string) bool {
	if strings.Contains(msg, "unknown shorthand flag: ' '") {
		return true
	}
	token, ok := unknownFlagToken(msg)
	if !ok {
		return false
	}
	return !isPlausibleFlagToken(token)
}

func unknownFlagToken(msg string) (string, bool) {
	const unknownFlag = "unknown flag: "
	if i := strings.Index(msg, unknownFlag); i >= 0 {
		token := strings.TrimSpace(msg[i+len(unknownFlag):])
		if cut := strings.IndexAny(token, " \t="); cut >= 0 {
			token = token[:cut]
		}
		return token, token != ""
	}
	const inSep = " in "
	if i := strings.LastIndex(msg, inSep); i >= 0 && strings.Contains(msg, "unknown shorthand flag") {
		return strings.TrimSpace(msg[i+len(inSep):]), true
	}
	return "", false
}

func isPlausibleFlagToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, " \t") {
		return false
	}
	name := strings.TrimLeft(token, "-")
	if name == "" || name == token {
		return false
	}
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func unknownFlagSuggestion(cmd *cobra.Command, msg string) string {
	token, ok := unknownFlagToken(msg)
	if !ok || cmd == nil {
		return ""
	}
	want := strings.TrimLeft(token, "-")
	if want == "" {
		return ""
	}
	shorthand := strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") && len(want) == 1

	bestName := ""
	bestDist := 0
	visit := func(f *pflag.Flag) {
		candidates := []string{"--" + f.Name}
		if f.Shorthand != "" {
			candidates = append(candidates, "-"+f.Shorthand)
		}
		for _, candidate := range candidates {
			got := strings.TrimLeft(candidate, "-")
			if shorthand && len(got) != 1 {
				continue
			}
			dist := levenshtein(strings.ToLower(want), strings.ToLower(got))
			if dist == 0 || dist > 2 {
				continue
			}
			if bestName == "" || dist < bestDist || (dist == bestDist && candidate < bestName) {
				bestName = candidate
				bestDist = dist
			}
		}
	}
	cmd.LocalFlags().VisitAll(visit)
	cmd.InheritedFlags().VisitAll(visit)
	if bestName == "" {
		return ""
	}
	return fmt.Sprintf("Unknown flag; did you mean %s?", bestName)
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
