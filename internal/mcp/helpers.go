package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
)

func (s *Server) directConfigContextOptions() configsvc.ContextOptions {
	opts := configsvc.ContextOptions{}
	for i := 0; i < len(s.baseArgs); i++ {
		arg := strings.TrimSpace(s.baseArgs[i])
		switch {
		case arg == "--config" && i+1 < len(s.baseArgs):
			opts.ConfigPathOverride = strings.TrimSpace(s.baseArgs[i+1])
			i++
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPathOverride = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
		case arg == "--state" && i+1 < len(s.baseArgs):
			opts.StatePathOverride = strings.TrimSpace(s.baseArgs[i+1])
			i++
		case strings.HasPrefix(arg, "--state="):
			opts.StatePathOverride = strings.TrimSpace(strings.TrimPrefix(arg, "--state="))
		}
	}
	return opts
}

// marshalResultEnvelope is the single adapter that turns a commandexec.Result
// into the wire JSON returned by every MCP tool call. The compact tools
// (raven_discover, raven_describe) and raven_invoke all funnel through this, so
// the server emits exactly one envelope shape regardless of which tool produced
// it. It returns the encoded envelope plus whether the result is an error, for
// the MCP tool-result isError flag.
func marshalResultEnvelope(result commandexec.Result) (string, bool) {
	b, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp: failed to marshal response envelope: %v\n", err)
		code := string(codes.ErrInternal)
		message := "failed to marshal response"
		suggestion := ""
		if !result.OK && result.Error != nil {
			code = string(result.Error.Code)
			message = result.Error.Message
			suggestion = result.Error.Suggestion
		}
		return fallbackEnvelopeJSON(code, message, suggestion, nil), true
	}
	return string(b), !result.OK
}

// successEnvelope builds a success envelope from a data payload. It wraps the
// data in a commandexec.Result and routes it through the shared adapter so
// compact-tool responses share the invoke path's envelope shape.
func successEnvelope(data map[string]interface{}, warnings []commandexec.Warning) string {
	result := commandexec.Result{OK: true, Data: data}
	if len(warnings) > 0 {
		result.Warnings = warnings
	}
	out, _ := marshalResultEnvelope(result)
	return out
}

// errorEnvelope builds an error envelope from a stable code, message, and
// optional suggestion/details. It routes through the shared commandexec.Result
// adapter so error responses match the invoke path's shape.
func errorEnvelope(code, message, suggestion string, details map[string]interface{}) string {
	errInfo := &commandexec.ErrorInfo{
		Code:       codes.ErrorCode(code),
		Message:    message,
		Suggestion: suggestion,
	}
	if len(details) > 0 {
		errInfo.Details = details
	}
	out, _ := marshalResultEnvelope(commandexec.Result{Error: errInfo})
	return out
}

func fallbackEnvelopeJSON(code, message, suggestion string, details map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(`{"ok":false,"error":{"code":`)
	b.WriteString(strconv.Quote(code))
	b.WriteString(`,"message":`)
	b.WriteString(strconv.Quote(message))
	if suggestion != "" {
		b.WriteString(`,"suggestion":`)
		b.WriteString(strconv.Quote(suggestion))
	}
	if len(details) > 0 {
		if detailJSON, err := json.Marshal(details); err == nil {
			b.WriteString(`,"details":`)
			b.Write(detailJSON)
		}
	}
	b.WriteString("}}")
	return b.String()
}

func boolValue(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "1", "true", "yes", "y", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
