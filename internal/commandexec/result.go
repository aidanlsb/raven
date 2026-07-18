package commandexec

import "github.com/aidanlsb/raven/internal/codes"

// Result is the transport-neutral Raven execution envelope.
type Result struct {
	OK       bool        `json:"ok"`
	Data     interface{} `json:"data,omitempty"`
	Error    *ErrorInfo  `json:"error,omitempty"`
	Warnings []Warning   `json:"warnings,omitempty"`
	Meta     *Meta       `json:"meta,omitempty"`
}

// ErrorInfo contains structured error information.
type ErrorInfo struct {
	Code       codes.ErrorCode `json:"code"`
	Message    string          `json:"message"`
	Details    interface{}     `json:"details,omitempty"`
	Suggestion string          `json:"suggestion,omitempty"`
}

// Warning represents a non-fatal warning.
type Warning struct {
	Code          codes.WarningCode `json:"code"`
	Message       string            `json:"message"`
	Ref           string            `json:"ref,omitempty"`
	SuggestedType string            `json:"suggested_type,omitempty"`
	CreateCommand string            `json:"create_command,omitempty"`
	// CreateInvoke is the structured, transport-neutral form of CreateCommand so
	// agents can act on remediation without shell-parsing the CLI string.
	CreateInvoke *Invoke `json:"create_invoke,omitempty"`
}

// Invoke is a structured command invocation matching Raven's JSON/MCP call
// shape: a command ID plus its named arguments (as passed to raven_invoke).
type Invoke struct {
	Command string                 `json:"command"`
	Args    map[string]interface{} `json:"args,omitempty"`
}

// VaultContext identifies which vault was used and how it was resolved.
type VaultContext struct {
	Name   string `json:"name,omitempty"`
	Path   string `json:"path"`
	Source string `json:"source"`
}

// MutationPhase is the standard applied-vs-preview signal carried by every
// mutating command response. Agents must read this instead of inferring intent
// from heterogeneous data fields (e.g. data.status, data.preview).
type MutationPhase string

const (
	// MutationPhasePreview means no durable change was written; the response
	// describes what a subsequent apply would do.
	MutationPhasePreview MutationPhase = "preview"
	// MutationPhaseApplied means the change was written to the vault.
	MutationPhaseApplied MutationPhase = "applied"
)

// MutationMeta is the uniform mutation signal attached to mutating responses.
type MutationMeta struct {
	Phase MutationPhase `json:"phase"`
}

// Meta contains metadata about the response.
type Meta struct {
	Count        int           `json:"count,omitempty"`
	QueryTimeMs  int64         `json:"query_time_ms,omitempty"`
	VaultContext *VaultContext `json:"vault_context,omitempty"`
	// Mutation is present on responses from mutating commands and reports
	// whether the change was applied or only previewed.
	Mutation *MutationMeta `json:"mutation,omitempty"`
}

// WithMutationPhase returns the result with meta.mutation.phase set to the given
// phase, allocating Meta when needed. It is a no-op on failure envelopes (a
// failed mutation neither previews nor applies). Handlers use this to report a
// phase explicitly for blocked or no-op states (e.g. a move awaiting
// confirmation); otherwise the shared pipeline fills in the phase.
func (r Result) WithMutationPhase(phase MutationPhase) Result {
	if !r.OK {
		return r
	}
	if r.Meta == nil {
		r.Meta = &Meta{}
	}
	r.Meta.Mutation = &MutationMeta{Phase: phase}
	return r
}

// Success builds a successful result envelope.
func Success(data interface{}, meta *Meta) Result {
	return Result{
		OK:   true,
		Data: data,
		Meta: meta,
	}
}

// SuccessWithWarnings builds a successful result envelope with warnings.
func SuccessWithWarnings(data interface{}, warnings []Warning, meta *Meta) Result {
	return Result{
		OK:       true,
		Data:     data,
		Warnings: warnings,
		Meta:     meta,
	}
}

// Failure builds an error result envelope.
func Failure(code codes.ErrorCode, message string, details interface{}, suggestion string) Result {
	return Result{
		OK: false,
		Error: &ErrorInfo{
			Code:       code,
			Message:    message,
			Details:    details,
			Suggestion: suggestion,
		},
	}
}
