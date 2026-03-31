package toolargs

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

// BuildCLIArgs builds CLI arguments from MCP tool arguments using the command registry.
// It returns nil for unknown tool names.
func BuildCLIArgs(toolName string, args map[string]interface{}) []string {
	cmdID, ok := commands.ResolveToolCommandID(toolName)
	if !ok {
		return nil
	}
	meta := commands.Registry[cmdID]

	normalizedArgs := normalizeArgs(args)
	cliArgs := strings.Fields(meta.Name)

	for _, flag := range meta.Flags {
		val, ok := normalizedArgs[flag.Name]
		if !ok {
			continue
		}

		switch flag.Type {
		case commands.FlagTypeBool:
			if boolVal, ok := val.(bool); ok && boolVal {
				cliArgs = append(cliArgs, "--"+flag.Name)
			}
		case commands.FlagTypeInt:
			if intVal, ok := intFlagValue(val); ok {
				cliArgs = append(cliArgs, "--"+flag.Name, intVal)
			}
		case commands.FlagTypeStringSlice:
			for _, item := range stringSliceValues(val) {
				if item != "" {
					cliArgs = append(cliArgs, "--"+flag.Name, item)
				}
			}
		case commands.FlagTypeJSON:
			switch typed := val.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					cliArgs = append(cliArgs, "--"+flag.Name, typed)
				}
			default:
				b, err := json.Marshal(typed)
				if err == nil {
					cliArgs = append(cliArgs, "--"+flag.Name, string(b))
				}
			}
		case commands.FlagTypeKeyValue:
			if isObjectArg(val) && hasFlag(meta.Flags, flag.Name+"-json") {
				continue
			}
			for _, pair := range keyValuePairs(val) {
				cliArgs = append(cliArgs, "--"+flag.Name, pair)
			}
		case commands.FlagTypePosKeyValue:
			continue
		default:
			if strVal := toString(val); strVal != "" {
				cliArgs = append(cliArgs, "--"+flag.Name, strVal)
			}
		}
	}

	cliArgs = append(cliArgs, "--json")
	cliArgs = append(cliArgs, "--")

	for _, arg := range meta.Args {
		if val, ok := normalizedArgs[arg.Name]; ok {
			if strVal, ok := val.(string); ok {
				cliArgs = append(cliArgs, strVal)
			}
		}
	}

	for _, flag := range meta.Flags {
		if flag.Type == commands.FlagTypePosKeyValue {
			if val, ok := normalizedArgs[flag.Name]; ok {
				if isObjectArg(val) && hasFlag(meta.Flags, flag.Name+"-json") {
					continue
				}
				cliArgs = append(cliArgs, keyValuePairs(val)...)
			}
		}
	}

	return cliArgs
}

func intFlagValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case int:
		return strconv.Itoa(val), true
	case int8:
		return strconv.FormatInt(int64(val), 10), true
	case int16:
		return strconv.FormatInt(int64(val), 10), true
	case int32:
		return strconv.FormatInt(int64(val), 10), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case uint:
		return strconv.FormatUint(uint64(val), 10), true
	case uint8:
		return strconv.FormatUint(uint64(val), 10), true
	case uint16:
		return strconv.FormatUint(uint64(val), 10), true
	case uint32:
		return strconv.FormatUint(uint64(val), 10), true
	case uint64:
		return strconv.FormatUint(val, 10), true
	case float32:
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return "", false
		}
		return strconv.FormatInt(int64(val), 10), true
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return "", false
		}
		return strconv.FormatInt(int64(val), 10), true
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return strconv.FormatInt(i, 10), true
		}
		if f, err := val.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return strconv.FormatInt(int64(f), 10), true
		}
		return "", false
	default:
		return "", false
	}
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%v", val)
	case bool:
		if val {
			return "true"
		}
		return ""
	default:
		return ""
	}
}

func stringSliceValues(v interface{}) []string {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		return []string{s}
	case []string:
		out := make([]string, 0, len(val))
		for _, item := range val {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			str := strings.TrimSpace(toString(item))
			if str != "" {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func keyValuePairs(v interface{}) []string {
	switch typed := v.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(typed)
		if s == "" {
			return nil
		}
		return []string{s}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s := strings.TrimSpace(toString(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case map[string]interface{}:
		out := make([]string, 0, len(typed))
		for k, value := range typed {
			out = append(out, fmt.Sprintf("%s=%v", k, value))
		}
		return out
	default:
		return nil
	}
}

func hasFlag(flags []commands.FlagMeta, name string) bool {
	for _, f := range flags {
		if f.Name == name {
			return true
		}
	}
	return false
}

func isObjectArg(v interface{}) bool {
	if v == nil {
		return false
	}
	_, ok := v.(map[string]interface{})
	return ok
}

func normalizeArgs(args map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(args)*2)
	for k, v := range args {
		normalized[k] = v
		hyphenKey := strings.ReplaceAll(k, "_", "-")
		if hyphenKey != k {
			normalized[hyphenKey] = v
		}
	}

	if v, ok := normalized["fields"]; ok {
		if _, exists := normalized["field"]; !exists {
			normalized["field"] = v
		}
	}
	if v, ok := normalized["field"]; ok {
		if _, exists := normalized["fields"]; !exists {
			normalized["fields"] = v
		}
	}

	if v, ok := normalized["field"]; ok && isObjectArg(v) {
		if _, exists := normalized["field-json"]; !exists {
			normalized["field-json"] = v
		}
	}
	if v, ok := normalized["fields"]; ok && isObjectArg(v) {
		if _, exists := normalized["fields-json"]; !exists {
			normalized["fields-json"] = v
		}
	}

	return normalized
}
