package commandexec

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
func Failure(code, message string, details interface{}, suggestion string) Result {
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
