package mcp

import (
	"encoding/json"
	"strings"

	"github.com/aidanlsb/raven/internal/configsvc"
)

type directWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

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

func successEnvelope(data map[string]interface{}, warnings []directWarning) string {
	payload := map[string]interface{}{
		"ok":   true,
		"data": data,
	}
	if len(warnings) > 0 {
		payload["warnings"] = warnings
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func errorEnvelope(code, message, suggestion string, details map[string]interface{}) string {
	errPayload := map[string]interface{}{
		"code":    code,
		"message": message,
	}
	if suggestion != "" {
		errPayload["suggestion"] = suggestion
	}
	if len(details) > 0 {
		errPayload["details"] = details
	}

	payload := map[string]interface{}{
		"ok":    false,
		"error": errPayload,
	}
	b, _ := json.Marshal(payload)
	return string(b)
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
