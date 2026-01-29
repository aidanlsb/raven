package workflow

import (
	"fmt"
	"strconv"
	"strings"
)

// interpolate replaces {{inputs.*}} and {{steps.*}} references inside s.
//
// Rules:
// - Escaping: \{{ and \}} produce literal braces.
// - Unknown variables are errors (to avoid silent typos).
func interpolate(s string, inputs map[string]string, steps map[string]interface{}) (string, error) {
	var out strings.Builder
	out.Grow(len(s))

	var errs []string

	for i := 0; i < len(s); {
		// Escapes
		if s[i] == '\\' && i+2 < len(s) && s[i+1] == '{' && s[i+2] == '{' {
			out.WriteString("{{")
			i += 3
			continue
		}
		if s[i] == '\\' && i+2 < len(s) && s[i+1] == '}' && s[i+2] == '}' {
			out.WriteString("}}")
			i += 3
			continue
		}

		// Interpolation
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			end := strings.Index(s[i+2:], "}}")
			if end < 0 {
				// No closing braces; treat literally.
				out.WriteByte(s[i])
				i++
				continue
			}
			end = i + 2 + end
			expr := strings.TrimSpace(s[i+2 : end])
			i = end + 2

			val, ok, err := resolveExpr(expr, inputs, steps)
			if err != nil {
				errs = append(errs, err.Error())
				out.WriteString("{{" + expr + "}}")
				continue
			}
			if !ok {
				errs = append(errs, fmt.Sprintf("unknown variable: %s", expr))
				out.WriteString("{{" + expr + "}}")
				continue
			}
			out.WriteString(val)
			continue
		}

		out.WriteByte(s[i])
		i++
	}

	if len(errs) > 0 {
		return out.String(), fmt.Errorf(strings.Join(errs, "; "))
	}
	return out.String(), nil
}

func resolveExpr(expr string, inputs map[string]string, steps map[string]interface{}) (string, bool, error) {
	if strings.HasPrefix(expr, "inputs.") {
		key := strings.TrimPrefix(expr, "inputs.")
		if key == "" {
			return "", false, fmt.Errorf("invalid inputs reference: %s", expr)
		}
		v, ok := inputs[key]
		if !ok {
			return "", false, nil
		}
		return v, true, nil
	}

	if strings.HasPrefix(expr, "steps.") {
		path := strings.TrimPrefix(expr, "steps.")
		val, ok := resolveStepPath(steps, path)
		if !ok {
			return "", false, nil
		}
		return formatForPrompt(val), true, nil
	}

	// context.* is an alias for steps.* used by simplified prompt workflows.
	if strings.HasPrefix(expr, "context.") {
		path := strings.TrimPrefix(expr, "context.")
		val, ok := resolveStepPath(steps, path)
		if !ok {
			return "", false, nil
		}
		return formatForPrompt(val), true, nil
	}

	return "", false, nil
}

func resolveStepPath(steps map[string]interface{}, path string) (interface{}, bool) {
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	cur, ok := steps[parts[0]]
	if !ok {
		return nil, false
	}
	for _, part := range parts[1:] {
		switch v := cur.(type) {
		case map[string]interface{}:
			next, ok := v[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// stringify was replaced by formatForPrompt in format.go.
