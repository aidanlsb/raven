package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultMaxListItems = 50
	defaultMaxStringLen = 20000
)

func formatForPrompt(v interface{}) string {
	s := formatValue(v)
	if len(s) > defaultMaxStringLen {
		return s[:defaultMaxStringLen] + "\n… (truncated)"
	}
	return s
}

func formatValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		// json.Unmarshal uses float64 for numbers.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case []map[string]interface{}:
		arr := make([]interface{}, 0, len(t))
		for _, item := range t {
			// wrap to common formatter path
			arr = append(arr, item)
		}
		return formatArray(arr)
	case map[string]interface{}:
		return formatObject(t)
	case []interface{}:
		return formatArray(t)
	default:
		// Pretty JSON fallback for unknown types.
		b, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

func formatObject(obj map[string]interface{}) string {
	// If it has content, prefer showing that.
	if c, ok := obj["content"].(string); ok && strings.TrimSpace(c) != "" {
		return c
	}

	// Special-case common wrappers like {"results": [...]}
	if res, ok := obj["results"]; ok {
		return formatValue(res)
	}

	// Otherwise, stable key/value listing (good for LLM).
	var keys []string
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		val := obj[k]
		if val == nil {
			continue
		}
		switch vv := val.(type) {
		case string:
			if strings.TrimSpace(vv) == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("- **%s**: %s", k, vv))
		default:
			lines = append(lines, fmt.Sprintf("- **%s**: %s", k, formatValue(vv)))
		}
	}
	if len(lines) == 0 {
		return "(empty)"
	}
	return strings.Join(lines, "\n")
}

func formatArray(arr []interface{}) string {
	if len(arr) == 0 {
		return "(none)"
	}

	n := len(arr)
	if n > defaultMaxListItems {
		n = defaultMaxListItems
	}

	var lines []string
	for i := 0; i < n; i++ {
		item := arr[i]
		switch vv := item.(type) {
		case map[string]interface{}:
			lines = append(lines, formatObjectSummary(vv, i+1))
		case string:
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, vv))
		default:
			// Compact JSON for scalar-ish values.
			b, err := json.Marshal(vv)
			if err != nil {
				lines = append(lines, fmt.Sprintf("%d. %v", i+1, vv))
			} else {
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, string(b)))
			}
		}
	}
	if len(arr) > defaultMaxListItems {
		lines = append(lines, fmt.Sprintf("… (%d more)", len(arr)-defaultMaxListItems))
	}
	return strings.Join(lines, "\n")
}

func formatObjectSummary(obj map[string]interface{}, num int) string {
	// Prefer content-like fields.
	content := ""
	if c, ok := obj["content"].(string); ok && strings.TrimSpace(c) != "" {
		content = c
	}
	if content == "" {
		if c, ok := obj["title"].(string); ok && strings.TrimSpace(c) != "" {
			content = c
		}
	}
	if content == "" {
		if c, ok := obj["name"].(string); ok && strings.TrimSpace(c) != "" {
			content = c
		}
	}
	if content == "" {
		if c, ok := obj["id"].(string); ok && strings.TrimSpace(c) != "" {
			content = c
		}
	}

	var parts []string
	if content != "" {
		parts = append(parts, content)
	}
	if fp, ok := obj["file_path"].(string); ok && fp != "" {
		parts = append(parts, fmt.Sprintf("[%s]", fp))
	}
	if line, ok := obj["line"].(float64); ok && line > 0 {
		parts = append(parts, fmt.Sprintf("line %d", int(line)))
	}
	if v, ok := obj["value"].(string); ok && v != "" {
		parts = append(parts, fmt.Sprintf("(%s)", v))
	}
	if len(parts) == 0 {
		b, _ := json.Marshal(obj)
		return fmt.Sprintf("%d. %s", num, string(b))
	}
	return fmt.Sprintf("%d. %s", num, strings.Join(parts, " "))
}
